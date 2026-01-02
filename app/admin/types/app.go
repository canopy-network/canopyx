package types

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/crosschain"
	"go.temporal.io/sdk/worker"

	"github.com/canopy-network/canopyx/pkg/temporal/indexer"

	adminstore "github.com/canopy-network/canopyx/pkg/db/admin"
	chainstore "github.com/canopy-network/canopyx/pkg/db/chain"
	"github.com/canopy-network/canopyx/pkg/redis"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"github.com/puzpuzpuz/xsync/v4"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
)

type App struct {
	// Database Client wrappers
	AdminDB      *adminstore.DB
	ChainsDB     *xsync.Map[string, chainstore.Store]
	CrossChainDB crosschain.Store

	// Temporal Client wrapper
	TemporalClient *temporal.Client

	// Temporal Workers
	MaintenanceWorker worker.Worker // Maintenance worker for cross-chain compaction (go.temporal.io/sdk/worker.Worker)

	// Redis Client (for WebSocket real-time events)
	RedisClient *redis.Client

	// Zap Logger
	Logger *zap.Logger

	// HTTP Server
	Server *http.Server

	// Queue stats cache (30s TTL to reduce Temporal API calls)
	QueueStatsCache *xsync.Map[string, CachedQueueStats]
}

// Start starts the application.
func (a *App) Start(ctx context.Context) {
	// Start a maintenance worker if it exists
	if a.MaintenanceWorker != nil {
		// Type assert to worker.Worker interface
		if err := a.MaintenanceWorker.Start(); err != nil {
			a.Logger.Fatal("Unable to start maintenance worker", zap.Error(err))
		}
		a.Logger.Info("Maintenance worker started successfully")
	}

	go func() { _ = a.Server.ListenAndServe() }()
	<-ctx.Done()

	// Stop maintenance worker
	if a.MaintenanceWorker != nil {
		a.Logger.Info("Stopping maintenance worker")
		a.MaintenanceWorker.Stop()
	}

	if a.AdminDB != nil {
		a.Logger.Info("closing admin database connection")
		err := a.AdminDB.Close()
		// This closes the shared client for cross-chain and chains connection
		if err != nil {
			a.Logger.Error("Failed to close database connection", zap.Error(err))
		}
	}

	a.Logger.Info("shutting down server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = a.Server.Shutdown(shutdownCtx)

	time.Sleep(200 * time.Millisecond)
	a.Logger.Info("さようなら!")
}

// NewChainDb initializes or retrieves an instance of ChainDB for a given blockchain identified by chainID.
// The database and tables must already exist (created by indexer).
// Returns the ChainDB instance or an error in case of failure.
//
// IMPORTANT: Reuses the admin DB's connection pool to avoid creating separate pools for each chain.
// This is efficient since admin only does occasional reads from the web UI.
func (a *App) NewChainDb(ctx context.Context, chainID string) (chainstore.Store, error) {
	if chainDb, ok := a.ChainsDB.Load(chainID); ok {
		// chainDb is already loaded
		return chainDb, nil
	}

	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return nil, fmt.Errorf("invalid chain ID format: %w", parseErr)
	}

	// Reuse admin DB's connection pool instead of creating a new one.
	// Admin only does occasional reads from web UI, no need for separate pools.
	// All queries use "database"."table" format, so one connection pool can serve multiple databases.
	chainDB := chainstore.NewWithSharedClient(a.AdminDB.Client, chainIDUint)

	// Initialize the database and tables if they don't exist yet.
	// This handles the race condition where admin creates the chain before the indexer pod starts.
	// InitializeDB is idempotent and uses CREATE IF NOT EXISTS, so it's safe to call multiple times.
	if err := chainDB.InitializeDB(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize chain database: %w", err)
	}

	a.ChainsDB.Store(chainID, chainDB)

	return chainDB, nil
}

// ReconcileSchedules ensures the required schedules for indexing are created if they do not already exist.
func (a *App) ReconcileSchedules(ctx context.Context) error {
	chains, err := a.AdminDB.ListChain(ctx, false)
	if err != nil {
		return err
	}

	for _, c := range chains {
		chainIDStr := strconv.FormatUint(c.ChainID, 10)
		if chainScheduleErr := a.EnsureChainSchedules(ctx, chainIDStr); chainScheduleErr != nil {
			return chainScheduleErr
		}
	}

	// Ensure a cross-chain compaction schedule
	// TODO: enable again once debugged to see why explodes.
	//if err := a.EnsureCrossChainCompactionSchedule(ctx); err != nil {
	//	return fmt.Errorf("failed to ensure cross-chain compaction schedule: %w", err)
	//}

	return nil
}

// EnsureHeadSchedule ensures the required schedules for indexing are created if they do not already exist.
func (a *App) EnsureHeadSchedule(ctx context.Context, chainID string) error {
	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}
	id := a.TemporalClient.GetHeadScheduleID(chainIDUint)
	h := a.TemporalClient.TSClient.GetHandle(ctx, id)
	_, err := h.Describe(ctx)
	if err == nil {
		a.Logger.Info("Head scan schedule already exists", zap.String("id", id), zap.String("chainID", chainID))
		return nil
	}

	var notFound *serviceerror.NotFound
	if errors.As(err, &notFound) {
		_, scheduleErr := a.TemporalClient.TSClient.Create(
			ctx, client.ScheduleOptions{
				ID:   id,
				Spec: a.TemporalClient.TwoSecondSpec(),
				Action: &client.ScheduleWorkflowAction{
					Workflow:                 indexer.HeadScanWorkflowName,
					Args:                     []interface{}{indexer.HeadScanInput{ChainID: chainIDUint}},
					TaskQueue:                a.TemporalClient.GetIndexerOpsQueue(chainIDUint),
					WorkflowExecutionTimeout: 10 * time.Minute, // Allow up to 10 minutes for HeadScan (direct scheduling can take time)
					WorkflowTaskTimeout:      2 * time.Minute,  // 2-minute task timeout for workflow logic
				},
			},
		)
		return scheduleErr
	}
	return err
}

// EnsureGapScanSchedule ensures the required schedules for indexing are created if they do not already exist.
func (a *App) EnsureGapScanSchedule(ctx context.Context, chainID string) error {
	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}
	id := a.TemporalClient.GetGapScanScheduleID(chainIDUint)
	h := a.TemporalClient.TSClient.GetHandle(ctx, id)
	_, err := h.Describe(ctx)
	if err == nil {
		a.Logger.Info("Gap scan schedule already exists", zap.String("id", id), zap.String("chainID", chainID))
		return nil
	}

	var notFound *serviceerror.NotFound
	if errors.As(err, &notFound) {
		a.Logger.Info("Creating gap scan schedule", zap.String("id", id))
		_, scheduleErr := a.TemporalClient.TSClient.Create(
			ctx,
			client.ScheduleOptions{
				ID:   id,
				Spec: a.TemporalClient.OneHourSpec(),
				Action: &client.ScheduleWorkflowAction{
					Workflow:                 indexer.GapScanWorkflowName,
					Args:                     []interface{}{indexer.GapScanInput{ChainID: chainIDUint}},
					TaskQueue:                a.TemporalClient.GetIndexerOpsQueue(chainIDUint),
					WorkflowExecutionTimeout: 10 * time.Minute, // Allow up to 10 minutes for GapScan (direct scheduling can take time)
					WorkflowTaskTimeout:      2 * time.Minute,  // 2-minute task timeout for workflow logic
				},
			},
		)
		return scheduleErr
	}
	return err
}

// EnsurePollSnapshotSchedule ensures the poll snapshot schedule is created if it does not already exist.
// This schedule runs every 5 minutes to capture governance poll snapshots.
func (a *App) EnsurePollSnapshotSchedule(ctx context.Context, chainID string) error {
	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}
	id := a.TemporalClient.GetPollSnapshotScheduleID(chainIDUint)
	h := a.TemporalClient.TSClient.GetHandle(ctx, id)
	_, err := h.Describe(ctx)
	if err == nil {
		a.Logger.Info("Poll snapshot schedule already exists", zap.String("id", id), zap.String("chainID", chainID))
		return nil
	}

	var notFound *serviceerror.NotFound
	if errors.As(err, &notFound) {
		a.Logger.Info("Creating poll snapshot schedule", zap.String("id", id))
		_, scheduleErr := a.TemporalClient.TSClient.Create(
			ctx,
			client.ScheduleOptions{
				ID:   id,
				Spec: a.TemporalClient.FiveMinuteSpec(),
				Action: &client.ScheduleWorkflowAction{
					Workflow:  indexer.PollSnapshotWorkflowName,
					Args:      []interface{}{}, // No input args needed
					TaskQueue: a.TemporalClient.GetIndexerOpsQueue(chainIDUint),
				},
			},
		)
		return scheduleErr
	}
	return err
}

// EnsureProposalSnapshotSchedule ensures the proposal snapshot schedule is created if it does not already exist.
// This schedule runs every 5 minutes to capture governance proposal snapshots.
func (a *App) EnsureProposalSnapshotSchedule(ctx context.Context, chainID string) error {
	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}
	id := a.TemporalClient.GetProposalSnapshotScheduleID(chainIDUint)
	h := a.TemporalClient.TSClient.GetHandle(ctx, id)
	_, err := h.Describe(ctx)
	if err == nil {
		a.Logger.Info("Proposal snapshot schedule already exists", zap.String("id", id), zap.String("chainID", chainID))
		return nil
	}

	var notFound *serviceerror.NotFound
	if errors.As(err, &notFound) {
		a.Logger.Info("Creating proposal snapshot schedule", zap.String("id", id))
		_, scheduleErr := a.TemporalClient.TSClient.Create(
			ctx,
			client.ScheduleOptions{
				ID:   id,
				Spec: a.TemporalClient.FiveMinuteSpec(),
				Action: &client.ScheduleWorkflowAction{
					Workflow:  indexer.ProposalSnapshotWorkflowName,
					Args:      []interface{}{}, // No input args needed
					TaskQueue: a.TemporalClient.GetIndexerOpsQueue(chainIDUint),
				},
			},
		)
		return scheduleErr
	}
	return err
}

// EnsureChainSchedules ensures the required schedules for indexing are created if they do not already exist.
func (a *App) EnsureChainSchedules(ctx context.Context, chainID string) error {
	if err := a.EnsureHeadSchedule(ctx, chainID); err != nil {
		return err
	}

	if err := a.EnsureGapScanSchedule(ctx, chainID); err != nil {
		return err
	}

	if err := a.EnsurePollSnapshotSchedule(ctx, chainID); err != nil {
		return err
	}

	if err := a.EnsureProposalSnapshotSchedule(ctx, chainID); err != nil {
		return err
	}

	return nil
}

// EnsureCrossChainCompactionSchedule ensures the cross-chain table compaction schedule is created.
func (a *App) EnsureCrossChainCompactionSchedule(ctx context.Context) error {
	id := a.TemporalClient.GetCrossChainCompactionScheduleID()
	h := a.TemporalClient.TSClient.GetHandle(ctx, id)
	_, err := h.Describe(ctx)
	if err == nil {
		a.Logger.Info("Cross-chain compaction schedule already exists", zap.String("id", id))
		return nil
	}

	var notFound *serviceerror.NotFound
	if errors.As(err, &notFound) {
		a.Logger.Info("Creating cross-chain compaction schedule", zap.String("id", id))
		_, scheduleErr := a.TemporalClient.TSClient.Create(
			ctx,
			client.ScheduleOptions{
				ID:   id,
				Spec: a.TemporalClient.OneHourSpec(), // Run every hour
				Action: &client.ScheduleWorkflowAction{
					Workflow:  "CompactCrossChainTablesWorkflow",       // Workflow name from pkg/temporal/admin
					Args:      []interface{}{map[string]interface{}{}}, // Empty input
					TaskQueue: a.TemporalClient.GetAdminMaintenanceQueue(),
				},
			},
		)
		return scheduleErr
	}
	return err
}

// LoadChainStore loads or creates a chain store for the given chain ID.
// Returns the store and a boolean indicating if the chain was loaded successfully.
func (a *App) LoadChainStore(ctx context.Context, chainID string) (chainstore.Store, bool) {
	if store, ok := a.ChainsDB.Load(chainID); ok {
		a.Logger.Debug("Chain store loaded from cache", zap.String("chainID", chainID))
		return store, true
	}

	a.Logger.Info("Chain store not in cache, creating new connection", zap.String("chainID", chainID))

	// Try to create the store if it doesn't exist
	store, err := a.NewChainDb(ctx, chainID)
	if err != nil {
		a.Logger.Error("Failed to create new chain store",
			zap.String("chainID", chainID),
			zap.Error(err),
			zap.String("error_detail", fmt.Sprintf("%+v", err)))
		return nil, false
	}

	a.Logger.Info("Successfully created new chain store", zap.String("chainID", chainID))
	return store, true
}

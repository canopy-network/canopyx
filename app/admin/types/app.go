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

	// Temporal Client Manager (multi-namespace support)
	TemporalManager *temporal.ClientManager

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

	// Close Temporal manager (closes all clients)
	if a.TemporalManager != nil {
		a.Logger.Info("Closing Temporal manager")
		a.TemporalManager.Close()
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

// NewChainDb retrieves or creates a ChainDB instance for a given blockchain identified by chainID.
// The database and tables must already exist (created by indexer at startup).
// Returns the ChainDB instance or an error in case of failure.
//
// IMPORTANT: This does NOT call InitializeDB - that should only happen once at startup in app.go.
// Reuses the admin DB's connection pool to avoid creating separate pools for each chain.
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
// The schedule is created in the chain's namespace (chain_<id>).
func (a *App) EnsureHeadSchedule(ctx context.Context, chainID string) error {
	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}

	// Get chain-specific client
	chainClient, err := a.TemporalManager.GetChainClient(ctx, chainIDUint)
	if err != nil {
		return fmt.Errorf("failed to get chain client: %w", err)
	}

	id := chainClient.HeadScheduleID // "headscan" (simplified)
	h := chainClient.TSClient.GetHandle(ctx, id)
	_, err = h.Describe(ctx)
	if err == nil {
		a.Logger.Info("Head scan schedule already exists",
			zap.String("id", id),
			zap.String("namespace", chainClient.Namespace),
			zap.String("chainID", chainID))
		return nil
	}

	var notFound *serviceerror.NotFound
	if errors.As(err, &notFound) {
		a.Logger.Info("Creating head scan schedule",
			zap.String("id", id),
			zap.String("namespace", chainClient.Namespace))
		_, scheduleErr := chainClient.TSClient.Create(
			ctx, client.ScheduleOptions{
				ID:   id,
				Spec: temporal.TwoSecondSpec(),
				Action: &client.ScheduleWorkflowAction{
					Workflow:                 indexer.HeadScanWorkflowName,
					Args:                     []interface{}{indexer.HeadScanInput{ChainID: chainIDUint}},
					TaskQueue:                chainClient.OpsQueue, // "ops" (simplified)
					WorkflowExecutionTimeout: 10 * time.Minute,
					WorkflowTaskTimeout:      2 * time.Minute,
				},
			},
		)
		return scheduleErr
	}
	return err
}

// EnsureGapScanSchedule ensures the required schedules for indexing are created if they do not already exist.
// The schedule is created in the chain's namespace (chain_<id>).
func (a *App) EnsureGapScanSchedule(ctx context.Context, chainID string) error {
	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}

	// Get chain-specific client
	chainClient, err := a.TemporalManager.GetChainClient(ctx, chainIDUint)
	if err != nil {
		return fmt.Errorf("failed to get chain client: %w", err)
	}

	id := chainClient.GapScanScheduleID // "gapscan" (simplified)
	h := chainClient.TSClient.GetHandle(ctx, id)
	_, err = h.Describe(ctx)
	if err == nil {
		a.Logger.Info("Gap scan schedule already exists",
			zap.String("id", id),
			zap.String("namespace", chainClient.Namespace),
			zap.String("chainID", chainID))
		return nil
	}

	var notFound *serviceerror.NotFound
	if errors.As(err, &notFound) {
		a.Logger.Info("Creating gap scan schedule",
			zap.String("id", id),
			zap.String("namespace", chainClient.Namespace))
		_, scheduleErr := chainClient.TSClient.Create(
			ctx,
			client.ScheduleOptions{
				ID:   id,
				Spec: temporal.OneHourSpec(),
				Action: &client.ScheduleWorkflowAction{
					Workflow:                 indexer.GapScanWorkflowName,
					Args:                     []interface{}{indexer.GapScanInput{ChainID: chainIDUint}},
					TaskQueue:                chainClient.OpsQueue, // "ops" (simplified)
					WorkflowExecutionTimeout: 10 * time.Minute,
					WorkflowTaskTimeout:      2 * time.Minute,
				},
			},
		)
		return scheduleErr
	}
	return err
}

// EnsurePollSnapshotSchedule ensures the poll snapshot schedule is created if it does not already exist.
// This schedule runs every 5 minutes to capture governance poll snapshots.
// The schedule is created in the chain's namespace (chain_<id>).
func (a *App) EnsurePollSnapshotSchedule(ctx context.Context, chainID string) error {
	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}

	// Get chain-specific client
	chainClient, err := a.TemporalManager.GetChainClient(ctx, chainIDUint)
	if err != nil {
		return fmt.Errorf("failed to get chain client: %w", err)
	}

	id := chainClient.PollSnapshotScheduleID // "pollsnapshot" (simplified)
	h := chainClient.TSClient.GetHandle(ctx, id)
	_, err = h.Describe(ctx)
	if err == nil {
		a.Logger.Info("Poll snapshot schedule already exists",
			zap.String("id", id),
			zap.String("namespace", chainClient.Namespace),
			zap.String("chainID", chainID))
		return nil
	}

	var notFound *serviceerror.NotFound
	if errors.As(err, &notFound) {
		a.Logger.Info("Creating poll snapshot schedule",
			zap.String("id", id),
			zap.String("namespace", chainClient.Namespace))
		_, scheduleErr := chainClient.TSClient.Create(
			ctx,
			client.ScheduleOptions{
				ID:   id,
				Spec: temporal.FiveMinuteSpec(),
				Action: &client.ScheduleWorkflowAction{
					Workflow:  indexer.PollSnapshotWorkflowName,
					Args:      []interface{}{}, // No input args needed
					TaskQueue: chainClient.OpsQueue,
				},
			},
		)
		return scheduleErr
	}
	return err
}

// EnsureProposalSnapshotSchedule ensures the proposal snapshot schedule is created if it does not already exist.
// This schedule runs every 5 minutes to capture governance proposal snapshots.
// The schedule is created in the chain's namespace (chain_<id>).
func (a *App) EnsureProposalSnapshotSchedule(ctx context.Context, chainID string) error {
	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}

	// Get chain-specific client
	chainClient, err := a.TemporalManager.GetChainClient(ctx, chainIDUint)
	if err != nil {
		return fmt.Errorf("failed to get chain client: %w", err)
	}

	id := chainClient.ProposalSnapshotScheduleID // "proposalsnapshot" (simplified)
	h := chainClient.TSClient.GetHandle(ctx, id)
	_, err = h.Describe(ctx)
	if err == nil {
		a.Logger.Info("Proposal snapshot schedule already exists",
			zap.String("id", id),
			zap.String("namespace", chainClient.Namespace),
			zap.String("chainID", chainID))
		return nil
	}

	var notFound *serviceerror.NotFound
	if errors.As(err, &notFound) {
		a.Logger.Info("Creating proposal snapshot schedule",
			zap.String("id", id),
			zap.String("namespace", chainClient.Namespace))
		_, scheduleErr := chainClient.TSClient.Create(
			ctx,
			client.ScheduleOptions{
				ID:   id,
				Spec: temporal.FiveMinuteSpec(),
				Action: &client.ScheduleWorkflowAction{
					Workflow:  indexer.ProposalSnapshotWorkflowName,
					Args:      []interface{}{}, // No input args needed
					TaskQueue: chainClient.OpsQueue,
				},
			},
		)
		return scheduleErr
	}
	return err
}

// EnsureCleanupStagingSchedule ensures the cleanup staging schedule is created if it does not already exist.
// This schedule runs hourly to batch cleanup staging tables for heights that have been successfully indexed.
// The schedule is created in the chain's namespace (chain_<id>).
func (a *App) EnsureCleanupStagingSchedule(ctx context.Context, chainID string) error {
	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}

	// Get chain-specific client
	chainClient, err := a.TemporalManager.GetChainClient(ctx, chainIDUint)
	if err != nil {
		return fmt.Errorf("failed to get chain client: %w", err)
	}

	id := chainClient.CleanupStagingScheduleID // "cleanupstaging"
	h := chainClient.TSClient.GetHandle(ctx, id)
	_, err = h.Describe(ctx)
	if err == nil {
		a.Logger.Info("Cleanup staging schedule already exists",
			zap.String("id", id),
			zap.String("namespace", chainClient.Namespace),
			zap.String("chainID", chainID))
		return nil
	}

	var notFound *serviceerror.NotFound
	if errors.As(err, &notFound) {
		a.Logger.Info("Creating cleanup staging schedule",
			zap.String("id", id),
			zap.String("namespace", chainClient.Namespace))
		_, scheduleErr := chainClient.TSClient.Create(
			ctx,
			client.ScheduleOptions{
				ID:   id,
				Spec: temporal.OneHourSpec(),
				Action: &client.ScheduleWorkflowAction{
					Workflow:  indexer.CleanupStagingWorkflowName,
					Args:      []interface{}{}, // Uses defaults: lookbackHours=2, bufferHours=1
					TaskQueue: chainClient.MaintenanceQueue,
				},
			},
		)
		return scheduleErr
	}
	return err
}

// EnsureChainSchedules ensures the required schedules for indexing are created if they do not already exist.
// It first ensures the chain's namespace exists, then creates all schedules in that namespace.
func (a *App) EnsureChainSchedules(ctx context.Context, chainID string) error {
	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}

	// Ensure namespace exists first
	if err := a.TemporalManager.EnsureChainNamespace(ctx, chainIDUint, 7*24*time.Hour); err != nil {
		return fmt.Errorf("failed to ensure chain namespace: %w", err)
	}

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

	if err := a.EnsureCleanupStagingSchedule(ctx, chainID); err != nil {
		return err
	}

	return nil
}

// EnsureCrossChainCompactionSchedule ensures the cross-chain table compaction schedule is created.
// This schedule is created in the admin namespace (canopyx).
func (a *App) EnsureCrossChainCompactionSchedule(ctx context.Context) error {
	// Get admin client
	adminClient, err := a.TemporalManager.GetAdminClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to get admin client: %w", err)
	}

	id := adminClient.CrossChainCompactionScheduleID // "crosschain:compaction"
	h := adminClient.TSClient.GetHandle(ctx, id)
	_, err = h.Describe(ctx)
	if err == nil {
		a.Logger.Info("Cross-chain compaction schedule already exists",
			zap.String("id", id),
			zap.String("namespace", adminClient.Namespace))
		return nil
	}

	var notFound *serviceerror.NotFound
	if errors.As(err, &notFound) {
		a.Logger.Info("Creating cross-chain compaction schedule",
			zap.String("id", id),
			zap.String("namespace", adminClient.Namespace))
		_, scheduleErr := adminClient.TSClient.Create(
			ctx,
			client.ScheduleOptions{
				ID:   id,
				Spec: temporal.OneHourSpec(),
				Action: &client.ScheduleWorkflowAction{
					Workflow:  "CompactCrossChainTablesWorkflow",
					Args:      []interface{}{map[string]interface{}{}},
					TaskQueue: adminClient.MaintenanceQueue, // "maintenance"
				},
			},
		)
		return scheduleErr
	}
	return err
}

// PauseChainSchedules pauses all schedules in a chain's namespace.
// This is called during soft-delete of a chain.
func (a *App) PauseChainSchedules(ctx context.Context, chainID string) error {
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}
	return a.TemporalManager.PauseChainSchedules(ctx, chainIDUint)
}

// UnpauseChainSchedules unpauses all schedules in a chain's namespace.
// This is called during restore of a soft-deleted chain.
func (a *App) UnpauseChainSchedules(ctx context.Context, chainID string) error {
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}
	return a.TemporalManager.UnpauseChainSchedules(ctx, chainIDUint)
}

// DeleteChainSchedules deletes all schedules in a chain's namespace.
// This is called before deleting the namespace itself during hard-delete.
func (a *App) DeleteChainSchedules(ctx context.Context, chainID string) error {
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}
	return a.TemporalManager.DeleteChainSchedules(ctx, chainIDUint)
}

// DeleteChainNamespace deletes a chain's Temporal namespace.
// This is called during hard-delete (permanent deletion) of a chain.
func (a *App) DeleteChainNamespace(ctx context.Context, chainID string) error {
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}
	return a.TemporalManager.DeleteChainNamespace(ctx, chainIDUint)
}

// TerminateRunningWorkflows terminates all running workflows in a chain's namespace.
// This is called during soft-delete to stop any in-progress work.
func (a *App) TerminateRunningWorkflows(ctx context.Context, chainID string, reason string) error {
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}
	return a.TemporalManager.TerminateRunningWorkflows(ctx, chainIDUint, reason)
}

// StartDeleteChainWorkflow starts the hard delete workflow in the admin namespace.
// Returns the workflow ID for tracking.
func (a *App) StartDeleteChainWorkflow(ctx context.Context, chainID uint64) (string, error) {
	adminClient, err := a.TemporalManager.GetAdminClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get admin client: %w", err)
	}

	workflowID, runID, err := adminClient.StartDeleteChainWorkflow(ctx, chainID)
	if err != nil {
		return "", err
	}

	a.Logger.Info("Delete chain workflow started",
		zap.Uint64("chain_id", chainID),
		zap.String("workflow_id", workflowID),
		zap.String("run_id", runID))

	return workflowID, nil
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

package types

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	globalstore "github.com/canopy-network/canopyx/pkg/db/global"
	"go.temporal.io/sdk/worker"

	temporaladmin "github.com/canopy-network/canopyx/pkg/temporal/admin"
	"github.com/canopy-network/canopyx/pkg/temporal/indexer"

	adminstore "github.com/canopy-network/canopyx/pkg/db/admin"
	"github.com/canopy-network/canopyx/pkg/redis"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"github.com/puzpuzpuz/xsync/v4"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
)

type App struct {
	// Database Client wrappers
	AdminDB  adminstore.Store
	GlobalDB globalstore.Store

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

	// Ensure global compaction schedule
	if err := a.EnsureGlobalCompactionSchedule(ctx); err != nil {
		return fmt.Errorf("failed to ensure global compaction schedule: %w", err)
	}

	return nil
}

// EnsureHeadSchedule ensures the required schedules for indexing are created if they do not already exist.
// The schedule is created in the chain's namespace (e.g., "5-a1b2c3").
func (a *App) EnsureHeadSchedule(ctx context.Context, chainID string) error {
	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}

	// Fetch namespace from database
	namespace, err := a.getChainNamespace(ctx, chainIDUint)
	if err != nil {
		return err
	}

	// Get a chain-specific client
	chainClient, err := a.TemporalManager.GetChainClient(ctx, chainIDUint, namespace)
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
// The schedule is created in the chain's namespace (e.g., "5-a1b2c3").
func (a *App) EnsureGapScanSchedule(ctx context.Context, chainID string) error {
	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}

	// Fetch namespace from database
	namespace, err := a.getChainNamespace(ctx, chainIDUint)
	if err != nil {
		return err
	}

	// Get chain-specific client
	chainClient, err := a.TemporalManager.GetChainClient(ctx, chainIDUint, namespace)
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
// The schedule is created in the chain's namespace (e.g., "5-a1b2c3").
func (a *App) EnsurePollSnapshotSchedule(ctx context.Context, chainID string) error {
	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}

	// Fetch namespace from database
	namespace, err := a.getChainNamespace(ctx, chainIDUint)
	if err != nil {
		return err
	}

	// Get chain-specific client
	chainClient, err := a.TemporalManager.GetChainClient(ctx, chainIDUint, namespace)
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
// The schedule is created in the chain's namespace (e.g., "5-a1b2c3").
func (a *App) EnsureProposalSnapshotSchedule(ctx context.Context, chainID string) error {
	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}

	// Fetch namespace from database
	namespace, err := a.getChainNamespace(ctx, chainIDUint)
	if err != nil {
		return err
	}

	// Get chain-specific client
	chainClient, err := a.TemporalManager.GetChainClient(ctx, chainIDUint, namespace)
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

// EnsureChainSchedules ensures the required schedules for indexing are created if they do not already exist.
// It first ensures the chain's namespace exists, then creates all schedules in that namespace.
func (a *App) EnsureChainSchedules(ctx context.Context, chainID string) error {
	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}

	// Fetch namespace from database
	namespace, err := a.getChainNamespace(ctx, chainIDUint)
	if err != nil {
		return err
	}

	// Ensure namespace exists first
	if err := a.TemporalManager.EnsureNamespace(ctx, chainIDUint, namespace, 7*24*time.Hour); err != nil {
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

	return nil
}

// EnsureGlobalCompactionSchedule ensures the global table compaction schedule is created.
// This schedule is created in the admin namespace (canopyx).
func (a *App) EnsureGlobalCompactionSchedule(ctx context.Context) error {
	// Get admin client
	adminClient, err := a.TemporalManager.GetAdminClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to get admin client: %w", err)
	}

	id := adminClient.GlobalCompactionScheduleID // "global:compaction"
	h := adminClient.TSClient.GetHandle(ctx, id)
	_, err = h.Describe(ctx)
	if err == nil {
		a.Logger.Info("Global compaction schedule already exists",
			zap.String("id", id),
			zap.String("namespace", adminClient.Namespace))
		return nil
	}

	var notFound *serviceerror.NotFound
	if errors.As(err, &notFound) {
		a.Logger.Info("Creating global compaction schedule",
			zap.String("id", id),
			zap.String("namespace", adminClient.Namespace))
		_, scheduleErr := adminClient.TSClient.Create(
			ctx,
			client.ScheduleOptions{
				ID:   id,
				Spec: temporal.OneHourSpec(),
				Action: &client.ScheduleWorkflowAction{
					Workflow:  temporaladmin.CompactGlobalTablesWorkflowName,
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
	namespace, err := a.getChainNamespace(ctx, chainIDUint)
	if err != nil {
		return err
	}
	return a.TemporalManager.PauseChainSchedules(ctx, chainIDUint, namespace)
}

// UnpauseChainSchedules unpauses all schedules in a chain's namespace.
// This is called during restore of a soft-deleted chain.
func (a *App) UnpauseChainSchedules(ctx context.Context, chainID string) error {
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}
	namespace, err := a.getChainNamespace(ctx, chainIDUint)
	if err != nil {
		return err
	}
	return a.TemporalManager.UnpauseChainSchedules(ctx, chainIDUint, namespace)
}

// DeleteChainSchedules deletes all schedules in a chain's namespace.
// This is called before deleting the namespace itself during hard-delete.
func (a *App) DeleteChainSchedules(ctx context.Context, chainID string) error {
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}
	namespace, err := a.getChainNamespace(ctx, chainIDUint)
	if err != nil {
		return err
	}
	return a.TemporalManager.DeleteChainSchedules(ctx, chainIDUint, namespace)
}

// DeleteChainNamespace deletes a chain's Temporal namespace.
// This is called during hard-delete (permanent deletion) of a chain.
// Returns the renamed namespace (e.g., "5-a1b2c3-deleted-xyz789") for monitoring cleanup.
func (a *App) DeleteChainNamespace(ctx context.Context, chainID string) (string, error) {
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return "", fmt.Errorf("invalid chain ID format: %w", parseErr)
	}
	namespace, err := a.getChainNamespace(ctx, chainIDUint)
	if err != nil {
		return "", err
	}
	return a.TemporalManager.DeleteNamespace(ctx, chainIDUint, namespace)
}

// TerminateRunningWorkflows terminates all running workflows in a chain's namespace.
// This is called during soft-delete to stop any in-progress work.
func (a *App) TerminateRunningWorkflows(ctx context.Context, chainID string, reason string) error {
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}
	namespace, err := a.getChainNamespace(ctx, chainIDUint)
	if err != nil {
		return err
	}
	return a.TemporalManager.TerminateRunningWorkflows(ctx, chainIDUint, namespace, reason)
}

// StartDeleteChainWorkflow starts the hard delete workflow in the admin namespace.
// renamedNamespace is the Temporal namespace after deletion (for cleanup monitoring).
// Returns the workflow ID for tracking.
func (a *App) StartDeleteChainWorkflow(ctx context.Context, chainID uint64, renamedNamespace string) (string, error) {
	adminClient, err := a.TemporalManager.GetAdminClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get admin client: %w", err)
	}

	workflowID, runID, err := adminClient.StartDeleteChainWorkflow(ctx, chainID, renamedNamespace)
	if err != nil {
		return "", err
	}

	a.Logger.Info("Delete chain workflow started",
		zap.Uint64("chain_id", chainID),
		zap.String("workflow_id", workflowID),
		zap.String("run_id", runID),
		zap.String("renamed_namespace", renamedNamespace))

	return workflowID, nil
}

// GetGlobalDBForChain returns a GlobalDB configured for the specified chain ID.
// This allows admin operations to query the global database with chain context.
func (a *App) GetGlobalDBForChain(chainID uint64) globalstore.Store {
	return globalstore.NewWithSharedClient(a.AdminDB.GetClient(), a.GlobalDB.DatabaseName(), chainID)
}

// getChainNamespace fetches the namespace for a chain from the database.
// Returns the namespace in format "{chain_id}-{namespace_uid}" (e.g., "5-a1b2c3").
func (a *App) getChainNamespace(ctx context.Context, chainID uint64) (string, error) {
	nsInfo, err := a.AdminDB.GetChainNamespaceInfo(ctx, chainID)
	if err != nil {
		return "", fmt.Errorf("failed to get chain namespace info: %w", err)
	}
	nsConfig := temporal.DefaultNamespaceConfig()
	return nsConfig.ChainNamespaceWithUID(chainID, nsInfo.NamespaceUID), nil
}

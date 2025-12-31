package indexer

import (
	"context"
	"strconv"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/activity"
	"github.com/canopy-network/canopyx/app/indexer/workflow"
	adminstore "github.com/canopy-network/canopyx/pkg/db/admin"
	chainstore "github.com/canopy-network/canopyx/pkg/db/chain"
	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	crosschainstore "github.com/canopy-network/canopyx/pkg/db/crosschain"
	"github.com/canopy-network/canopyx/pkg/logging"
	"github.com/canopy-network/canopyx/pkg/redis"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"github.com/canopy-network/canopyx/pkg/temporal/indexer"
	"github.com/canopy-network/canopyx/pkg/utils"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"
	"go.uber.org/zap"
)

const (
	defaultCatchupThreshold        = 200
	defaultDirectScheduleBatchSize = 50
	defaultSchedulerBatchSize      = 250 // Reduced from 5000 to avoid overwhelming Temporal/Cassandra at scale (100+ chains)
	defaultBlockTimeSeconds        = 20

	// Parallelism calculation constants - single source of truth
	// These constants define the resource requirements per block workflow
	// TODO: Tuneup this numbers better base on experience.
	peakConcurrentActivitiesPerBlock = 30  // Actual peak during PromoteData phase (20 stagingEntities)
	connectionsPerActivity           = 2   // One connection per executing activity
	bufferConnections                = 100 // Buffer for ops workflows + parallel cleanup workflows (20 activities × ~10 concurrent cleanups)

	// Default parallel block limits per worker type
	defaultLiveParallelBlocks       = 5  // Live indexing: low latency, small batches
	defaultHistoricalParallelBlocks = 10 // Historical: high throughput, large batches
	defaultReindexParallelBlocks    = 10 // Reindex: high throughput, large batches
)

type App struct {
	LiveWorker       worker.Worker // NEW: Live block indexing (optimized for low-latency)
	HistoricalWorker worker.Worker // NEW: Historical block indexing (optimized for throughput)
	ReindexWorker    worker.Worker // NEW: Reindex block processing (optimized for throughput, dedicated queue)
	OpsWorker        worker.Worker // UNCHANGED: Operations (headscan, gapscan, scheduler)
	TemporalClient   *temporal.Client
	Logger           *zap.Logger

	// Database connections (need to be closed on shutdown)
	IndexerDB    adminstore.Store
	ChainDB      chainstore.Store
	CrossChainDB crosschainstore.Store
	RedisClient  *redis.Client
}

// Start starts the worker and blocks until the context is canceled.
func (a *App) Start(ctx context.Context) {
	if err := a.LiveWorker.Start(); err != nil {
		a.Logger.Fatal("Unable to start live worker", zap.Error(err))
	}
	if err := a.HistoricalWorker.Start(); err != nil {
		a.Logger.Fatal("Unable to start historical worker", zap.Error(err))
	}
	if err := a.ReindexWorker.Start(); err != nil {
		a.Logger.Fatal("Unable to start reindex worker", zap.Error(err))
	}
	if err := a.OpsWorker.Start(); err != nil {
		a.Logger.Fatal("Unable to start operations worker", zap.Error(err))
	}
	<-ctx.Done()
	a.Stop()
}

// Stop stops the worker and closes all database connections.
func (a *App) Stop() {
	a.LiveWorker.Stop()
	a.HistoricalWorker.Stop()
	a.ReindexWorker.Stop()
	a.OpsWorker.Stop()
	time.Sleep(200 * time.Millisecond)

	// Close database connections to prevent connection pool leaks
	if a.IndexerDB != nil {
		if err := a.IndexerDB.Close(); err != nil {
			a.Logger.Error("Failed to close indexer DB connection", zap.Error(err))
		}
	}
	if a.ChainDB != nil {
		if err := a.ChainDB.Close(); err != nil {
			a.Logger.Error("Failed to close chain DB connection", zap.Error(err))
		}
	}
	if a.CrossChainDB != nil {
		if err := a.CrossChainDB.Close(); err != nil {
			a.Logger.Error("Failed to close cross-chain DB connection", zap.Error(err))
		}
	}
	if a.RedisClient != nil {
		if err := a.RedisClient.Close(); err != nil {
			a.Logger.Error("Failed to close Redis connection", zap.Error(err))
		}
	}

	a.Logger.Info("さようなら!")
}

// Initialize initializes the application.
func Initialize(ctx context.Context) *App {
	logger, err := logging.New()
	if err != nil {
		// nothing else to do here, we'll just log to stderr'
		panic(err)
	}

	chainID := utils.Env("CHAIN_ID", "")
	if chainID == "" {
		logger.Fatal("CHAIN_ID environment variable is required")
	}
	// Parse chainID to uint64 for Temporal queue names
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		logger.Fatal("CHAIN_ID must be a valid unsigned integer", zap.String("chain_id", chainID), zap.Error(parseErr))
	}

	// ========================================================================
	// PARALLELISM CALCULATION - Single Source of Truth
	// ========================================================================
	// Everything derives from: How many blocks can we index in parallel?
	// This determines: connection pool size, Temporal limits, etc.

	liveParallelBlocks := utils.EnvInt("LIVE_PARALLEL_BLOCKS", defaultLiveParallelBlocks)
	if liveParallelBlocks <= 0 {
		liveParallelBlocks = defaultLiveParallelBlocks
	}
	historicalParallelBlocks := utils.EnvInt("HISTORICAL_PARALLEL_BLOCKS", defaultHistoricalParallelBlocks)
	if historicalParallelBlocks <= 0 {
		historicalParallelBlocks = defaultHistoricalParallelBlocks
	}
	reindexParallelBlocks := utils.EnvInt("REINDEX_PARALLEL_BLOCKS", defaultReindexParallelBlocks)
	if reindexParallelBlocks <= 0 {
		reindexParallelBlocks = defaultReindexParallelBlocks
	}

	// Calculate Temporal worker limits from parallel blocks
	liveMaxWorkflows := liveParallelBlocks
	liveMaxActivities := liveParallelBlocks * peakConcurrentActivitiesPerBlock

	historicalMaxWorkflows := historicalParallelBlocks
	historicalMaxActivities := historicalParallelBlocks * peakConcurrentActivitiesPerBlock

	reindexMaxWorkflows := reindexParallelBlocks
	reindexMaxActivities := reindexParallelBlocks * peakConcurrentActivitiesPerBlock

	// Calculate required ClickHouse connection pool sizes
	// Formula: (parallel_blocks × peak_concurrent_activities × connections_per_activity) + buffer
	totalParallelBlocks := liveParallelBlocks + historicalParallelBlocks + reindexParallelBlocks
	chainIdleConns := totalParallelBlocks * peakConcurrentActivitiesPerBlock * connectionsPerActivity
	chainMaxConns := chainIdleConns + bufferConnections

	// Configure pool sizes for chain database (high throughput)
	chainPoolConfig := clickhouse.PoolConfig{
		MaxOpenConns:    chainMaxConns,
		MaxIdleConns:    chainIdleConns,
		ConnMaxLifetime: clickhouse.ParseConnMaxLifetime(utils.Env("CLICKHOUSE_CONN_MAX_LIFETIME", "1h")),
		Component:       "indexer_chain",
	}

	// Configure pool sizes for an admin database (low throughput)
	adminPoolConfig := clickhouse.PoolConfig{
		MaxOpenConns:    50,
		MaxIdleConns:    10,
		ConnMaxLifetime: clickhouse.ParseConnMaxLifetime(utils.Env("CLICKHOUSE_CONN_MAX_LIFETIME", "1h")),
		Component:       "indexer_admin",
	}

	// Configure pool sizes for a cross-chain database (medium throughput)
	// Cross-chain writes happen via materialized views when chain data is inserted
	// Estimate: 2 connections per parallel block (less frequent than direct chain writes)
	crosschainIdleConns := totalParallelBlocks * 2
	crosschainMaxConns := crosschainIdleConns + 10 // Small buffer for maintenance operations

	crosschainPoolConfig := clickhouse.PoolConfig{
		MaxOpenConns:    crosschainMaxConns,
		MaxIdleConns:    crosschainIdleConns,
		ConnMaxLifetime: clickhouse.ParseConnMaxLifetime(utils.Env("CLICKHOUSE_CONN_MAX_LIFETIME", "1h")),
		Component:       "indexer_crosschain",
	}

	logger.Info("Parallelism configuration",
		zap.Int("live_parallel_blocks", liveParallelBlocks),
		zap.Int("live_max_workflows", liveMaxWorkflows),
		zap.Int("live_max_activities", liveMaxActivities),
		zap.Int("historical_parallel_blocks", historicalParallelBlocks),
		zap.Int("historical_max_workflows", historicalMaxWorkflows),
		zap.Int("historical_max_activities", historicalMaxActivities),
		zap.Int("reindex_parallel_blocks", reindexParallelBlocks),
		zap.Int("reindex_max_workflows", reindexMaxWorkflows),
		zap.Int("reindex_max_activities", reindexMaxActivities),
		zap.Int("chain_idle_connections", chainIdleConns),
		zap.Int("chain_max_connections", chainMaxConns),
		zap.Int("admin_max_connections", adminPoolConfig.MaxOpenConns),
		zap.Int("crosschain_idle_connections", crosschainIdleConns),
		zap.Int("crosschain_max_connections", crosschainMaxConns),
		zap.Int("peak_concurrent_activities_per_block", peakConcurrentActivitiesPerBlock),
		zap.Int("connections_per_activity", connectionsPerActivity),
	)

	// Initialize databases with calculated pool configurations
	indexerDbName := utils.Env("INDEXER_DB", "canopyx_indexer")
	indexerDb, err := adminstore.NewWithPoolConfig(ctx, logger, indexerDbName, adminPoolConfig)
	if err != nil {
		logger.Fatal("Unable to initialize indexer database", zap.Error(err))
	}

	chainDb, chainDbErr := chainstore.NewWithPoolConfig(ctx, logger, chainIDUint, chainPoolConfig)
	if chainDbErr != nil {
		logger.Fatal("Unable to initialize chain database", zap.Error(chainDbErr))
	}

	crossChainDbName := utils.Env("CROSSCHAIN_DB", "canopyx_cross_chain")
	crossChainDb, crossChainDbErr := crosschainstore.NewWithPoolConfig(ctx, logger, crossChainDbName, crosschainPoolConfig)
	if crossChainDbErr != nil {
		logger.Fatal("Unable to initialize cross-chain database", zap.Error(crossChainDbErr))
	}

	// Set up cross-chain sync now that the chain database is fully initialized
	// This creates materialized views that automatically sync new data to global tables
	if setupErr := crossChainDb.SetupChainSync(ctx, chainIDUint); setupErr != nil {
		// Non-fatal: log warning and continue (manual setup via admin API is still possible)
		logger.Warn("Failed to setup cross-chain sync - cross-chain queries may be incomplete",
			zap.Uint64("chain_id", chainIDUint),
			zap.Error(setupErr),
			zap.String("note", "You can manually trigger sync via admin API"))
	} else {
		logger.Info("Cross-chain sync setup complete", zap.Uint64("chain_id", chainIDUint))
	}

	temporalClient, err := temporal.NewClient(ctx, logger)
	if err != nil {
		logger.Fatal("Unable to establish temporal connection", zap.Error(err))
	}

	if err := temporalClient.EnsureNamespace(ctx, 7*24*time.Hour); err != nil {
		logger.Fatal("Unable to ensure temporal namespace", zap.Error(err))
	}
	logger.Info("Temporal namespace ready", zap.String("namespace", temporalClient.Namespace))

	// Initialize Redis client for real-time event publishing
	redisClient, err := redis.NewClient(ctx, logger)
	if err != nil {
		logger.Fatal("Unable to establish Redis connection", zap.Error(err))
	}

	// RPC rate limiting: Configured for high-throughput parallel block indexing (700k+ blocks)
	// RPS: Requests per second, Burst: Burst capacity for short spikes
	rpcOpts := rpc.Opts{RPS: 500, Burst: 1000, BreakerFailures: 10, BreakerCooldown: 30 * time.Second}
	catchupThreshold := utils.EnvInt("SCHEDULER_CATCHUP_THRESHOLD", defaultCatchupThreshold)
	if catchupThreshold <= 0 {
		catchupThreshold = defaultCatchupThreshold
	}
	directScheduleBatch := utils.EnvInt("DIRECT_SCHEDULE_BATCH_SIZE", defaultDirectScheduleBatchSize)
	if directScheduleBatch <= 0 {
		directScheduleBatch = defaultDirectScheduleBatchSize
	}
	// SCHEDULER_BATCH_SIZE: Number of workflows to schedule per batch operation
	// Default: 500 (balanced to avoid overwhelming Temporal/Cassandra at 100+ chain scale)
	// With 100 chains: 100 × 500 = 50k workflows per batch cycle (manageable for Cassandra)
	// Lower values reduce Cassandra write pressure at cost of more frequent batch operations
	// Activity timeout: 2 minutes in workflow/ops.go
	schedulerBatch := utils.EnvInt("SCHEDULER_BATCH_SIZE", defaultSchedulerBatchSize)
	if schedulerBatch <= 0 {
		schedulerBatch = defaultSchedulerBatchSize
	}
	blockTimeSeconds := utils.EnvInt("BLOCK_TIME_SECONDS", defaultBlockTimeSeconds)
	if blockTimeSeconds <= 0 {
		blockTimeSeconds = defaultBlockTimeSeconds
	}

	activityContext := &activity.Context{
		ChainID:              chainIDUint,
		Logger:               logger,
		AdminDB:              indexerDb,
		ChainDB:              chainDb,
		CrossChainDB:         crossChainDb,
		RPCFactory:           rpc.NewHTTPFactory(rpcOpts),
		RPCOpts:              rpcOpts,
		TemporalClient:       temporalClient,
		RedisClient:          redisClient,
		WorkerMaxParallelism: utils.EnvInt("SCHEDULER_BATCH_MAX_PARALLELISM", 0),
	}

	workflowContext := workflow.Context{
		ChainID:         chainIDUint,
		TemporalClient:  temporalClient,
		ActivityContext: activityContext,
		Config: workflow.Config{
			CatchupThreshold:        uint64(catchupThreshold),
			DirectScheduleBatchSize: uint64(directScheduleBatch),
			SchedulerBatchSize:      uint64(schedulerBatch),
			BlockTimeSeconds:        uint64(blockTimeSeconds),
		},
	}

	// Create Live Worker - optimized for low-latency, high-priority blocks
	liveWorker := worker.New(
		temporalClient.TClient,
		temporalClient.GetIndexerLiveQueue(chainIDUint),
		worker.Options{
			MaxConcurrentWorkflowTaskPollers:       10,
			MaxConcurrentActivityTaskPollers:       10,
			MaxConcurrentWorkflowTaskExecutionSize: liveMaxWorkflows,
			MaxConcurrentActivityExecutionSize:     liveMaxActivities,
			WorkerStopTimeout:                      1 * time.Minute,
		},
	)

	// Create Historical Worker - optimized for high-throughput batch processing
	historicalWorker := worker.New(
		temporalClient.TClient,
		temporalClient.GetIndexerHistoricalQueue(chainIDUint),
		worker.Options{
			MaxConcurrentWorkflowTaskPollers:       50,
			MaxConcurrentActivityTaskPollers:       50,
			MaxConcurrentWorkflowTaskExecutionSize: historicalMaxWorkflows,
			MaxConcurrentActivityExecutionSize:     historicalMaxActivities,
			WorkerStopTimeout:                      1 * time.Minute,
		},
	)

	// Create Reindex Worker - optimized for high-throughput reindex processing
	reindexWorker := worker.New(
		temporalClient.TClient,
		temporalClient.GetIndexerReindexQueue(chainIDUint),
		worker.Options{
			MaxConcurrentWorkflowTaskPollers:       50,
			MaxConcurrentActivityTaskPollers:       50,
			MaxConcurrentWorkflowTaskExecutionSize: reindexMaxWorkflows,
			MaxConcurrentActivityExecutionSize:     reindexMaxActivities,
			WorkerStopTimeout:                      1 * time.Minute,
		},
	)

	// Register IndexBlockWorkflow on live, historical, and reindex workers (same workflow, different queues)
	for _, w := range []worker.Worker{liveWorker, historicalWorker, reindexWorker} {
		w.RegisterWorkflowWithOptions(
			workflowContext.IndexBlockWorkflow,
			temporalworkflow.RegisterOptions{
				Name: indexer.IndexBlockWorkflowName,
			},
		)
	}

	// Register snapshot workflows on live, historical, and reindex workers
	// These were moved from ops queue to avoid congestion from cleanup workflows
	for _, w := range []worker.Worker{liveWorker, historicalWorker, reindexWorker} {
		w.RegisterWorkflowWithOptions(
			workflowContext.PollSnapshotWorkflow,
			temporalworkflow.RegisterOptions{Name: indexer.PollSnapshotWorkflowName},
		)
		w.RegisterWorkflowWithOptions(
			workflowContext.ProposalSnapshotWorkflow,
			temporalworkflow.RegisterOptions{Name: indexer.ProposalSnapshotWorkflowName},
		)
		w.RegisterWorkflowWithOptions(
			workflowContext.LPSnapshotWorkflow,
			temporalworkflow.RegisterOptions{Name: indexer.LPSnapshotWorkflowName},
		)
	}

	// Register all IndexBlock activities on live, historical, and reindex workers
	for _, w := range []worker.Worker{liveWorker, historicalWorker, reindexWorker} {
		w.RegisterActivity(activityContext.PrepareIndexBlock)
		w.RegisterActivity(activityContext.FetchBlockFromRPC)
		w.RegisterActivity(activityContext.SaveBlock)
		w.RegisterActivity(activityContext.IndexTransactions)
		w.RegisterActivity(activityContext.IndexAccounts)
		w.RegisterActivity(activityContext.IndexEvents)
		w.RegisterActivity(activityContext.IndexPools)
		w.RegisterActivity(activityContext.IndexOrders)
		w.RegisterActivity(activityContext.IndexDexPrices)
		w.RegisterActivity(activityContext.IndexParams)
		w.RegisterActivity(activityContext.IndexValidators)
		w.RegisterActivity(activityContext.IndexCommittees)
		w.RegisterActivity(activityContext.IndexDexBatch)
		w.RegisterActivity(activityContext.IndexSupply)
		w.RegisterActivity(activityContext.SaveBlockSummary)
		w.RegisterActivity(activityContext.PromoteData)
		w.RegisterActivity(activityContext.RecordIndexed)
		// Snapshot activities - moved from ops queue to avoid congestion
		w.RegisterActivity(activityContext.IndexPoll)
		w.RegisterActivity(activityContext.IndexProposals)
		w.RegisterActivity(activityContext.ComputeLPSnapshots)
	}

	// Ops worker configuration calculated from total indexing capacity
	// CleanupStagingWorkflow triggers after each indexed block, so peak cleanup concurrency
	// equals total parallel blocks across all workers (live + historical + reindex)
	// Each cleanup workflow uses local activities (runs in-process, no task queue overhead)
	opsMaxCleanupWorkflows := totalParallelBlocks * 2 // 2x buffer for burst scenarios
	opsMaxActivities := 20                            // For HeadScan, GapScan, Scheduler (regular activities)

	logger.Info("Ops worker configuration",
		zap.Int("total_parallel_blocks", totalParallelBlocks),
		zap.Int("ops_max_cleanup_workflows", opsMaxCleanupWorkflows),
		zap.Int("ops_max_activities", opsMaxActivities),
	)

	opsWorker := worker.New(
		temporalClient.TClient,
		temporalClient.GetIndexerOpsQueue(chainIDUint),
		worker.Options{
			// Cleanup workflows use local activities - need high workflow task poller count
			MaxConcurrentWorkflowTaskPollers: opsMaxCleanupWorkflows,
			// Regular activities (HeadScan, GapScan, Scheduler) - moderate count
			MaxConcurrentActivityTaskPollers: opsMaxActivities,
			// Execution size should match or exceed poller count for cleanup workflows
			MaxConcurrentWorkflowTaskExecutionSize: opsMaxCleanupWorkflows,
			MaxConcurrentActivityExecutionSize:     opsMaxActivities * 10,
			WorkerStopTimeout:                      1 * time.Minute,
		},
	)

	opsWorker.RegisterWorkflowWithOptions(
		workflowContext.HeadScan,
		temporalworkflow.RegisterOptions{Name: indexer.HeadScanWorkflowName},
	)
	opsWorker.RegisterWorkflowWithOptions(
		workflowContext.GapScanWorkflow,
		temporalworkflow.RegisterOptions{Name: indexer.GapScanWorkflowName},
	)
	opsWorker.RegisterWorkflowWithOptions(
		workflowContext.SchedulerWorkflow,
		temporalworkflow.RegisterOptions{Name: indexer.SchedulerWorkflowName},
	)
	opsWorker.RegisterWorkflowWithOptions(
		workflowContext.ReindexSchedulerWorkflow,
		temporalworkflow.RegisterOptions{Name: indexer.ReindexSchedulerWorkflowName},
	)
	opsWorker.RegisterWorkflowWithOptions(
		workflowContext.CleanupStagingWorkflow,
		temporalworkflow.RegisterOptions{Name: indexer.CleanupStagingWorkflowName},
	)
	// NOTE: Poll/Proposal/LP snapshot workflows moved to live/historical/reindex workers
	// to avoid congestion from cleanup workflows on ops queue
	opsWorker.RegisterActivity(activityContext.GetLatestHead)
	opsWorker.RegisterActivity(activityContext.GetLastIndexed)
	opsWorker.RegisterActivity(activityContext.FindGaps)
	opsWorker.RegisterActivity(activityContext.StartIndexWorkflow)
	opsWorker.RegisterActivity(activityContext.StartIndexWorkflowBatch)
	opsWorker.RegisterActivity(activityContext.StartReindexWorkflowBatch)
	opsWorker.RegisterActivity(activityContext.IsSchedulerWorkflowRunning)
	opsWorker.RegisterActivity(activityContext.CleanPromotedData)
	opsWorker.RegisterActivity(activityContext.CleanAllPromotedData)
	// NOTE: Poll/Proposal/LP snapshot activities moved to live/historical/reindex workers

	return &App{
		LiveWorker:       liveWorker,
		HistoricalWorker: historicalWorker,
		ReindexWorker:    reindexWorker,
		OpsWorker:        opsWorker,
		TemporalClient:   temporalClient,
		Logger:           logger,
		IndexerDB:        indexerDb,
		ChainDB:          chainDb,
		CrossChainDB:     crossChainDb,
		RedisClient:      redisClient,
	}
}

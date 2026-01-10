package indexer

import (
    "context"
    "strconv"
    "time"

    "github.com/canopy-network/canopyx/app/indexer/activity"
    "github.com/canopy-network/canopyx/app/indexer/workflow"
    adminstore "github.com/canopy-network/canopyx/pkg/db/admin"
    "github.com/canopy-network/canopyx/pkg/db/clickhouse"
    globalstore "github.com/canopy-network/canopyx/pkg/db/global"
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
    defaultCatchupThreshold        = 500
    defaultDirectScheduleBatchSize = 50
    defaultSchedulerBatchSize      = 250 // Reduced from 5000 to avoid overwhelming Temporal/Cassandra at scale (100+ chains)
    defaultBlockTimeSeconds        = 20

    // Parallelism calculation constants - single source of truth
    // These constants define the resource requirements per block workflow
    // TODO: Tuneup this numbers better base on experience.
    // app/indexer/activity/blob.go:125 -> 10 concurrent + 2 individual which could be parallel too + 2 buffer
    peakConcurrentActivitiesPerBlock = 4
    connectionsPerActivity           = 14
    bufferConnections                = 50 // Buffer for ops workflows

    // Default parallel block limits per worker type
    // These match the deployed values in deploy/k8s/controller/base/deployment.yaml
    defaultLiveParallelBlocks       = 2 // Live indexing: low latency, small batches
    defaultHistoricalParallelBlocks = 5 // Historical: high throughput, large batches
    defaultReindexParallelBlocks    = 5 // Reindex: high throughput, large batches
)

type App struct {
    LiveWorker       worker.Worker // Live block indexing (optimized for low-latency)
    HistoricalWorker worker.Worker // Historical block indexing (optimized for throughput)
    ReindexWorker    worker.Worker // Reindex block processing (optimized for throughput, dedicated queue)
    OpsWorker        worker.Worker // Operations (headscan, gapscan, scheduler)
    ChainClient      *temporal.ChainClient
    Logger           *zap.Logger

    // Database connections (need to be closed on shutdown)
    IndexerDB   adminstore.Store
    GlobalDB    globalstore.Store // New single-DB architecture (replaces per-chain DB)
    RedisClient *redis.Client
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
    if a.GlobalDB != nil {
        if err := a.GlobalDB.Close(); err != nil {
            a.Logger.Error("Failed to close global DB connection", zap.Error(err))
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

    // Configure pool sizes for a chain database (high throughput)
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
        zap.Int("peak_concurrent_activities_per_block", peakConcurrentActivitiesPerBlock),
        zap.Int("connections_per_activity", connectionsPerActivity),
    )

    // Initialize databases with calculated pool configurations
    indexerDbName := utils.Env("INDEXER_DB", "canopyx_indexer")
    indexerDb, err := adminstore.NewWithPoolConfig(ctx, logger, indexerDbName, adminPoolConfig)
    if err != nil {
        logger.Fatal("Unable to initialize indexer database", zap.Error(err))
    }

    // Initialize a global database with chain ID context (new single-DB architecture)
    globalDBName := utils.Env("GLOBAL_DB", globalstore.DefaultGlobalDBName)
    globalDb, globalDbErr := globalstore.NewWithPoolConfig(ctx, logger, globalDBName, chainIDUint, chainPoolConfig)
    if globalDbErr != nil {
        logger.Fatal("Unable to initialize global database", zap.Error(globalDbErr))
    }
    logger.Info("Global database initialized",
        zap.String("database", globalDb.Name),
        zap.Uint64("chain_id", chainIDUint))

    // Fetch namespace info from database to get the unique namespace UID
    // This ensures we connect to the correct namespace after chain recreation
    nsInfo, err := indexerDb.GetChainNamespaceInfo(ctx, chainIDUint)
    if err != nil {
        logger.Fatal("Unable to get chain namespace info", zap.Error(err))
    }

    // Build namespace name with UID: "{chain_id}-{namespace_uid}" (e.g., "5-a1b2c3")
    nsConfig := temporal.DefaultNamespaceConfig()
    namespace := nsConfig.ChainNamespaceWithUID(chainIDUint, nsInfo.NamespaceUID)
    logger.Info("Using Temporal namespace",
        zap.Uint64("chain_id", chainIDUint),
        zap.String("namespace_uid", nsInfo.NamespaceUID),
        zap.String("namespace", namespace))

    chainClient, err := temporal.NewChainClientWithNamespace(ctx, logger, chainIDUint, namespace)
    if err != nil {
        logger.Fatal("Unable to establish temporal connection", zap.Error(err))
    }

    if err := chainClient.EnsureNamespace(ctx, 7*24*time.Hour); err != nil {
        logger.Fatal("Unable to ensure temporal namespace", zap.Error(err))
    }
    logger.Info("Temporal namespace ready", zap.String("namespace", chainClient.Namespace))

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
        GlobalDB:             globalDb,
        RPCFactory:           rpc.NewHTTPFactory(rpcOpts),
        RPCOpts:              rpcOpts,
        ChainClient:          chainClient,
        RedisClient:          redisClient,
        WorkerMaxParallelism: utils.EnvInt("SCHEDULER_BATCH_MAX_PARALLELISM", 0),
    }

    workflowContext := workflow.Context{
        ChainID:         chainIDUint,
        ChainClient:     chainClient,
        ActivityContext: activityContext,
        Config: workflow.Config{
            CatchupThreshold:        uint64(catchupThreshold),
            DirectScheduleBatchSize: uint64(directScheduleBatch),
            SchedulerBatchSize:      uint64(schedulerBatch),
            BlockTimeSeconds:        uint64(blockTimeSeconds),
        },
    }

    // Create Live Worker - optimized for low-latency, high-priority blocks
    logger.Info("Creating live worker",
        zap.String("namespace", chainClient.Namespace),
        zap.String("queue", chainClient.LiveQueue),
        zap.Uint64("chain_id", chainIDUint),
    )
    liveWorker := worker.New(
        chainClient.TClient,
        chainClient.LiveQueue,
        worker.Options{
            MaxConcurrentWorkflowTaskPollers:       liveParallelBlocks * 2,
            MaxConcurrentActivityTaskPollers:       peakConcurrentActivitiesPerBlock * 2,
            MaxConcurrentWorkflowTaskExecutionSize: liveMaxWorkflows,
            MaxConcurrentActivityExecutionSize:     liveMaxActivities,
            WorkerStopTimeout:                      1 * time.Minute,
        },
    )

    // Create Historical Worker - optimized for high-throughput batch processing
    logger.Info("Creating historical worker",
        zap.String("namespace", chainClient.Namespace),
        zap.String("queue", chainClient.HistoricalQueue),
        zap.Uint64("chain_id", chainIDUint),
    )
    historicalWorker := worker.New(
        chainClient.TClient,
        chainClient.HistoricalQueue,
        worker.Options{
            MaxConcurrentWorkflowTaskPollers:       historicalMaxWorkflows * 2,
            MaxConcurrentActivityTaskPollers:       peakConcurrentActivitiesPerBlock * 2,
            MaxConcurrentWorkflowTaskExecutionSize: historicalMaxWorkflows,
            MaxConcurrentActivityExecutionSize:     historicalMaxActivities,
            WorkerStopTimeout:                      1 * time.Minute,
        },
    )

    // Create Reindex Worker - optimized for high-throughput reindex processing
    logger.Info("Creating reindex worker",
        zap.String("namespace", chainClient.Namespace),
        zap.String("queue", chainClient.ReindexQueue),
        zap.Uint64("chain_id", chainIDUint),
    )
    reindexWorker := worker.New(
        chainClient.TClient,
        chainClient.ReindexQueue,
        worker.Options{
            MaxConcurrentWorkflowTaskPollers:       reindexMaxWorkflows * 2,
            MaxConcurrentActivityTaskPollers:       peakConcurrentActivitiesPerBlock * 2,
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

    // Register all IndexBlock activities on live, historical, and reindex workers
    for _, w := range []worker.Worker{liveWorker, historicalWorker, reindexWorker} {
        w.RegisterActivity(activityContext.PrepareIndexBlock)
        w.RegisterActivity(activityContext.RecordIndexed)
        w.RegisterActivity(activityContext.IndexBlockFromBlob)
    }

    // Ops worker configuration - optimized for HeadScan, GapScan, Scheduler workflows
    opsMaxWorkflows := 20  // HeadScan, GapScan, Scheduler, Snapshot workflows
    opsMaxActivities := 50 // Activities for HeadScan, GapScan, Scheduler, Snapshots

    logger.Info("Ops worker configuration",
        zap.Int("ops_max_workflows", opsMaxWorkflows),
        zap.Int("ops_max_activities", opsMaxActivities),
    )

    opsWorker := worker.New(
        chainClient.TClient,
        chainClient.OpsQueue,
        worker.Options{
            MaxConcurrentWorkflowTaskPollers:       opsMaxWorkflows,
            MaxConcurrentActivityTaskPollers:       opsMaxActivities,
            MaxConcurrentWorkflowTaskExecutionSize: opsMaxWorkflows,
            MaxConcurrentActivityExecutionSize:     opsMaxActivities,
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
        workflowContext.PollSnapshotWorkflow,
        temporalworkflow.RegisterOptions{Name: indexer.PollSnapshotWorkflowName},
    )
    opsWorker.RegisterWorkflowWithOptions(
        workflowContext.ProposalSnapshotWorkflow,
        temporalworkflow.RegisterOptions{Name: indexer.ProposalSnapshotWorkflowName},
    )
    opsWorker.RegisterWorkflowWithOptions(
        workflowContext.LPSnapshotWorkflow,
        temporalworkflow.RegisterOptions{Name: indexer.LPSnapshotWorkflowName},
    )
    opsWorker.RegisterActivity(activityContext.GetLatestHead)
    opsWorker.RegisterActivity(activityContext.GetLastIndexed)
    opsWorker.RegisterActivity(activityContext.FindGaps)
    opsWorker.RegisterActivity(activityContext.StartIndexWorkflow)
    opsWorker.RegisterActivity(activityContext.StartIndexWorkflowBatch)
    opsWorker.RegisterActivity(activityContext.StartReindexWorkflowBatch)
    opsWorker.RegisterActivity(activityContext.IsSchedulerWorkflowRunning)
    opsWorker.RegisterActivity(activityContext.IndexPoll)
    opsWorker.RegisterActivity(activityContext.IndexProposals)
    opsWorker.RegisterActivity(activityContext.ComputeLPSnapshots)

    return &App{
        LiveWorker:       liveWorker,
        HistoricalWorker: historicalWorker,
        ReindexWorker:    reindexWorker,
        OpsWorker:        opsWorker,
        ChainClient:      chainClient,
        Logger:           logger,
        IndexerDB:        indexerDb,
        GlobalDB:         globalDb,
        RedisClient:      redisClient,
    }
}

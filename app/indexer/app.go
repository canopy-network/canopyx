package indexer

import (
	"context"
	"strconv"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/activity"
	"github.com/canopy-network/canopyx/app/indexer/workflow"
	adminstore "github.com/canopy-network/canopyx/pkg/db/admin"
	chainstore "github.com/canopy-network/canopyx/pkg/db/chain"
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
	defaultSchedulerBatchSize      = 5000 // Increased from 1000 to reduce overhead from frequent batch operations
	defaultBlockTimeSeconds        = 20
)

type App struct {
	LiveWorker       worker.Worker // NEW: Live block indexing (optimized for low-latency)
	HistoricalWorker worker.Worker // NEW: Historical block indexing (optimized for throughput)
	OpsWorker        worker.Worker // UNCHANGED: Operations (headscan, gapscan, scheduler)
	TemporalClient   *temporal.Client
	Logger           *zap.Logger
}

// Start starts the worker and blocks until the context is canceled.
func (a *App) Start(ctx context.Context) {
	if err := a.LiveWorker.Start(); err != nil {
		a.Logger.Fatal("Unable to start live worker", zap.Error(err))
	}
	if err := a.HistoricalWorker.Start(); err != nil {
		a.Logger.Fatal("Unable to start historical worker", zap.Error(err))
	}
	if err := a.OpsWorker.Start(); err != nil {
		a.Logger.Fatal("Unable to start operations worker", zap.Error(err))
	}
	<-ctx.Done()
	a.Stop()
}

// Stop stops the worker.
func (a *App) Stop() {
	a.LiveWorker.Stop()
	a.HistoricalWorker.Stop()
	a.OpsWorker.Stop()
	time.Sleep(200 * time.Millisecond)
	a.Logger.Info("さようなら!")
}

// Initialize initializes the application.
func Initialize(ctx context.Context) *App {
	logger, err := logging.New()
	if err != nil {
		// nothing else to do here, we'll just log to stderr'
		panic(err)
	}

	indexerDbName := utils.Env("INDEXER_DB", "canopyx_indexer")
	indexerDb, err := adminstore.New(ctx, logger, indexerDbName)
	if err != nil {
		logger.Fatal("Unable to initialize indexer database", zap.Error(err))
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

	chainDb, chainDbErr := chainstore.New(ctx, logger, chainIDUint)
	if chainDbErr != nil {
		logger.Fatal("Unable to initialize chain database", zap.Error(chainDbErr))
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
	rpcOpts := rpc.Opts{RPS: 1000, Burst: 2000, BreakerFailures: 10, BreakerCooldown: 30 * time.Second}
	catchupThreshold := utils.EnvInt("SCHEDULER_CATCHUP_THRESHOLD", defaultCatchupThreshold)
	if catchupThreshold <= 0 {
		catchupThreshold = defaultCatchupThreshold
	}
	directScheduleBatch := utils.EnvInt("DIRECT_SCHEDULE_BATCH_SIZE", defaultDirectScheduleBatchSize)
	if directScheduleBatch <= 0 {
		directScheduleBatch = defaultDirectScheduleBatchSize
	}
	// SCHEDULER_BATCH_SIZE: Number of workflows to schedule per batch operation
	// Default: 5000 (optimized to reduce overhead from frequent batch operations)
	// For 750k blocks: 150 batch operations vs 750 with previous default of 1000
	// Higher values (e.g., 10000) can further reduce overhead but require proportionally longer timeouts
	// Consider: Activity timeout is set to 2 minutes in workflow/ops.go
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
			// Lower poller count - live queue should have small backlog
			MaxConcurrentWorkflowTaskPollers: 10,
			MaxConcurrentActivityTaskPollers: 10,
			// Lower execution limits - focus on throughput per block
			MaxConcurrentWorkflowTaskExecutionSize: 100,
			MaxConcurrentActivityExecutionSize:     200,
			WorkerStopTimeout:                      1 * time.Minute,
		},
	)

	// Create Historical Worker - optimized for high-throughput batch processing
	historicalWorker := worker.New(
		temporalClient.TClient,
		temporalClient.GetIndexerHistoricalQueue(chainIDUint),
		worker.Options{
			// Higher poller count for large backlogs
			MaxConcurrentWorkflowTaskPollers: 50,
			MaxConcurrentActivityTaskPollers: 50,
			// High execution limits for parallel processing
			MaxConcurrentWorkflowTaskExecutionSize: 2000,
			MaxConcurrentActivityExecutionSize:     5000,
			WorkerStopTimeout:                      1 * time.Minute,
		},
	)

	// Register IndexBlockWorkflow on BOTH workers (same workflow, different queues)
	liveWorker.RegisterWorkflowWithOptions(
		workflowContext.IndexBlockWorkflow,
		temporalworkflow.RegisterOptions{
			Name: indexer.IndexBlockWorkflowName,
		},
	)
	historicalWorker.RegisterWorkflowWithOptions(
		workflowContext.IndexBlockWorkflow,
		temporalworkflow.RegisterOptions{
			Name: indexer.IndexBlockWorkflowName,
		},
	)

	// Register all IndexBlock activities on BOTH workers
	for _, w := range []worker.Worker{liveWorker, historicalWorker} {
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
		w.RegisterActivity(activityContext.IndexPoll)
		w.RegisterActivity(activityContext.SaveBlockSummary)
		w.RegisterActivity(activityContext.PromoteData)
		w.RegisterActivity(activityContext.RecordIndexed)
	}

	opsWorker := worker.New(
		temporalClient.TClient,
		temporalClient.GetIndexerOpsQueue(chainIDUint),
		worker.Options{
			MaxConcurrentWorkflowTaskPollers: 5,
			MaxConcurrentActivityTaskPollers: 5,
			WorkerStopTimeout:                1 * time.Minute,
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
		workflowContext.CleanupStagingWorkflow,
		temporalworkflow.RegisterOptions{Name: indexer.CleanupStagingWorkflowName},
	)
	opsWorker.RegisterActivity(activityContext.GetLatestHead)
	opsWorker.RegisterActivity(activityContext.GetLastIndexed)
	opsWorker.RegisterActivity(activityContext.FindGaps)
	opsWorker.RegisterActivity(activityContext.StartIndexWorkflow)
	opsWorker.RegisterActivity(activityContext.StartIndexWorkflowBatch)
	opsWorker.RegisterActivity(activityContext.IsSchedulerWorkflowRunning)
	opsWorker.RegisterActivity(activityContext.CleanPromotedData)

	return &App{
		LiveWorker:       liveWorker,
		HistoricalWorker: historicalWorker,
		OpsWorker:        opsWorker,
		TemporalClient:   temporalClient,
		Logger:           logger,
	}
}

package indexer

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/indexer/activity"
	"github.com/canopy-network/canopyx/pkg/indexer/workflow"
	"github.com/canopy-network/canopyx/pkg/logging"
	"github.com/canopy-network/canopyx/pkg/redis"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"github.com/canopy-network/canopyx/pkg/temporal"
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
	LiveWorker       worker.Worker  // NEW: Live block indexing (optimized for low-latency)
	HistoricalWorker worker.Worker  // NEW: Historical block indexing (optimized for throughput)
	OpsWorker        worker.Worker  // UNCHANGED: Operations (headscan, gapscan, scheduler)
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

	indexerDb, _, basicDbsErr := db.NewBasicDbs(ctx, logger)
	if basicDbsErr != nil {
		logger.Fatal("Unable to initialize basic databases", zap.Error(basicDbsErr))
	}

	chainsDb, chainsDbErr := indexerDb.EnsureChainsDbs(ctx)
	if chainsDbErr != nil {
		logger.Fatal("Unable to initialize chains database", zap.Error(chainsDbErr))
	}

	temporalClient, err := temporal.NewClient(ctx, logger)
	if err != nil {
		logger.Fatal("Unable to establish temporal connection", zap.Error(err))
	}

	// Initialize Redis client for real-time event publishing
	redisClient, err := redis.NewClient(ctx, logger)
	if err != nil {
		logger.Fatal("Unable to establish Redis connection", zap.Error(err))
	}

	chainID := utils.Env("CHAIN_ID", "")
	if chainID == "" {
		logger.Fatal("CHAIN_ID environment variable is required")
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
		Logger:                  logger,
		IndexerDB:               indexerDb,
		ChainsDB:                chainsDb,
		RPCFactory:              rpc.NewHTTPFactory(rpcOpts),
		RPCOpts:                 rpcOpts,
		TemporalClient:          temporalClient,
		RedisClient:             redisClient,
		SchedulerMaxParallelism: utils.EnvInt("SCHEDULER_BATCH_MAX_PARALLELISM", 0),
	}
	workflowContext := workflow.Context{
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
		temporalClient.GetIndexerLiveQueue(chainID),
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
		temporalClient.GetIndexerHistoricalQueue(chainID),
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
			Name: workflow.IndexBlockWorkflowName,
		},
	)
	historicalWorker.RegisterWorkflowWithOptions(
		workflowContext.IndexBlockWorkflow,
		temporalworkflow.RegisterOptions{
			Name: workflow.IndexBlockWorkflowName,
		},
	)

	// Register all IndexBlock activities on BOTH workers
	for _, w := range []worker.Worker{liveWorker, historicalWorker} {
		w.RegisterActivity(activityContext.PrepareIndexBlock)
		w.RegisterActivity(activityContext.FetchBlockFromRPC)
		w.RegisterActivity(activityContext.SaveBlock)
		w.RegisterActivity(activityContext.IndexBlock)
		w.RegisterActivity(activityContext.IndexTransactions)
		w.RegisterActivity(activityContext.SaveBlockSummary)
		w.RegisterActivity(activityContext.RecordIndexed)
	}

	opsWorker := worker.New(
		temporalClient.TClient,
		temporalClient.GetIndexerOpsQueue(chainID),
		worker.Options{
			MaxConcurrentWorkflowTaskPollers: 5,
			MaxConcurrentActivityTaskPollers: 5,
			WorkerStopTimeout:                1 * time.Minute,
		},
	)

	opsWorker.RegisterWorkflowWithOptions(
		workflowContext.HeadScan,
		temporalworkflow.RegisterOptions{Name: workflow.HeadScanWorkflowName},
	)
	opsWorker.RegisterWorkflowWithOptions(
		workflowContext.GapScanWorkflow,
		temporalworkflow.RegisterOptions{Name: workflow.GapScanWorkflowName},
	)
	opsWorker.RegisterWorkflowWithOptions(
		workflowContext.SchedulerWorkflow,
		temporalworkflow.RegisterOptions{Name: workflow.SchedulerWorkflowName},
	)
	opsWorker.RegisterActivity(activityContext.GetLatestHead)
	opsWorker.RegisterActivity(activityContext.GetLastIndexed)
	opsWorker.RegisterActivity(activityContext.FindGaps)
	opsWorker.RegisterActivity(activityContext.StartIndexWorkflow)
	opsWorker.RegisterActivity(activityContext.StartIndexWorkflowBatch)
	opsWorker.RegisterActivity(activityContext.IsSchedulerWorkflowRunning)

	return &App{
		LiveWorker:       liveWorker,
		HistoricalWorker: historicalWorker,
		OpsWorker:        opsWorker,
		TemporalClient:   temporalClient,
		Logger:           logger,
	}
}

package indexer

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/indexer/activity"
	"github.com/canopy-network/canopyx/pkg/indexer/workflow"
	"github.com/canopy-network/canopyx/pkg/logging"
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
	Worker         worker.Worker
	OpsWorker      worker.Worker
	TemporalClient *temporal.Client
	Logger         *zap.Logger
}

// Start starts the worker and blocks until the context is canceled.
func (a *App) Start(ctx context.Context) {
	if err := a.Worker.Start(); err != nil {
		a.Logger.Fatal("Unable to start worker", zap.Error(err))
	}
	if err := a.OpsWorker.Start(); err != nil {
		a.Logger.Fatal("Unable to start operations worker", zap.Error(err))
	}
	<-ctx.Done()
	a.Stop()
}

// Stop stops the worker.
func (a *App) Stop() {
	a.Worker.Stop()
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

	// Turn on the temporal worker to listen on chain id task queue (chain-specific workflow/activity)
	// Configured for high-throughput parallel processing of 700k+ blocks
	wkr := worker.New(
		temporalClient.TClient,
		temporalClient.GetIndexerQueue(chainID),
		worker.Options{
			// Pollers: Number of goroutines polling for tasks from Temporal server
			MaxConcurrentWorkflowTaskPollers: 50,
			MaxConcurrentActivityTaskPollers: 50,
			// Executors: Maximum number of concurrent workflow/activity executions
			// High limits to allow massive parallelism when processing large backlogs
			MaxConcurrentWorkflowTaskExecutionSize: 2000, // Allow 2000 concurrent workflow executions
			MaxConcurrentActivityExecutionSize:     5000, // Allow 5000 concurrent activity executions
			WorkerStopTimeout:                      1 * time.Minute,
		},
	)

	// Register the workflow
	wkr.RegisterWorkflowWithOptions(
		workflowContext.IndexBlockWorkflow,
		temporalworkflow.RegisterOptions{
			Name: workflow.IndexBlockWorkflowName,
		},
	)
	// Register all the activities
	wkr.RegisterActivity(activityContext.PrepareIndexBlock)
	wkr.RegisterActivity(activityContext.FetchBlockFromRPC)
	wkr.RegisterActivity(activityContext.SaveBlock)
	wkr.RegisterActivity(activityContext.IndexBlock)
	wkr.RegisterActivity(activityContext.IndexTransactions)
	wkr.RegisterActivity(activityContext.SaveBlockSummary)
	wkr.RegisterActivity(activityContext.RecordIndexed)

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
		Worker:         wkr,
		OpsWorker:      opsWorker,
		TemporalClient: temporalClient,
		Logger:         logger,
	}
}

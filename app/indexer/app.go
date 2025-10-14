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

    rpcOpts := rpc.Opts{RPS: 20, Burst: 40, BreakerFailures: 3, BreakerCooldown: 5 * time.Second}
    activityContext := &activity.Context{
        Logger:         logger,
        IndexerDB:      indexerDb,
        ChainsDB:       chainsDb,
        RPCFactory:     rpc.NewHTTPFactory(rpcOpts),
        RPCOpts:        rpcOpts,
        TemporalClient: temporalClient,
    }
    workflowContext := workflow.Context{
        TemporalClient:  temporalClient,
        ActivityContext: activityContext,
    }

    // Turn on the temporal worker to listen on chain id task queue (chain-specific workflow/activity)
    wkr := worker.New(
        temporalClient.TClient,
        temporalClient.GetIndexerQueue(chainID),
        worker.Options{
            MaxConcurrentWorkflowTaskPollers: 10,
            MaxConcurrentActivityTaskPollers: 10,
            WorkerStopTimeout:                1 * time.Minute,
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
    wkr.RegisterActivity(activityContext.IndexBlock)
    wkr.RegisterActivity(activityContext.IndexTransactions)
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
    opsWorker.RegisterActivity(activityContext.IsSchedulerWorkflowRunning)

    return &App{
        Worker:         wkr,
        OpsWorker:      opsWorker,
        TemporalClient: temporalClient,
        Logger:         logger,
    }
}

package indexer

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/indexer/activity"
	"github.com/canopy-network/canopyx/pkg/indexer/workflow"
	"github.com/canopy-network/canopyx/pkg/logging"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"github.com/canopy-network/canopyx/pkg/utils"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"
	"go.uber.org/zap"
)

type App struct {
	Worker         worker.Worker
	TemporalClient *temporal.Client
	Logger         *zap.Logger
}

// Start starts the worker and blocks until the context is canceled.
func (a *App) Start(ctx context.Context) {
	err := a.Worker.Start()
	if err != nil {
		a.Logger.Fatal("Unable to start worker", zap.Error(err))
	}
	<-ctx.Done()
	a.Stop()
}

// Stop stops the worker.
func (a *App) Stop() {
	a.Worker.Stop()
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

	activityContext := &activity.Context{
		Logger:    logger,
		IndexerDB: indexerDb,
		ChainsDB:  chainsDb,
	}
	workflowContext := workflow.Context{
		TemporalClient:  temporalClient,
		ActivityContext: activityContext,
	}

	// Turn on the temporal worker to listen on chain id task queue (chain specific workflow/activity)
	wkr := worker.New(
		temporalClient.TClient,
		temporalClient.GetIndexerQueue(chainID),
		worker.Options{},
	)

	// Register the workflow
	wkr.RegisterWorkflowWithOptions(
		workflowContext.IndexBlockWorkflow,
		temporalworkflow.RegisterOptions{
			Name: "IndexBlockWorkflow",
		},
	)
	// Register all the activities
	wkr.RegisterActivity(activityContext.IndexBlock)
	wkr.RegisterActivity(activityContext.IndexTransactions)
	wkr.RegisterActivity(activityContext.RecordIndexed)

	return &App{
		Worker:         wkr,
		TemporalClient: temporalClient,
		Logger:         logger,
	}
}

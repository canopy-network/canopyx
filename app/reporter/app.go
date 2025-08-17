package workerreports

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/logging"
	"github.com/canopy-network/canopyx/pkg/reporter/activity"
	"github.com/canopy-network/canopyx/pkg/reporter/workflow"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"go.temporal.io/sdk/worker"
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

	indexerDb, reportDb, basicDbsErr := db.NewBasicDbs(ctx, logger)
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

	activityContext := &activity.Context{
		Logger:         logger,
		IndexerDB:      indexerDb,
		ReportsDB:      reportDb,
		ChainsDB:       chainsDb,
		TemporalClient: temporalClient,
	}
	workflowContext := workflow.Context{
		ActivityContext: activityContext,
	}

	// Turn on the temporal worker to listen on chain id task queue (chain specific workflow/activity)
	wkr := worker.New(
		temporalClient.TClient,
		temporalClient.ReportsQueue,
		worker.Options{},
	)

	// Register the workflow
	wkr.RegisterWorkflow(workflowContext.ComputeTxStatsWorkflow)
	// Register all the activities
	wkr.RegisterActivity(activityContext.ComputeTxsAllChains)

	return &App{
		Worker:         wkr,
		TemporalClient: temporalClient,
		Logger:         logger,
	}
}

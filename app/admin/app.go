package admin

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/app/admin/activity"
	"github.com/canopy-network/canopyx/app/admin/types"
	"github.com/canopy-network/canopyx/app/admin/workflow"
	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/logging"
	reporteractivity "github.com/canopy-network/canopyx/pkg/reporter/activity"
	reporterworkflow "github.com/canopy-network/canopyx/pkg/reporter/workflow"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"go.temporal.io/sdk/worker"
	"go.uber.org/zap"
)

func Initialize(ctx context.Context) *types.App {
	logger, err := logging.New()
	if err != nil {
		// nothing else to do here, we'll just log to stderr'
		panic(err)
	}

	indexerDb, reportsDb, basicDbsErr := db.NewBasicDbs(ctx, logger)
	if basicDbsErr != nil {
		logger.Fatal("Unable to initialize basic databases", zap.Error(basicDbsErr))
	}

	indexerDbInitErr := indexerDb.InitializeDB(ctx)
	if indexerDbInitErr != nil {
		logger.Fatal("Unable to initialize indexer database", zap.Error(indexerDbInitErr))
	}

	reportsDbInitErr := reportsDb.InitializeDB(ctx)
	if reportsDbInitErr != nil {
		logger.Fatal("Unable to initialize reports database", zap.Error(reportsDbInitErr))
	}

	chainsDb, chainsDbErr := indexerDb.EnsureChainsDbs(ctx)
	if chainsDbErr != nil {
		logger.Fatal("Unable to initialize chains database", zap.Error(chainsDbErr))
	}

	temporalClient, err := temporal.NewClient(ctx, logger)
	if err != nil {
		logger.Fatal("Unable to establish temporal connection", zap.Error(err))
	}

	// This will listen to workflows/activities for the ManagerQueue (head, gap, etc.)
	managerTemporalWorker := worker.New(temporalClient.TClient, temporalClient.ManagerQueue, worker.Options{
		MaxConcurrentWorkflowTaskPollers: 10,
		MaxConcurrentActivityTaskPollers: 10,
		WorkerStopTimeout:                1 * time.Minute,
	})

	adminActivityContext := &activity.Context{
		IndexerDB:      indexerDb,
		ReportsDB:      reportsDb,
		ChainsDB:       chainsDb,
		TemporalClient: temporalClient,
	}
	adminWorkflowContext := &workflow.Context{
		ActivityContext: adminActivityContext,
		TemporalClient:  temporalClient,
	}

	// All the manager workflows
	managerTemporalWorker.RegisterWorkflow(adminWorkflowContext.HeadScan)
	managerTemporalWorker.RegisterWorkflow(adminWorkflowContext.GapScanWorkflow)
	// All the manager activities
	managerTemporalWorker.RegisterActivity(adminActivityContext.GetLastIndexed)
	managerTemporalWorker.RegisterActivity(adminActivityContext.GetLatestHead)
	managerTemporalWorker.RegisterActivity(adminActivityContext.StartIndexWorkflow)
	managerTemporalWorker.RegisterActivity(adminActivityContext.FindGaps)

	app := &types.App{
		// Database initialization
		AdminDB:  indexerDb,
		ReportDB: reportsDb,
		ChainsDB: chainsDb,

		// Temporal initialization
		TemporalClient: temporalClient,

		// Logger initialization
		Logger: logger,

		// Context initialization
		AdminWorkflowContext: adminWorkflowContext,
		ReporterWorkflowContext: &reporterworkflow.Context{
			ActivityContext: &reporteractivity.Context{
				IndexerDB:      indexerDb,
				ReportsDB:      reportsDb,
				ChainsDB:       chainsDb,
				TemporalClient: temporalClient,
			},
		},

		// Worker initialization
		Worker: managerTemporalWorker,
	}

	err = app.ReconcileSchedules(ctx)
	if err != nil {
		logger.Fatal("Unable to reconcile schedules", zap.Error(err))
	}

	return app
}

package admin

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/app/admin/activity"
	"github.com/canopy-network/canopyx/app/admin/types"
	"github.com/canopy-network/canopyx/app/admin/workflow"
	adminstore "github.com/canopy-network/canopyx/pkg/db/admin"
	"github.com/canopy-network/canopyx/pkg/db/crosschain"
	"github.com/canopy-network/canopyx/pkg/logging"
	"github.com/canopy-network/canopyx/pkg/redis"
	"github.com/canopy-network/canopyx/pkg/temporal"
	temporaladmin "github.com/canopy-network/canopyx/pkg/temporal/admin"
	"github.com/canopy-network/canopyx/pkg/utils"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"
	"go.uber.org/zap"
)

func Initialize(ctx context.Context) *types.App {
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

	chainsDb, chainsDbErr := indexerDb.EnsureChainsDbs(ctx)
	if chainsDbErr != nil {
		logger.Fatal("Unable to initialize chains database", zap.Error(chainsDbErr))
	}

	temporalClient, err := temporal.NewClient(ctx, logger)
	if err != nil {
		logger.Fatal("Unable to establish temporal connection", zap.Error(err))
	}

	// Ensure the Temporal namespace exists (Helm chart doesn't auto-create it)
	// Use 7-day retention to match the Helm values configuration
	err = temporalClient.EnsureNamespace(ctx, 7*24*time.Hour)
	if err != nil {
		logger.Fatal("Unable to ensure temporal namespace", zap.Error(err))
	}
	logger.Info("Temporal namespace ready", zap.String("namespace", temporalClient.Namespace))

	// Initialize Redis client for real-time WebSocket events (optional)
	var redisClient *redis.Client
	if utils.Env("REDIS_ENABLED", "false") == "true" {
		redisClient, err = redis.NewClient(ctx, logger)
		if err != nil {
			logger.Warn("Failed to initialize Redis client - WebSocket real-time events will be disabled",
				zap.Error(err))
			redisClient = nil
		} else {
			logger.Info("Redis client initialized for WebSocket real-time events")
		}
	} else {
		logger.Info("Redis disabled - WebSocket real-time events will not be available")
	}

	app := &types.App{
		// Database initialization
		AdminDB:  indexerDb,
		ChainsDB: chainsDb,

		// Temporal initialization
		TemporalClient: temporalClient,

		// Redis initialization
		RedisClient: redisClient,

		// Logger initialization
		Logger: logger,

		// Queue stats cache initialization (30s TTL to reduce Temporal API rate limiting)
		QueueStatsCache: types.NewQueueStatsCache(),
	}

	// Initialize cross-chain database (required)
	crossChainDBName := utils.Env("CROSSCHAIN_DB", "canopyx_cross_chain")
	crossChainDB, crossChainErr := crosschain.NewStore(ctx, logger, crossChainDBName)
	if crossChainErr != nil {
		logger.Fatal("Cross-chain database initialization failed",
			zap.Error(crossChainErr))
	}

	// Initialize schema (create global tables if they don't exist)
	// This is idempotent - safe to call on every startup
	if schemaErr := crossChainDB.InitializeSchema(ctx); schemaErr != nil {
		logger.Fatal("Cross-chain schema initialization failed",
			zap.Error(schemaErr))
	}

	app.CrossChainDB = crossChainDB
	logger.Info("Cross-chain database initialized successfully",
		zap.String("database", crossChainDB.Name))

	// Set up materialized views for all existing chains
	// This is idempotent - SetupChainSync uses CREATE IF NOT EXISTS for MVs
	chains, listErr := app.AdminDB.ListChain(ctx)
	if listErr != nil {
		logger.Fatal("Failed to list chains for cross-chain sync setup",
			zap.Error(listErr))
	}

	for _, chain := range chains {
		if syncErr := crossChainDB.SetupChainSync(ctx, chain.ChainID); syncErr != nil {
			logger.Fatal("Failed to setup cross-chain sync for chain",
				zap.Uint64("chain_id", chain.ChainID),
				zap.Error(syncErr))
		}
		logger.Info("Cross-chain sync setup complete for chain",
			zap.Uint64("chain_id", chain.ChainID))
	}

	// Initialize Temporal maintenance worker for cross-chain compaction
	activityContext := &activity.Context{
		Logger:         logger,
		CrossChainDB:   crossChainDB,
		TemporalClient: temporalClient,
	}
	workflowContext := workflow.Context{
		TemporalClient:  temporalClient,
		ActivityContext: activityContext,
	}

	// Create a maintenance worker
	maintenanceWorker := worker.New(
		temporalClient.TClient,
		temporalClient.GetAdminMaintenanceQueue(),
		worker.Options{
			MaxConcurrentWorkflowTaskPollers:       5,
			MaxConcurrentActivityTaskPollers:       5,
			MaxConcurrentWorkflowTaskExecutionSize: 10,
			MaxConcurrentActivityExecutionSize:     10,
			WorkerStopTimeout:                      1 * time.Minute,
		},
	)

	// Register a compaction workflow
	maintenanceWorker.RegisterWorkflowWithOptions(
		workflowContext.CompactCrossChainTablesWorkflow,
		temporalworkflow.RegisterOptions{
			Name: temporaladmin.CompactCrossChainTablesWorkflowName,
		},
	)

	// Register compaction activities
	maintenanceWorker.RegisterActivity(activityContext.CompactGlobalTable)
	maintenanceWorker.RegisterActivity(activityContext.CompactAllGlobalTables)
	maintenanceWorker.RegisterActivity(activityContext.LogCompactionSummary)

	app.MaintenanceWorker = maintenanceWorker
	logger.Info("Maintenance worker initialized successfully")

	err = app.ReconcileSchedules(ctx)
	if err != nil {
		logger.Fatal("Unable to reconcile schedules", zap.Error(err))
	}

	return app
}

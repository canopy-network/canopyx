package admin

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/app/admin/activity"
	"github.com/canopy-network/canopyx/app/admin/types"
	"github.com/canopy-network/canopyx/app/admin/workflow"
	adminstore "github.com/canopy-network/canopyx/pkg/db/admin"
	chainstore "github.com/canopy-network/canopyx/pkg/db/chain"
	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	"github.com/canopy-network/canopyx/pkg/db/crosschain"
	"github.com/canopy-network/canopyx/pkg/logging"
	"github.com/canopy-network/canopyx/pkg/redis"
	"github.com/canopy-network/canopyx/pkg/temporal"
	temporaladmin "github.com/canopy-network/canopyx/pkg/temporal/admin"
	"github.com/canopy-network/canopyx/pkg/utils"
	"github.com/puzpuzpuz/xsync/v4"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"
	"go.uber.org/zap"
)

// ========================================================================
// PARALLELISM CALCULATION - Admin Component
// ========================================================================
// Admin has fixed, low parallelism:
// - Maintenance worker: 10 concurrent activities max
// - Web UI queries: occasional, low volume
// Formula: (max_activities × connections_per_activity) + buffer

const (
	maintenanceMaxActivities = 10 // Maintenance worker concurrent activities
	connectionsPerActivity   = 2  // Max ClickHouse connections per activity
	bufferConnections        = 10 // Buffer for web UI queries
)

func Initialize(ctx context.Context) *types.App {
	logger, err := logging.New()
	if err != nil {
		// nothing else to do here, we'll just log to stderr'
		panic(err)
	}

	// Calculate required connection pool sizes
	adminIdleConns := maintenanceMaxActivities * connectionsPerActivity
	adminMaxConns := adminIdleConns + bufferConnections

	// Configure pool sizes for an admin database (low throughput)
	// This single pool is shared by admin DB, crosschain DB, and per-chain DBs
	adminPoolConfig := clickhouse.PoolConfig{
		MaxOpenConns:    adminMaxConns,
		MaxIdleConns:    adminIdleConns,
		ConnMaxLifetime: clickhouse.ParseConnMaxLifetime(utils.Env("CLICKHOUSE_CONN_MAX_LIFETIME", "1h")),
		Component:       "admin",
	}

	logger.Info("Admin parallelism configuration",
		zap.Int("maintenance_max_activities", maintenanceMaxActivities),
		zap.Int("connections_per_activity", connectionsPerActivity),
		zap.Int("total_max_connections", adminMaxConns),
		zap.String("note", "shared by admin, crosschain, and per-chain DBs"),
	)

	indexerDbName := utils.Env("INDEXER_DB", "canopyx_indexer")

	indexerDb, err := adminstore.NewWithPoolConfig(ctx, logger, indexerDbName, adminPoolConfig)
	if err != nil {
		logger.Fatal("Unable to initialize indexer database", zap.Error(err))
	}

	// Initialize a cross-chain database (required)
	// Reuse admin DB's connection pool - crosschain ops are part of the same 10 concurrent activities
	crossChainDBName := utils.Env("CROSSCHAIN_DB", "canopyx_cross_chain")
	crossChainDB := crosschain.NewWithSharedClient(indexerDb.Client, crossChainDBName)

	// Initialize a database and tables (create if they don't exist)
	// This is idempotent - safe to call on every startup
	if dbErr := crossChainDB.InitializeDB(ctx); dbErr != nil {
		logger.Fatal("Cross-chain database initialization failed",
			zap.Error(dbErr))
	}

	logger.Info("Cross-chain database initialized successfully",
		zap.String("database", crossChainDB.Name))

	// Create multi-namespace Temporal client manager
	temporalManager, err := temporal.NewClientManager(ctx, logger)
	if err != nil {
		logger.Fatal("Unable to create temporal manager", zap.Error(err))
	}

	// Get admin client and ensure namespace exists
	adminClient, err := temporalManager.GetAdminClient(ctx)
	if err != nil {
		logger.Fatal("Unable to get admin temporal client", zap.Error(err))
	}

	err = adminClient.EnsureNamespace(ctx, 7*24*time.Hour)
	if err != nil {
		logger.Fatal("Unable to ensure admin temporal namespace", zap.Error(err))
	}
	logger.Info("Admin Temporal namespace ready", zap.String("namespace", adminClient.Namespace))

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
		AdminDB:      indexerDb,
		CrossChainDB: crossChainDB,
		ChainsDB:     xsync.NewMap[string, chainstore.Store](), // Initialize empty chain DB cache

		// Temporal initialization (multi-namespace support)
		TemporalManager: temporalManager,

		// Redis initialization
		RedisClient: redisClient,

		// Logger initialization
		Logger: logger,

		// Queue stats cache initialization (30s TTL to reduce Temporal API rate limiting)
		QueueStatsCache: types.NewQueueStatsCache(),
	}

	logger.Info("Admin app initialized",
		zap.Bool("chains_db_initialized", app.ChainsDB != nil),
		zap.Bool("admin_db_initialized", app.AdminDB != nil))

	// Set up materialized views for all existing chains in parallel
	chains, listErr := app.AdminDB.ListChain(ctx, false)
	if listErr != nil {
		logger.Warn("Failed to list chains for cross-chain sync setup, sync will be retried later",
			zap.Error(listErr))
	} else if len(chains) > 0 {
		for _, chain := range chains {
			if err := crossChainDB.SetupChainSync(ctx, chain.ChainID); err != nil {
				logger.Fatal("Failed to setup cross-chain sync for chain", zap.Error(err))
			}
		}
	}

	// Initialize Temporal maintenance worker for cross-chain compaction
	activityContext := &activity.Context{
		Logger:          logger,
		AdminDB:         indexerDb,
		CrossChainDB:    crossChainDB,
		TemporalManager: temporalManager,
	}
	workflowContext := workflow.Context{
		TemporalManager: temporalManager,
		ActivityContext: activityContext,
	}

	// Create a maintenance worker on admin namespace
	maintenanceWorker := worker.New(
		adminClient.TClient,
		adminClient.MaintenanceQueue, // "maintenance" (simplified)
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

	// Register delete chain workflow
	maintenanceWorker.RegisterWorkflowWithOptions(
		workflowContext.DeleteChainWorkflow,
		temporalworkflow.RegisterOptions{
			Name: temporaladmin.DeleteChainWorkflowName,
		},
	)

	// Register compaction activities
	maintenanceWorker.RegisterActivity(activityContext.CompactGlobalTable)
	maintenanceWorker.RegisterActivity(activityContext.CompactAllGlobalTables)
	maintenanceWorker.RegisterActivity(activityContext.LogCompactionSummary)

	// Register delete chain activities
	maintenanceWorker.RegisterActivity(activityContext.CleanCrossChainData)
	maintenanceWorker.RegisterActivity(activityContext.DropChainDatabase)
	maintenanceWorker.RegisterActivity(activityContext.CleanAdminTables)

	app.MaintenanceWorker = maintenanceWorker
	logger.Info("Maintenance worker initialized successfully")

	err = app.ReconcileSchedules(ctx)
	if err != nil {
		logger.Fatal("Unable to reconcile schedules", zap.Error(err))
	}

	return app
}

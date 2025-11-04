package admin

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/app/admin/types"
	adminstore "github.com/canopy-network/canopyx/pkg/db/admin"
	"github.com/canopy-network/canopyx/pkg/logging"
	"github.com/canopy-network/canopyx/pkg/redis"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"github.com/canopy-network/canopyx/pkg/utils"
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
	// Use 7 days retention to match the Helm values configuration
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

	err = app.ReconcileSchedules(ctx)
	if err != nil {
		logger.Fatal("Unable to reconcile schedules", zap.Error(err))
	}

	return app
}

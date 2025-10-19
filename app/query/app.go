package query

import (
	"context"

	"github.com/canopy-network/canopyx/app/query/types"
	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/logging"
	"github.com/canopy-network/canopyx/pkg/redis"
	"github.com/canopy-network/canopyx/pkg/utils"
	"go.uber.org/zap"
)

// Initialize initializes the application.
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

	// Initialize admin database tables (chains, index_progress, etc.)
	if err := indexerDb.InitializeDB(ctx); err != nil {
		logger.Fatal("Unable to initialize admin database tables", zap.Error(err))
	}

	chainsDb, chainsDbErr := indexerDb.EnsureChainsDbs(ctx)
	if chainsDbErr != nil {
		logger.Fatal("Unable to initialize chains database", zap.Error(chainsDbErr))
	}

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
		IndexerDB:   indexerDb,
		ReportDB:    reportsDb,
		ChainsDB:    chainsDb,
		RedisClient: redisClient,
		Logger:      logger,
	}

	return app
}

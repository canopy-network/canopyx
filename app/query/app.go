package query

import (
	"context"

	"github.com/canopy-network/canopyx/app/query/types"
	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/logging"
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

	chainsDb, chainsDbErr := indexerDb.EnsureChainsDbs(ctx)
	if chainsDbErr != nil {
		logger.Fatal("Unable to initialize chains database", zap.Error(chainsDbErr))
	}

	app := &types.App{
		IndexerDB: indexerDb,
		ReportDB:  reportsDb,
		ChainsDB:  chainsDb,
		Logger:    logger,
	}

	return app
}

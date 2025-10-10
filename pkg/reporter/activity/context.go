package activity

import (
	"context"

	"go.uber.org/zap"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"github.com/puzpuzpuz/xsync/v4"
)

type Context struct {
	Logger         *zap.Logger
	IndexerDB      db.AdminStore
	ReportsDB      db.ReportsStore
	ChainsDB       *xsync.Map[string, db.ChainStore]
	TemporalClient *temporal.Client
}

// NewChainDb returns a chain store for the provided chain ID.
func (c *Context) NewChainDb(ctx context.Context, chainID string) (db.ChainStore, error) {
	if chainDb, ok := c.ChainsDB.Load(chainID); ok {
		// chainDb is already loaded
		return chainDb, nil
	}

	chainDb, chainDbErr := db.NewChainDb(ctx, c.Logger, chainID)
	if chainDbErr != nil {
		return nil, chainDbErr
	}

	c.ChainsDB.Store(chainID, chainDb)

	return chainDb, nil
}

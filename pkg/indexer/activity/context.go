package activity

import (
	"context"

	"go.uber.org/zap"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/puzpuzpuz/xsync/v4"
)

type Context struct {
	Logger    *zap.Logger
	IndexerDB *db.AdminDB
	ChainsDB  *xsync.Map[string, *db.ChainDB]
}

// NewChainDb returns a new ChainDB instance for the provided chain ID.
func (c *Context) NewChainDb(ctx context.Context, chainID string) (*db.ChainDB, error) {
	if chainDb, ok := c.ChainsDB.Load(chainID); ok {
		// chainDb is already loaded
		return chainDb, nil
	}

	chainDB, chainDBErr := db.NewChainDb(ctx, c.Logger, chainID)
	if chainDBErr != nil {
		return nil, chainDBErr
	}

	c.ChainsDB.Store(chainID, chainDB)

	return chainDB, nil
}

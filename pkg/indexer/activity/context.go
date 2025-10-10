package activity

import (
	"context"

	"go.uber.org/zap"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/rpc"
	temporalclient "github.com/canopy-network/canopyx/pkg/temporal"
	"github.com/puzpuzpuz/xsync/v4"
)

type Context struct {
	Logger         *zap.Logger
	IndexerDB      db.AdminStore
	ChainsDB       *xsync.Map[string, db.ChainStore]
	RPCFactory     rpc.Factory
	RPCOpts        rpc.Opts
	TemporalClient *temporalclient.Client
}

// NewChainDb returns a chain store instance for the provided chain ID.
func (c *Context) NewChainDb(ctx context.Context, chainID string) (db.ChainStore, error) {
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

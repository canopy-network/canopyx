package activity

import (
	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"github.com/puzpuzpuz/xsync/v4"
)

type Context struct {
	IndexerDB      *db.AdminDB
	ReportsDB      *db.ReportsDB
	ChainsDB       *xsync.Map[string, *db.ChainDB]
	TemporalClient *temporal.Client
}

//func (c *Context) NewChainDb(ctx context.Context, chainID string) (*db.ChainDB, error) {
//    if chainDb, ok := c.ChainsDB.Load(chainID); ok {
//        // chainDb is already loaded
//        return chainDb, nil
//    }
//
//    chainDB := db.NewChainDB(c.DBClient, chainID)
//    chainDBErr := chainDB.InitializeDB(ctx)
//    if chainDBErr != nil {
//        return nil, chainDBErr
//    }
//
//    c.ChainsDB.Store(chainID, chainDB)
//
//    return chainDB, nil
//}

package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"go.uber.org/zap"
)

// EnsureGenesisCached ensures that the genesis state is cached in the database.
// This activity is executed BEFORE indexing height 1, as height 1 requires comparing
// RPC(1) vs Genesis(0) to detect initial account changes.
//
// Workflow:
// 1. Check if genesis already cached (query genesis table for height=0)
// 2. If cached: return early (idempotent)
// 3. If not cached: fetch from RPC and store as JSON
//
// The genesis data is stored in the "genesis" table as a JSON string for easy retrieval.
func (c *Context) EnsureGenesisCached(ctx context.Context, input types.EnsureGenesisCachedInput) error {
	start := time.Now()

	// Get chain metadata
	ch, err := c.IndexerDB.GetChain(ctx, input.ChainID)
	if err != nil {
		return err
	}

	// Get chain database
	chainDb, err := c.NewChainDb(ctx, input.ChainID)
	if err != nil {
		return err
	}

	// Need to cast to *db.ChainDB to access HasGenesis and InsertGenesis methods
	dbImpl, ok := chainDb.(*db.ChainDB)
	if !ok {
		return fmt.Errorf("chainDb is not *db.ChainDB")
	}

	// Check if genesis already cached
	hasGenesis, err := dbImpl.HasGenesis(ctx, 0)
	if err != nil {
		return fmt.Errorf("check genesis exists: %w", err)
	}

	if hasGenesis {
		c.Logger.Info("Genesis already cached, skipping",
			zap.String("chainId", input.ChainID),
			zap.Float64("durationMs", float64(time.Since(start).Microseconds())/1000.0))
		return nil
	}

	// Fetch genesis from RPC
	c.Logger.Info("Fetching genesis state from RPC",
		zap.String("chainId", input.ChainID))

	cli := c.rpcClient(ch.RPCEndpoints)
	genesis, err := cli.GetGenesisState(ctx, 0)
	if err != nil {
		return err
	}

	// Marshal to JSON for storage
	genesisJSON, err := json.Marshal(genesis)
	if err != nil {
		return err
	}

	// Store in genesis table
	err = dbImpl.InsertGenesis(ctx, 0, string(genesisJSON), time.Now())
	if err != nil {
		return fmt.Errorf("insert genesis: %w", err)
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	c.Logger.Info("Genesis cached successfully",
		zap.String("chainId", input.ChainID),
		zap.Int("numAccounts", len(genesis.Accounts)),
		zap.Float64("durationMs", durationMs))

	return nil
}

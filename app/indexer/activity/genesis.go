package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
func (ac *Context) EnsureGenesisCached(ctx context.Context) error {
	start := time.Now()

	// Get chain database
	chainDb, err := ac.GetChainDb(ctx, ac.ChainID)
	if err != nil {
		return err
	}

	// Check if genesis already cached
	hasGenesis, err := chainDb.HasGenesis(ctx, 0)
	if err != nil {
		return fmt.Errorf("check genesis exists: %w", err)
	}

	if hasGenesis {
		ac.Logger.Info("Genesis already cached, skipping",
			zap.Uint64("chainId", ac.ChainID),
			zap.Float64("durationMs", float64(time.Since(start).Microseconds())/1000.0))
		return nil
	}

	// Fetch genesis from RPC
	ac.Logger.Info("Fetching genesis state from RPC",
		zap.Uint64("chainId", ac.ChainID))

	cli, cliErr := ac.rpcClient(ctx)
	if cliErr != nil {
		return cliErr
	}
	genesis, err := cli.StateByHeight(ctx, 0)
	if err != nil {
		return err
	}

	// Marshal to JSON for storage
	genesisJSON, err := json.Marshal(genesis)
	if err != nil {
		return err
	}

	// Store in the genesis table
	err = chainDb.InsertGenesis(ctx, 0, string(genesisJSON), time.Now())
	if err != nil {
		return fmt.Errorf("insert genesis: %w", err)
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	ac.Logger.Info("Genesis cached successfully",
		zap.Uint64("chainId", ac.ChainID),
		zap.Int("numAccounts", len(genesis.Accounts)),
		zap.Float64("durationMs", durationMs))

	return nil
}

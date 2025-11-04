package activity

import (
	"context"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// IndexDexPoolPoints indexes DEX pool points by holder for a given block height.
//
// Core Algorithm:
// 1. Fetch all pools from RPC using PoolsByHeight() endpoint
// 2. For each pool, iterate through its PoolPoints array
// 3. For each holder in PoolPoints, create a DexPoolPointsByHolder snapshot
// 4. Calculate derived fields (liquidity_pool_points, liquidity_pool_id)
// 5. Insert all snapshots to staging table in batch
//
// Snapshot Strategy:
// This implementation creates snapshots at every height (not snapshot-on-change) because:
// - The RPC PoolsByHeight() endpoint returns current chain head state, not historical state by height
// - Unlike AccountsByHeight(height), there is no PoolsByHeight(height) available
// - Future optimization: Add RPC support for historical pool queries to enable snapshot-on-change
//
// Performance Considerations:
// - Can create many rows if pools have many point holders (LPs)
// - Batch insertion used for efficiency
// - Logs number of holders indexed for monitoring
//
// Created Height Tracking:
// Holder creation heights are tracked via the dex_pool_points_created_height materialized view,
// which automatically calculates MIN(height) for each (address, pool_id) pair.
func (c *Context) IndexDexPoolPoints(ctx context.Context, in types.IndexDexPoolPointsInput) (types.IndexDexPoolPointsOutput, error) {
	start := time.Now()

	// Get chain configuration
	ch, err := c.AdminDB.GetChain(ctx, in.ChainID)
	if err != nil {
		return types.IndexDexPoolPointsOutput{}, err
	}

	// Acquire (or ping) the chain DB to validate it exists
	chainDb, chainDbErr := c.NewChainDb(ctx, in.ChainID)
	if chainDbErr != nil {
		return types.IndexDexPoolPointsOutput{}, temporal.NewApplicationErrorWithCause(
			"unable to acquire chain database",
			"chain_db_error",
			chainDbErr,
		)
	}

	// Fetch all pools from RPC
	// Note: This returns pools at the current chain head, not at the specific height
	// Future improvement: Add height-based pool queries to RPC for true snapshot-on-change
	cli := c.rpcClient(ch.RPCEndpoints)
	rpcPools, err := cli.PoolsByHeight(ctx, in.Height)
	if err != nil {
		return types.IndexDexPoolPointsOutput{}, fmt.Errorf("fetch pools: %w", err)
	}

	// Flatten pool points into individual holder snapshots
	holders := make([]*indexer.DexPoolPointsByHolder, 0)
	for _, rpcPool := range rpcPools {
		// Skip pools with no points or empty points array
		if len(rpcPool.Points) == 0 || rpcPool.TotalPoolPoints == 0 {
			continue
		}

		// Calculate pool IDs using the same logic as Pool.CalculatePoolIDs()
		liquidityPoolID := indexer.LiquidityPoolAddend + rpcPool.ChainID

		// Iterate through each holder's points
		for _, pointEntry := range rpcPool.Points {
			// Skip holders with zero points
			if pointEntry.Points == 0 {
				continue
			}

			// Calculate holder's share of liquidity pool
			// Formula: pool.Amount * holder.Points / pool.TotalPoints
			var liquidityPoolPoints uint64
			if rpcPool.TotalPoolPoints > 0 {
				// Use uint64 arithmetic to avoid overflow
				// This calculates the proportional share of the pool amount
				liquidityPoolPoints = (rpcPool.Amount * pointEntry.Points) / rpcPool.TotalPoolPoints
			}

			holder := &indexer.DexPoolPointsByHolder{
				Address:             pointEntry.Address,
				PoolID:              rpcPool.ID,
				Height:              in.Height,
				HeightTime:          in.BlockTime,
				Committee:           rpcPool.ChainID, // Committee is the pool's chain ID
				Points:              pointEntry.Points,
				LiquidityPoolPoints: liquidityPoolPoints,
				LiquidityPoolID:     liquidityPoolID,
			}
			holders = append(holders, holder)
		}
	}

	numHolders := uint32(len(holders))

	c.Logger.Debug("IndexDexPoolPoints extracted from pools",
		zap.Uint64("height", in.Height),
		zap.Int("numPools", len(rpcPools)),
		zap.Uint32("numHolders", numHolders))

	// Insert pool points holders to staging table (two-phase commit pattern)
	if len(holders) > 0 {
		if err := chainDb.InsertDexPoolPointsByHolderStaging(ctx, holders); err != nil {
			return types.IndexDexPoolPointsOutput{}, fmt.Errorf("insert pool points staging: %w", err)
		}
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.IndexDexPoolPointsOutput{
		NumHolders: numHolders,
		DurationMs: durationMs,
	}, nil
}

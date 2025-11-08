package activity

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/canopy-network/canopyx/pkg/rpc"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/db/transform"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// IndexPools indexes pools for a given block height.
// Fetches all pools from RPC, converts to database models, and inserts to staging table.
// Returns output containing the number of indexed pools and execution duration in milliseconds.
func (ac *Context) IndexPools(ctx context.Context, in types.ActivityIndexAtHeight) (types.ActivityIndexPoolsOutput, error) {
	start := time.Now()

	// Get chain configuration
	cli, err := ac.rpcClient(ctx)
	if err != nil {
		return types.ActivityIndexPoolsOutput{}, err
	}

	// Acquire (or ping) the chain DB to validate it exists
	chainDb, chainDbErr := ac.GetChainDb(ctx, ac.ChainID)
	if chainDbErr != nil {
		return types.ActivityIndexPoolsOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	// Phase 1: Parallel RPC workers - fetch all data concurrently
	var (
		rpcPools      []*rpc.RpcPool
		previousPools []*rpc.RpcPool
		poolEvents    []*indexer.Event
		rpcPoolsErr   error
		prevPoolsErr  error
		eventsErr     error
		wg            sync.WaitGroup
	)

	wg.Add(3)

	// Worker 1: Fetch current pools at H
	go func() {
		defer wg.Done()
		rpcPools, rpcPoolsErr = cli.PoolsByHeight(ctx, in.Height)
	}()

	// Worker 2: Fetch previous pools at H-1 for snapshot-on-change
	go func() {
		defer wg.Done()
		if in.Height == 1 {
			previousPools = make([]*rpc.RpcPool, 0)
			return
		}
		previousPools, prevPoolsErr = cli.PoolsByHeight(ctx, in.Height-1)
	}()

	// Worker 3: Query pool events from staging table (event-driven correlation)
	// These events provide context for pool state changes:
	// - EventDexLiquidityDeposit: LP adds liquidity
	// - EventDexLiquidityWithdraw: LP removes liquidity
	// - EventDexSwap: Swap executes affecting pool balances
	go func() {
		defer wg.Done()
		poolEvents, eventsErr = chainDb.GetEventsByTypeAndHeight(
			ctx, in.Height, true,
			rpc.EventTypeAsStr(rpc.EventTypeDexLiquidityDeposit),
			rpc.EventTypeAsStr(rpc.EventTypeDexLiquidityWithdraw),
			rpc.EventTypeAsStr(rpc.EventTypeDexSwap),
		)
	}()

	// Wait for all workers to complete
	wg.Wait()

	// Check all worker errors
	if rpcPoolsErr != nil {
		return types.ActivityIndexPoolsOutput{}, fmt.Errorf("fetch current pools at height %d: %w", in.Height, rpcPoolsErr)
	}
	if prevPoolsErr != nil {
		return types.ActivityIndexPoolsOutput{}, fmt.Errorf("fetch previous pools at height %d: %w", in.Height-1, prevPoolsErr)
	}
	if eventsErr != nil {
		return types.ActivityIndexPoolsOutput{}, fmt.Errorf("query pool events at height %d: %w", in.Height, eventsErr)
	}

	// Phase 2: Process pools (snapshot pattern)
	// Convert RPC pools to database models
	// Each pool has its own chain_id (from the blockchain's nested chain structure)
	// We index all pools at each height (snapshot pattern)
	pools := make([]*indexer.Pool, 0, len(rpcPools))
	for _, rpcPool := range rpcPools {
		pool := transform.Pool(rpcPool, in.Height)
		pool.HeightTime = in.BlockTime
		// Calculate the derived pool ID fields based on ChainID
		pool.CalculatePoolIDs()
		pools = append(pools, pool)
	}

	numPools := uint32(len(pools))
	numDeposits := 0
	numWithdrawals := 0
	numSwaps := 0

	// Count event types for metrics
	for _, event := range poolEvents {
		switch event.EventType {
		case rpc.EventTypeAsStr(rpc.EventTypeDexLiquidityDeposit):
			numDeposits++
		case rpc.EventTypeAsStr(rpc.EventTypeDexLiquidityWithdraw):
			numWithdrawals++
		case rpc.EventTypeAsStr(rpc.EventTypeDexSwap):
			numSwaps++
		}
	}

	ac.Logger.Info("Indexed pools",
		zap.Uint64("chainId", ac.ChainID),
		zap.Uint64("height", in.Height),
		zap.Uint32("numPools", numPools),
		zap.Int("depositEvents", numDeposits),
		zap.Int("withdrawalEvents", numWithdrawals),
		zap.Int("swapEvents", numSwaps))

	// Insert pools to staging table (two-phase commit pattern)
	if err := chainDb.InsertPoolsStaging(ctx, pools); err != nil {
		return types.ActivityIndexPoolsOutput{}, err
	}

	// Phase 3: Process pool holders (snapshot-on-change)
	// Build previous holder map for O(1) lookups
	prevHolderMap := make(map[string]uint64) // key: "poolID:address" -> points
	for _, pool := range previousPools {
		for _, pointEntry := range pool.Points {
			key := fmt.Sprintf("%d:%s", pool.ID, pointEntry.Address)
			prevHolderMap[key] = pointEntry.Points
		}
	}

	// Compare and create snapshots only for changed holders
	changedHolders := make([]*indexer.PoolPointsByHolder, 0)
	for _, rpcPool := range rpcPools {
		if len(rpcPool.Points) == 0 || rpcPool.TotalPoolPoints == 0 {
			continue
		}

		liquidityPoolID := indexer.LiquidityPoolAddend + rpcPool.ChainID

		for _, pointEntry := range rpcPool.Points {
			if pointEntry.Points == 0 {
				continue
			}

			key := fmt.Sprintf("%d:%s", rpcPool.ID, pointEntry.Address)
			prevPoints, existed := prevHolderMap[key]

			// Snapshot-on-change: only insert if points changed or holder is new
			if !existed || pointEntry.Points != prevPoints {
				var liquidityPoolPoints uint64
				if rpcPool.TotalPoolPoints > 0 {
					liquidityPoolPoints = (rpcPool.Amount * pointEntry.Points) / rpcPool.TotalPoolPoints
				}

				holder := &indexer.PoolPointsByHolder{
					Address:             pointEntry.Address,
					PoolID:              rpcPool.ID,
					Height:              in.Height,
					HeightTime:          in.BlockTime,
					Committee:           rpcPool.ChainID,
					Points:              pointEntry.Points,
					LiquidityPoolPoints: liquidityPoolPoints,
					LiquidityPoolID:     liquidityPoolID,
				}
				changedHolders = append(changedHolders, holder)
			}
		}
	}

	// Insert changed holders to staging table
	if len(changedHolders) > 0 {
		if err := chainDb.InsertPoolPointsByHolderStaging(ctx, changedHolders); err != nil {
			return types.ActivityIndexPoolsOutput{}, err
		}
	}

	ac.Logger.Info("Indexed pool point holders (snapshot-on-change)",
		zap.Uint64("chainId", ac.ChainID),
		zap.Uint64("height", in.Height),
		zap.Int("changedHolders", len(changedHolders)))

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.ActivityIndexPoolsOutput{
		NumPools:       numPools,
		NumPoolHolders: uint32(len(changedHolders)),
		DurationMs:     durationMs,
	}, nil
}

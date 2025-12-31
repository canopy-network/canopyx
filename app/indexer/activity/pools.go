package activity

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/canopy-network/canopy/fsm"
	"github.com/canopy-network/canopy/lib"
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

	// Get RPC client with height-aware endpoint selection
	cli, err := ac.rpcClientForHeight(ctx, in.Height)
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
		rpcPools      []*fsm.Pool
		previousPools []*fsm.Pool
		poolEvents    []*indexer.Event
		rpcPoolsErr   error
		prevPoolsErr  error
		eventsErr     error
	)

	// Get a subgroup from the shared worker pool for parallel RPC fetching
	pool := ac.WorkerPool(3)
	group := pool.NewGroupContext(ctx)
	groupCtx := group.Context()

	// Worker 1: Fetch current pools at H
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		rpcPools, rpcPoolsErr = cli.PoolsByHeight(groupCtx, in.Height)
	})

	// Worker 2: Fetch previous pools at H-1 for snapshot-on-change
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		if in.Height == 1 {
			previousPools = make([]*fsm.Pool, 0)
			return
		}
		previousPools, prevPoolsErr = cli.PoolsByHeight(groupCtx, in.Height-1)
	})

	// Worker 3: Query pool events from staging table (event-driven correlation)
	// These events provide context for pool state changes:
	// - EventDexLiquidityDeposit: LP adds liquidity
	// - EventDexLiquidityWithdraw: LP removes liquidity
	// - EventDexSwap: Swap executes affecting pool balances
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		poolEvents, eventsErr = chainDb.GetEventsByTypeAndHeight(
			groupCtx, in.Height, true,
			string(lib.EventTypeDexLiquidityDeposit),
			string(lib.EventTypeDexLiquidityWithdraw),
			string(lib.EventTypeDexSwap),
		)
	})

	// Wait for all workers to complete
	if err := group.Wait(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, pond.ErrGroupStopped) {
		ac.Logger.Warn("parallel RPC fetch encountered error",
			zap.Uint64("chainId", ac.ChainID),
			zap.Uint64("height", in.Height),
			zap.Error(err),
		)
	}

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

	// Build previous pools map for H-1 delta calculation
	prevPoolsMap := make(map[uint64]*indexer.Pool, len(previousPools))
	for _, prevPool := range previousPools {
		dbPool := transform.Pool(prevPool, in.Height-1)
		prevPoolsMap[dbPool.PoolID] = dbPool
	}

	// Convert RPC pools to database models and calculate H-1 deltas
	// Each pool has its own chain_id (from the blockchain's nested chain structure)
	// We index all pools at each height (snapshot pattern)
	pools := make([]*indexer.Pool, 0, len(rpcPools))
	var numPoolsNew uint32

	for _, rpcPool := range rpcPools {
		pool := transform.Pool(rpcPool, in.Height)
		pool.HeightTime = in.BlockTime
		// Calculate the derived pool ID fields based on ChainID
		pool.CalculatePoolIDs()

		// Calculate H-1 deltas by comparing with previous pool state
		if prevPool, exists := prevPoolsMap[pool.PoolID]; exists {
			pool.AmountDelta = int64(pool.Amount) - int64(prevPool.Amount)
			pool.TotalPointsDelta = int64(pool.TotalPoints) - int64(prevPool.TotalPoints)
			pool.LPCountDelta = int32(pool.LPCount) - int32(prevPool.LPCount)
		} else {
			// No previous state (new pool or height 1), deltas are zero
			pool.AmountDelta = 0
			pool.TotalPointsDelta = 0
			pool.LPCountDelta = 0
			numPoolsNew++
		}

		pools = append(pools, pool)
	}

	numPools := uint32(len(pools))
	numDeposits := 0
	numWithdrawals := 0
	numSwaps := 0

	// Count event types for metrics
	for _, event := range poolEvents {
		switch event.EventType {
		case string(lib.EventTypeDexLiquidityDeposit):
			numDeposits++
		case string(lib.EventTypeDexLiquidityWithdraw):
			numWithdrawals++
		case string(lib.EventTypeDexSwap):
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

	// Phase 3: Process pool holders (snapshot-on-change with TotalPoolPoints tracking)
	// Build previous holder map for O(1) lookups
	prevHolderMap := make(map[string]uint64)       // key: "poolID:address" -> points
	prevPoolTotalPoints := make(map[uint64]uint64) // key: poolID -> TotalPoolPoints at H-1

	for _, pool := range previousPools {
		prevPoolTotalPoints[pool.Id] = pool.TotalPoolPoints
		for _, pointEntry := range pool.Points {
			// Convert protobuf []byte address to hex string
			address := hex.EncodeToString(pointEntry.Address)
			key := fmt.Sprintf("%d:%s", pool.Id, address)
			prevHolderMap[key] = pointEntry.Points
		}
	}

	// Compare and create snapshots for changed holders
	// Also re-snapshot ALL holders if pool's TotalPoolPoints changed
	changedHolders := make([]*indexer.PoolPointsByHolder, 0)
	var numPoolHoldersNew uint32

	for _, rpcPool := range rpcPools {
		if len(rpcPool.Points) == 0 {
			continue
		}

		// Extract ChainID from PoolID (fsm.Pool doesn't have separate ChainID field)
		chainID := indexer.ExtractChainIDFromPoolID(rpcPool.Id)
		liquidityPoolID := indexer.LiquidityPoolAddend + chainID

		// Check if pool's TotalPoolPoints changed
		// If it changed, we need to re-snapshot ALL holders of this pool
		prevTotalPoints, poolExisted := prevPoolTotalPoints[rpcPool.Id]
		poolTotalPointsChanged := !poolExisted || rpcPool.TotalPoolPoints != prevTotalPoints

		for _, pointEntry := range rpcPool.Points {
			// Convert protobuf []byte address to hex string for database storage
			address := hex.EncodeToString(pointEntry.Address)
			key := fmt.Sprintf("%d:%s", rpcPool.Id, address)
			prevPoints, holderExisted := prevHolderMap[key]

			// Snapshot conditions:
			// 1. Holder is new (!holderExisted)
			// 2. Holder's points changed (pointEntry.Points != prevPoints)
			// 3. Pool's TotalPoolPoints changed (poolTotalPointsChanged)
			shouldSnapshot := !holderExisted || pointEntry.Points != prevPoints || poolTotalPointsChanged

			if shouldSnapshot {
				// Track new holders
				if !holderExisted {
					numPoolHoldersNew++
				}

				holder := &indexer.PoolPointsByHolder{
					Address:             address,
					PoolID:              rpcPool.Id,
					Height:              in.Height,
					HeightTime:          in.BlockTime,
					Committee:           chainID,
					Points:              pointEntry.Points,
					LiquidityPoolPoints: rpcPool.TotalPoolPoints, // Store pool's total points
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
		NumPools:          numPools,
		NumPoolsNew:       numPoolsNew,
		NumPoolHolders:    uint32(len(changedHolders)),
		NumPoolHoldersNew: numPoolHoldersNew,
		DurationMs:        durationMs,
	}, nil
}

package activity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/canopy-network/canopyx/app/indexer/types"
	indexer "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"go.uber.org/zap"
)

// IndexSupply indexes token supply metrics for a given block using the snapshot-on-change pattern.
//
// Core Algorithm:
// 1. Parallel RPC fetch: Fetch Supply(H) and Supply(H-1) simultaneously
// 2. Special case: If H=1, assume genesis supply (no previous state)
// 3. Compare: Detect if Total, Staked, or DelegatedOnly changed
// 4. Insert to staging: Only if supply metrics changed
//
// Performance:
// - Parallel RPC fetching reduces latency by ~50% (2 concurrent requests)
// - Only stores changed supply snapshots (sparse storage)
// - No events correlation needed (supply is derived from chain state)
func (ac *Context) IndexSupply(ctx context.Context, input types.ActivityIndexAtHeight) (types.ActivityIndexSupplyOutput, error) {
	start := time.Now()

	cli, err := ac.rpcClient(ctx)
	if err != nil {
		return types.ActivityIndexSupplyOutput{}, err
	}

	// Get chain database
	chainDb, err := ac.GetChainDb(ctx, ac.ChainID)
	if err != nil {
		return types.ActivityIndexSupplyOutput{}, err
	}

	// Parallel RPC fetch using shared worker pool
	var (
		currentSupply  *indexer.Supply
		previousSupply *indexer.Supply
		currentErr     error
		previousErr    error
	)

	// Get a subgroup from the shared worker pool for parallel RPC fetching
	pool := ac.WorkerPool(2)
	group := pool.NewGroupContext(ctx)
	groupCtx := group.Context()

	// Worker 1: Fetch current height supply
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		rpcSupply, err := cli.SupplyByHeight(groupCtx, input.Height)
		if err != nil {
			currentErr = err
			return
		}
		currentSupply = &indexer.Supply{
			Total:         rpcSupply.Total,
			Staked:        rpcSupply.Staked,
			DelegatedOnly: rpcSupply.DelegatedOnly,
			Height:        input.Height,
			HeightTime:    input.BlockTime,
		}
	})

	// Worker 2: Fetch previous height supply (H-1)
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		if input.Height == 1 {
			// Genesis case: no previous supply
			previousSupply = nil
			return
		}

		rpcSupply, err := cli.SupplyByHeight(groupCtx, input.Height-1)
		if err != nil {
			previousErr = err
			return
		}
		previousSupply = &indexer.Supply{
			Total:         rpcSupply.Total,
			Staked:        rpcSupply.Staked,
			DelegatedOnly: rpcSupply.DelegatedOnly,
		}
	})

	// Wait for all workers to complete
	if err := group.Wait(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, pond.ErrGroupStopped) {
		ac.Logger.Warn("parallel RPC fetch encountered error",
			zap.Uint64("chainId", ac.ChainID),
			zap.Uint64("height", input.Height),
			zap.Error(err),
		)
	}

	// Check for errors - all RPC calls must succeed
	if currentErr != nil {
		return types.ActivityIndexSupplyOutput{}, fmt.Errorf("fetch supply at height %d: %w", input.Height, currentErr)
	}
	if previousErr != nil && input.Height > 1 {
		return types.ActivityIndexSupplyOutput{}, fmt.Errorf("fetch supply at height %d: %w", input.Height-1, previousErr)
	}

	// Detect changes (snapshot-on-change)
	changed := false
	if previousSupply == nil {
		// Genesis: always insert
		changed = true
	} else if currentSupply.Total != previousSupply.Total ||
		currentSupply.Staked != previousSupply.Staked ||
		currentSupply.DelegatedOnly != previousSupply.DelegatedOnly {
		changed = true
	}

	// Only insert if supply changed
	if changed {
		if err := chainDb.InsertSupplyStaging(ctx, []*indexer.Supply{currentSupply}); err != nil {
			return types.ActivityIndexSupplyOutput{}, fmt.Errorf("insert supply staging: %w", err)
		}
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	ac.Logger.Info("Indexed supply",
		zap.Uint64("chainId", ac.ChainID),
		zap.Uint64("height", input.Height),
		zap.Bool("changed", changed),
		zap.Uint64("total", currentSupply.Total),
		zap.Uint64("staked", currentSupply.Staked),
		zap.Uint64("delegatedOnly", currentSupply.DelegatedOnly),
		zap.Float64("durationMs", durationMs))

	return types.ActivityIndexSupplyOutput{
		Changed:    changed,
		DurationMs: durationMs,
	}, nil
}

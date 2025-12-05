package activity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/canopy-network/canopyx/app/indexer/types"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/db/transform"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// IndexDexPrices indexes DEX price information for a given block.
// Returns output containing the number of indexed price records and execution duration in milliseconds.
func (ac *Context) IndexDexPrices(ctx context.Context, in types.ActivityIndexAtHeight) (types.ActivityIndexDexPricesOutput, error) {
	start := time.Now()

	// Get RPC client with height-aware endpoint selection
	cli, err := ac.rpcClientForHeight(ctx, in.Height)
	if err != nil {
		return types.ActivityIndexDexPricesOutput{}, err
	}

	// Acquire (or ping) the chain DB just to validate it exists.
	chainDb, chainDbErr := ac.GetChainDb(ctx, ac.ChainID)
	if chainDbErr != nil {
		return types.ActivityIndexDexPricesOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	// Parallel RPC fetch using shared worker pool for performance
	var (
		rpcPricesH   []*rpc.RpcDexPrice
		rpcPricesH1  []*rpc.RpcDexPrice
		rpcPricesErr error
		prevErr      error
	)

	// Get a subgroup from the shared worker pool for parallel RPC fetching
	pool := ac.WorkerPool(2)
	group := pool.NewGroupContext(ctx)
	groupCtx := group.Context()

	// Worker 1: Fetch DEX prices at height H
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		rpcPricesH, rpcPricesErr = cli.DexPricesByHeight(groupCtx, in.Height)
	})

	// Worker 2: Fetch DEX prices at height H-1 for delta calculation
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		if in.Height <= 1 {
			rpcPricesH1 = make([]*rpc.RpcDexPrice, 0)
			return
		}
		rpcPricesH1, prevErr = cli.DexPricesByHeight(groupCtx, in.Height-1)
	})

	// Wait for all workers to complete
	if err := group.Wait(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, pond.ErrGroupStopped) {
		ac.Logger.Warn("parallel RPC fetch encountered error",
			zap.Uint64("chainId", ac.ChainID),
			zap.Uint64("height", in.Height),
			zap.Error(err),
		)
	}

	// Check for errors
	if rpcPricesErr != nil {
		return types.ActivityIndexDexPricesOutput{}, fmt.Errorf("fetch DEX prices at height %d: %w", in.Height, rpcPricesErr)
	}
	if prevErr != nil {
		return types.ActivityIndexDexPricesOutput{}, fmt.Errorf("fetch previous DEX prices at height %d: %w", in.Height-1, prevErr)
	}

	// Build previous prices map for O(1) lookups
	prevPricesMap := make(map[string]*indexer.DexPrice, len(rpcPricesH1))
	for _, price := range rpcPricesH1 {
		dbPrice := transform.DexPrice(price)
		key := transform.DexPriceKey(dbPrice.LocalChainID, dbPrice.RemoteChainID)
		prevPricesMap[key] = dbPrice
	}

	// Convert RPC types to database models and calculate H-1 deltas
	dbPrices := make([]*indexer.DexPrice, 0, len(rpcPricesH))
	for _, rpcPrice := range rpcPricesH {
		dbPrice := transform.DexPrice(rpcPrice)
		dbPrice.Height = in.Height
		dbPrice.HeightTime = in.BlockTime

		// Calculate deltas by comparing with H-1
		key := transform.DexPriceKey(dbPrice.LocalChainID, dbPrice.RemoteChainID)
		if priceH1, exists := prevPricesMap[key]; exists {
			dbPrice.PriceDelta = int64(dbPrice.PriceE6) - int64(priceH1.PriceE6)
			dbPrice.LocalPoolDelta = int64(dbPrice.LocalPool) - int64(priceH1.LocalPool)
			dbPrice.RemotePoolDelta = int64(dbPrice.RemotePool) - int64(priceH1.RemotePool)
		} else {
			// No H-1 data (new pool or height 1), deltas are zero
			dbPrice.PriceDelta = 0
			dbPrice.LocalPoolDelta = 0
			dbPrice.RemotePoolDelta = 0
		}

		dbPrices = append(dbPrices, dbPrice)
	}

	numPrices := uint32(len(dbPrices))
	ac.Logger.Debug("IndexDexPrices fetched from RPC",
		zap.Uint64("height", in.Height),
		zap.Uint32("numPrices", numPrices))

	// Insert DEX prices to staging table (two-phase commit pattern)
	if err := chainDb.InsertDexPricesStaging(ctx, dbPrices); err != nil {
		return types.ActivityIndexDexPricesOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.ActivityIndexDexPricesOutput{
		NumPrices:  numPrices,
		DurationMs: durationMs,
	}, nil
}

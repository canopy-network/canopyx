package activity

import (
	"context"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/db/transform"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// IndexDexPrices indexes DEX price information for a given block.
// Returns output containing the number of indexed price records and execution duration in milliseconds.
func (ac *Context) IndexDexPrices(ctx context.Context, in types.ActivityIndexAtHeight) (types.ActivityIndexDexPricesOutput, error) {
	start := time.Now()

	cli, err := ac.rpcClient(ctx)
	if err != nil {
		return types.ActivityIndexDexPricesOutput{}, err
	}

	// Acquire (or ping) the chain DB just to validate it exists.
	chainDb, chainDbErr := ac.GetChainDb(ctx, ac.ChainID)
	if chainDbErr != nil {
		return types.ActivityIndexDexPricesOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	// Fetch DEX prices from RPC at height H
	rpcPricesH, err := cli.DexPricesByHeight(ctx, in.Height)
	if err != nil {
		return types.ActivityIndexDexPricesOutput{}, err
	}

	// Fetch DEX prices from RPC at height H-1 for delta calculation
	// (using RPC(H) vs RPC(H-1) pattern - no database query needed)
	var rpcPricesH1 map[string]*indexer.DexPrice
	if in.Height > 1 {
		rpcH1, err := cli.DexPricesByHeight(ctx, in.Height-1)
		if err != nil {
			return types.ActivityIndexDexPricesOutput{}, fmt.Errorf("fetch previous DEX prices at height %d: %w", in.Height-1, err)
		}
		// Build map keyed by (local_chain_id, remote_chain_id) for fast lookup
		rpcPricesH1 = make(map[string]*indexer.DexPrice, len(rpcH1))
		for _, price := range rpcH1 {
			dbPrice := transform.DexPrice(price)
			key := transform.DexPriceKey(dbPrice.LocalChainID, dbPrice.RemoteChainID)
			rpcPricesH1[key] = dbPrice
		}
	} else {
		rpcPricesH1 = make(map[string]*indexer.DexPrice)
	}

	// Convert RPC types to database models and calculate H-1 deltas
	dbPrices := make([]*indexer.DexPrice, 0, len(rpcPricesH))
	for _, rpcPrice := range rpcPricesH {
		dbPrice := transform.DexPrice(rpcPrice)
		dbPrice.Height = in.Height
		dbPrice.HeightTime = in.BlockTime

		// Calculate deltas by comparing with H-1
		key := transform.DexPriceKey(dbPrice.LocalChainID, dbPrice.RemoteChainID)
		if priceH1, exists := rpcPricesH1[key]; exists {
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

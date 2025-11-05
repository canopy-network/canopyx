package activity

import (
	"context"
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

	// Fetch DEX prices from RPC
	rpcPrices, err := cli.DexPricesByHeight(ctx, in.Height)
	if err != nil {
		return types.ActivityIndexDexPricesOutput{}, err
	}

	// Convert RPC types to database models and populate height fields
	dbPrices := make([]*indexer.DexPrice, 0, len(rpcPrices))
	for _, rpcPrice := range rpcPrices {
		dbPrice := transform.DexPrice(rpcPrice)
		dbPrice.Height = in.Height
		dbPrice.HeightTime = in.BlockTime
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

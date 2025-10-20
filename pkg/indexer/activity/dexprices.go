package activity

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// IndexDexPrices indexes DEX price information for a given block.
// Returns output containing the number of indexed price records and execution duration in milliseconds.
func (c *Context) IndexDexPrices(ctx context.Context, in types.IndexDexPricesInput) (types.IndexDexPricesOutput, error) {
	start := time.Now()

	ch, err := c.IndexerDB.GetChain(ctx, in.ChainID)
	if err != nil {
		return types.IndexDexPricesOutput{}, err
	}

	// Acquire (or ping) the chain DB just to validate it exists.
	chainDb, chainDbErr := c.NewChainDb(ctx, in.ChainID)
	if chainDbErr != nil {
		return types.IndexDexPricesOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	// Fetch DEX prices from RPC
	cli := c.rpcClient(ch.RPCEndpoints)
	prices, err := cli.DexPrices(ctx)
	if err != nil {
		return types.IndexDexPricesOutput{}, err
	}

	// Populate Height and HeightTime fields using the block height and timestamp
	for i := range prices {
		prices[i].Height = in.Height
		prices[i].HeightTime = in.BlockTime
	}

	numPrices := uint32(len(prices))
	c.Logger.Debug("IndexDexPrices fetched from RPC",
		zap.Uint64("height", in.Height),
		zap.Uint32("numPrices", numPrices))

	// Insert DEX prices to staging table (two-phase commit pattern)
	if err := chainDb.InsertDexPricesStaging(ctx, prices); err != nil {
		return types.IndexDexPricesOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.IndexDexPricesOutput{
		NumPrices:  numPrices,
		DurationMs: durationMs,
	}, nil
}

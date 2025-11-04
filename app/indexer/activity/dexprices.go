package activity

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	indexer "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/db/transform"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// IndexDexPrices indexes DEX price information for a given block.
// Returns output containing the number of indexed price records and execution duration in milliseconds.
func (c *Context) IndexDexPrices(ctx context.Context, in types.IndexDexPricesInput) (types.IndexDexPricesOutput, error) {
	start := time.Now()

	ch, err := c.AdminDB.GetChain(ctx, in.ChainID)
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
	rpcPrices, err := cli.DexPricesByHeight(ctx, in.Height)
	if err != nil {
		return types.IndexDexPricesOutput{}, err
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
	c.Logger.Debug("IndexDexPrices fetched from RPC",
		zap.Uint64("height", in.Height),
		zap.Uint32("numPrices", numPrices))

	// Insert DEX prices to staging table (two-phase commit pattern)
	if err := chainDb.InsertDexPricesStaging(ctx, dbPrices); err != nil {
		return types.IndexDexPricesOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.IndexDexPricesOutput{
		NumPrices:  numPrices,
		DurationMs: durationMs,
	}, nil
}

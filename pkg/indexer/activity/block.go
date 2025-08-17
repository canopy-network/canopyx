package activity

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"go.temporal.io/sdk/temporal"

	"github.com/canopy-network/canopyx/pkg/rpc"
)

// IndexBlock indexes a block for a given chain.
func (c *Context) IndexBlock(ctx context.Context, in types.IndexBlockInput) (uint64, error) {
	ch, err := c.IndexerDB.GetChain(ctx, in.ChainID)
	if err != nil {
		return 0, err
	}

	// Acquire (or ping) the chain DB just to validate it exists.
	chainDb, chainDbErr := c.NewChainDb(ctx, in.ChainID)
	if chainDbErr != nil {
		return in.Height, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	cli := rpc.NewHTTPWithOpts(rpc.Opts{Endpoints: ch.RPCEndpoints, RPS: 20, Burst: 40, BreakerFailures: 3, BreakerCooldown: 5 * time.Second})
	blk, err := cli.BlockByHeight(ctx, in.Height)
	if err != nil {
		return in.Height, err
	}

	if in.BlockSummaries == nil {
		return in.Height, temporal.NewApplicationErrorWithCause("input block summaries not found", "block_summaries_not_found", nil)
	}

	blk.NumTxs = in.BlockSummaries.NumTxs

	_, err = chainDb.Db.NewInsert().Model(blk).Exec(ctx)

	if err != nil {
		return blk.Height, err
	}

	return blk.Height, nil
}

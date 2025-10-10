package activity

import (
	"context"

	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"go.temporal.io/sdk/temporal"
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

	cli := c.rpcClient(ch.RPCEndpoints)
	blk, err := cli.BlockByHeight(ctx, in.Height)
	if err != nil {
		return in.Height, err
	}

	if in.BlockSummaries == nil {
		return in.Height, temporal.NewApplicationErrorWithCause("input block summaries not found", "block_summaries_not_found", nil)
	}

	blk.NumTxs = in.BlockSummaries.NumTxs

	if err = chainDb.InsertBlock(ctx, blk); err != nil {
		return blk.Height, err
	}

	return blk.Height, nil
}
func (c *Context) rpcClient(endpoints []string) rpc.Client {
	factory := c.RPCFactory
	if factory == nil {
		factory = rpc.NewHTTPFactory(c.RPCOpts)
	}
	return factory.NewClient(endpoints)
}

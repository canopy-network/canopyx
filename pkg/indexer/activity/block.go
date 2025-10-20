package activity

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"go.temporal.io/sdk/temporal"
)

// FetchBlockFromRPC is a local activity that fetches a block from the RPC endpoint.
// This is a local activity to enable fast retries when waiting for block propagation.
// Returns the block data wrapped in FetchBlockOutput.
func (c *Context) FetchBlockFromRPC(ctx context.Context, in types.IndexBlockInput) (types.FetchBlockOutput, error) {
	start := time.Now()

	ch, err := c.IndexerDB.GetChain(ctx, in.ChainID)
	if err != nil {
		return types.FetchBlockOutput{}, err
	}

	cli := c.rpcClient(ch.RPCEndpoints)
	blk, err := cli.BlockByHeight(ctx, in.Height)
	if err != nil {
		return types.FetchBlockOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.FetchBlockOutput{Block: blk, DurationMs: durationMs}, nil
}

// SaveBlock saves a fetched block to the database.
// This is a regular activity (not local) to ensure database writes are properly tracked in workflow history.
// Returns output containing the saved block height and execution duration in milliseconds.
func (c *Context) SaveBlock(ctx context.Context, in types.SaveBlockInput) (types.IndexBlockOutput, error) {
	start := time.Now()

	// Acquire (or ping) the chain DB just to validate it exists.
	chainDb, chainDbErr := c.NewChainDb(ctx, in.ChainID)
	if chainDbErr != nil {
		return types.IndexBlockOutput{Height: in.Height}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	// Insert block to staging table (two-phase commit pattern)
	if err := chainDb.InsertBlocksStaging(ctx, in.Block); err != nil {
		return types.IndexBlockOutput{Height: in.Height}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.IndexBlockOutput{Height: in.Height, DurationMs: durationMs}, nil
}

// IndexBlock indexes a block for a given chain (legacy method - fetches and saves in one step).
// Returns output containing the indexed block height and execution duration in milliseconds.
// NOTE: This is kept for backward compatibility but new code should use FetchBlockFromRPC + SaveBlock.
func (c *Context) IndexBlock(ctx context.Context, in types.IndexBlockInput) (types.IndexBlockOutput, error) {
	start := time.Now()

	ch, err := c.IndexerDB.GetChain(ctx, in.ChainID)
	if err != nil {
		return types.IndexBlockOutput{}, err
	}

	// Acquire (or ping) the chain DB just to validate it exists.
	chainDb, chainDbErr := c.NewChainDb(ctx, in.ChainID)
	if chainDbErr != nil {
		return types.IndexBlockOutput{Height: in.Height}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	cli := c.rpcClient(ch.RPCEndpoints)
	blk, err := cli.BlockByHeight(ctx, in.Height)
	if err != nil {
		return types.IndexBlockOutput{Height: in.Height}, err
	}

	// Insert block to staging table (two-phase commit pattern)
	if err = chainDb.InsertBlocksStaging(ctx, blk); err != nil {
		return types.IndexBlockOutput{Height: blk.Height}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.IndexBlockOutput{Height: blk.Height, DurationMs: durationMs}, nil
}

func (c *Context) rpcClient(endpoints []string) rpc.Client {
	factory := c.RPCFactory
	if factory == nil {
		factory = rpc.NewHTTPFactory(c.RPCOpts)
	}
	return factory.NewClient(endpoints)
}

package activity

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"github.com/canopy-network/canopyx/pkg/db/transform"
	"go.temporal.io/sdk/temporal"
)

// FetchBlockFromRPC is a local activity that fetches a block from the RPC endpoint.
// This is a local activity to enable fast retries when waiting for block propagation.
// Returns the block data wrapped in ActivityFetchBlockOutput.
func (ac *Context) FetchBlockFromRPC(ctx context.Context, in types.ActivityFetchBlockInput) (types.ActivityFetchBlockOutput, error) {
	start := time.Now()

	cli, err := ac.rpcClient(ctx)
	if err != nil {
		return types.ActivityFetchBlockOutput{}, err
	}

	rpcBlock, err := cli.BlockByHeight(ctx, in.Height)
	if err != nil {
		return types.ActivityFetchBlockOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.ActivityFetchBlockOutput{Block: rpcBlock, DurationMs: durationMs}, nil
}

// SaveBlock saves a fetched block to the database.
// This is a regular activity (not local) to ensure database writes are properly tracked in workflow history.
// Returns output containing the saved block height and execution duration in milliseconds.
func (ac *Context) SaveBlock(ctx context.Context, in types.ActivitySaveBlockInput) (types.ActivitySaveBlockOutput, error) {
	start := time.Now()

	chainDb, chainDbErr := ac.GetChainDb(ctx, ac.ChainID)
	if chainDbErr != nil {
		return types.ActivitySaveBlockOutput{Height: in.Height},
			temporal.NewApplicationErrorWithCause(
				"unable to acquire chain database",
				"chain_db_error",
				chainDbErr,
			)
	}

	// Convert an RPC type to an indexer model
	block := transform.Block(in.Block)

	// Insert block to staging table (two-phase commit pattern)
	if err := chainDb.InsertBlocksStaging(ctx, block); err != nil {
		return types.ActivitySaveBlockOutput{Height: in.Height}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.ActivitySaveBlockOutput{Height: in.Height, DurationMs: durationMs}, nil
}

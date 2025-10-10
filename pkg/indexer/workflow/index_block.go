package workflow

import (
	"time"

	"github.com/canopy-network/canopyx/pkg/indexer/activity"
	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// IndexBlockWorkflow Kick off indexing of a block for a given chain in a separate workflow at `index:<chain>` queue
func (c *Context) IndexBlockWorkflow(ctx workflow.Context, in types.IndexBlockInput) error {
	// TODO: add checks for chainId and height values at `in` param, also verify database exists (it should)
	//  check if the current options are good enough for indexing the block
	retry := &temporal.RetryPolicy{
		InitialInterval:    time.Second,
		BackoffCoefficient: 1.2,
		MaximumInterval:    5 * time.Second, // we need it fast since blocks are the 20s apart
		MaximumAttempts:    0,               // zero means unlimited - we want it keeps trying to index a block until it succeeds
	}
	ao := workflow.ActivityOptions{
		// NOTE: this should never reach 2 minutes to index a block, but WHO knows...
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy:         retry,
		// his own queue to avoid blocking a single queue for all chains
		TaskQueue: c.TemporalClient.GetIndexerQueue(in.ChainID),
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var skip bool
	if err := workflow.ExecuteActivity(ctx, (*activity.Context).PrepareIndexBlock, in).Get(ctx, &skip); err != nil {
		return err
	}
	if skip {
		return nil
	}

	var indexedTxs uint32
	if err := workflow.ExecuteActivity(ctx, (*activity.Context).IndexTransactions, in).Get(ctx, &indexedTxs); err != nil {
		return err
	}

	in.BlockSummaries = &types.BlockSummaries{NumTxs: indexedTxs}

	// Store the block as the last step also prevent have partial blocks indexed into the database.
	// pass the values that we need to "summarize" into the block, like the number of txs
	// this will minimize the reads and small updates on to the block record
	var indexedHeight uint64
	if err := workflow.ExecuteActivity(ctx, (*activity.Context).IndexBlock, in).Get(ctx, &indexedHeight); err != nil {
		return err
	}

	// TODO: keep adding here more entities to index

	return workflow.ExecuteActivity(ctx, (*activity.Context).RecordIndexed, in).Get(ctx, nil)
}

package workflow

import (
	"encoding/json"
	"time"

	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// IndexBlockWorkflow kicks off indexing of a block for a given chain in a separate workflow at `index:<chain>` queue.
// It tracks the execution time of each activity and records detailed timing metrics.
func (wc *Context) IndexBlockWorkflow(ctx workflow.Context, in types.IndexBlockInput) error {
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
		TaskQueue: wc.TemporalClient.GetIndexerQueue(in.ChainID),
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Collect timing data for each activity
	timings := make(map[string]float64)

	// 1. PrepareIndexBlock - check if block needs indexing
	var prepareOut types.PrepareIndexBlockOutput
	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.PrepareIndexBlock, in).Get(ctx, &prepareOut); err != nil {
		return err
	}
	timings["prepare_index_ms"] = prepareOut.DurationMs

	// If block should be skipped, record timing and exit early
	if prepareOut.Skip {
		// Even for skipped blocks, we record that we checked them
		detailBytes, _ := json.Marshal(timings)
		recordInput := types.RecordIndexedInput{
			ChainID:        in.ChainID,
			Height:         in.Height,
			IndexingTimeMs: prepareOut.DurationMs,
			IndexingDetail: string(detailBytes),
		}
		return workflow.ExecuteActivity(ctx, wc.ActivityContext.RecordIndexed, recordInput).Get(ctx, nil)
	}

	// 2. IndexTransactions - fetch and index transactions
	var txOut types.IndexTransactionsOutput
	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexTransactions, in).Get(ctx, &txOut); err != nil {
		return err
	}
	timings["index_transactions_ms"] = txOut.DurationMs

	// Pass transaction count to block summary
	in.BlockSummaries = &types.BlockSummaries{NumTxs: txOut.NumTxs}

	// 3. IndexBlock - fetch and store block metadata
	// Store the block as the last step to prevent having partial blocks indexed into the database.
	// Pass the values that we need to "summarize" into the block, like the number of txs.
	// This will minimize the reads and small updates on to the block record.
	var blockOut types.IndexBlockOutput
	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexBlock, in).Get(ctx, &blockOut); err != nil {
		return err
	}
	timings["index_block_ms"] = blockOut.DurationMs

	// TODO: keep adding here more entities to index

	// Calculate total indexing time (sum of all activity durations)
	var totalMs float64
	for _, duration := range timings {
		totalMs += duration
	}

	// Marshal timing details to JSON
	detailBytes, err := json.Marshal(timings)
	if err != nil {
		// If marshaling fails, use empty string but still record the total
		detailBytes = []byte("{}")
	}

	// 4. RecordIndexed - persist indexing progress with timing metrics
	recordInput := types.RecordIndexedInput{
		ChainID:        in.ChainID,
		Height:         in.Height,
		IndexingTimeMs: totalMs,
		IndexingDetail: string(detailBytes),
	}

	return workflow.ExecuteActivity(ctx, wc.ActivityContext.RecordIndexed, recordInput).Get(ctx, nil)
}

package workflow

import (
	"encoding/json"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
	"time"
)

// IndexBlockWorkflow kicks off indexing of a block for a given chain in a separate workflow at `index:<chain>` queue.
// It tracks the execution time of each activity and records detailed timing metrics.
func (wc *Context) IndexBlockWorkflow2(ctx workflow.Context, in types.WorkflowIndexBlockInput) error {
	//  check if the current options are good enough for indexing the block
	retry := &temporal.RetryPolicy{
		InitialInterval:    100 * time.Millisecond, // Start quickly for block propagation delays
		BackoffCoefficient: 1.2,                    // Grow moderately (200ms, 300ms, 450ms, 675ms, 1s...)
		MaximumInterval:    2 * time.Second,        // Cap at 2s for faster retries when block isn't ready
		MaximumAttempts:    0,                      // zero means unlimited - we want it keeps trying to index a block until it succeeds
	}
	// Extract the task queue from workflow info - activities must run on the same queue as the workflow
	// This ensures activities run on the correct queue (live or historical) that the workflow was scheduled to
	info := workflow.GetInfo(ctx)
	taskQueue := info.TaskQueueName

	ao := workflow.ActivityOptions{
		// NOTE: this should never reach 2 minutes to index a block, but WHO knows...
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy:         retry,
		// Use the same task queue as the workflow (live or historical)
		TaskQueue: taskQueue,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)
	// Collect timing data for each activity
	timings := make(map[string]float64)

	// 1. PrepareIndexBlock - check if block needs indexing TODO durability plan required for partial writes (2 phase commitment doesn't fix that)
	var prepareOut types.ActivityPrepareIndexBlockOutput
	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.PrepareIndexBlock, types.ActivityIndexBlockInput{
		Height:      in.Height,
		PriorityKey: in.PriorityKey,
		Reindex:     in.Reindex,
	}).Get(ctx, &prepareOut); err != nil {
		return err
	}
	timings["prepare_index_ms"] = prepareOut.DurationMs

	// If block should be skipped, record timing and exit early
	if prepareOut.Skip {
		// Even for skipped blocks, we record that we checked them
		detailBytes, _ := json.Marshal(timings)
		recordInput := types.ActivityRecordIndexedInput{
			Height:         in.Height,
			IndexingTimeMs: prepareOut.DurationMs,
			IndexingDetail: string(detailBytes),
		}
		return workflow.ExecuteActivity(ctx, wc.ActivityContext.RecordIndexed, recordInput).Get(ctx, nil)
	}

	// 2. FetchBlock - fetch block from RPC (local activity for fast retries)
	// This uses a local activity to retry quickly when waiting for block propagation
	// NO TIMEOUT: We want unlimited retries since blocks MUST eventually be indexed
	// The retry policy handles backoff, we just need to be patient
	var fetchOut types.ActivityFetchBlobOutput
	localActivityOpts := workflow.LocalActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		RetryPolicy:         retry,
	}
	localCtx := workflow.WithLocalActivityOptions(ctx, localActivityOpts)
	if err := workflow.ExecuteLocalActivity(localCtx, wc.ActivityContext.FetchBlobFromRPC, types.ActivityFetchBlockInput{
		Height: in.Height,
	}).Get(localCtx, &fetchOut); err != nil {
		return err
	}
	timings["fetch_block_ms"] = fetchOut.DurationMs

	indexFromBlobInput := types.ActivityIndexBlockFromBlobInput{
		Height:            in.Height,
		Blobs:             fetchOut.Blobs,
		PrepareDurationMs: prepareOut.DurationMs,
		FetchDurationMs:   fetchOut.DurationMs,
	}
	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexBlockFromBlob, indexFromBlobInput).Get(ctx, nil); err != nil {
		return err
	}
	return nil
}

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

    // 2. FetchBlock - fetch block from RPC (local activity for fast retries)
    // This uses a local activity to retry quickly when waiting for block propagation
    // Timeout: 5 minutes to allow ~150 retry attempts (2s interval) for blocks not ready yet
    // With 20s block time, worst case is waiting ~60s for a block to become available
    var fetchOut types.FetchBlockOutput
    localActivityOpts := workflow.LocalActivityOptions{
        StartToCloseTimeout: 5 * time.Minute, // Allow enough time for blocks to become available
        RetryPolicy:         retry,           // Use the same fast retry policy
    }
    localCtx := workflow.WithLocalActivityOptions(ctx, localActivityOpts)
    if err := workflow.ExecuteLocalActivity(localCtx, wc.ActivityContext.FetchBlockFromRPC, in).Get(localCtx, &fetchOut); err != nil {
        return err
    }
    timings["fetch_block_ms"] = fetchOut.DurationMs

    // 3. SaveBlock - store block metadata to database (regular activity)
    // This MUST come before entity indexing to ensure the block exists on-chain
    saveBlockInput := types.SaveBlockInput{
        ChainID: in.ChainID,
        Height:  in.Height,
        Block:   fetchOut.Block,
    }
    var saveBlockOut types.IndexBlockOutput
    if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.SaveBlock, saveBlockInput).Get(ctx, &saveBlockOut); err != nil {
        return err
    }
    timings["save_block_ms"] = saveBlockOut.DurationMs

    // 4. IndexTransactions - fetch and index transactions
    // Now safe to index entities since block existence is confirmed
    // Pass the block input which includes the block data with timestamp
    indexTxInput := types.IndexTransactionsInput{
        ChainID:   in.ChainID,
        Height:    in.Height,
        BlockTime: fetchOut.Block.Time,
    }
    var txOut types.IndexTransactionsOutput
    if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexTransactions, indexTxInput).Get(ctx, &txOut); err != nil {
        return err
    }
    timings["index_transactions_ms"] = txOut.DurationMs

    // TODO: Add more entity indexing activities here (events, logs, etc.)
    // Each activity should return a summary output with DurationMs and entity counts

    // Collect all summaries for aggregation
    summaries := types.BlockSummaries{
        NumTxs: txOut.NumTxs,
    }

    // 5. SaveBlockSummary - aggregate and save all entity summaries
    saveInput := types.SaveBlockSummaryInput{
        ChainID:   in.ChainID,
        Height:    in.Height,
        BlockTime: fetchOut.Block.Time,
        Summaries: summaries,
    }
    var saveSummaryOut types.SaveBlockSummaryOutput
    if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.SaveBlockSummary, saveInput).Get(ctx, &saveSummaryOut); err != nil {
        return err
    }
    timings["save_block_summary_ms"] = saveSummaryOut.DurationMs

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

    // 5. RecordIndexed - persist indexing progress with timing metrics
    recordInput := types.RecordIndexedInput{
        ChainID:        in.ChainID,
        Height:         in.Height,
        IndexingTimeMs: totalMs,
        IndexingDetail: string(detailBytes),
    }

    return workflow.ExecuteActivity(ctx, wc.ActivityContext.RecordIndexed, recordInput).Get(ctx, nil)
}

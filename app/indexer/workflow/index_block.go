package workflow

import (
    "github.com/canopy-network/canopyx/app/indexer/types"
    "go.temporal.io/sdk/temporal"
    "go.temporal.io/sdk/workflow"
    "time"
)

// IndexBlockWorkflow kicks off indexing of a block for a given chain in a separate workflow at `index:<chain>` queue.
// It tracks the execution time of each activity and records detailed timing metrics.
func (wc *Context) IndexBlockWorkflow(ctx workflow.Context, in types.WorkflowIndexBlockInput) error {
    logger := workflow.GetLogger(ctx)

    // Extract the task queue from workflow info - activities must run on the same queue as the workflow
    // This ensures activities run on the correct queue (live or historical) that the workflow was scheduled to
    info := workflow.GetInfo(ctx)
    taskQueue := info.TaskQueueName

    // Log workflow execution start with namespace info for debugging
    logger.Info("IndexBlockWorkflow started",
        "chain_id", wc.ChainID,
        "height", in.Height,
        "namespace", info.Namespace,
        "task_queue", taskQueue,
        "workflow_id", info.WorkflowExecution.ID,
        "run_id", info.WorkflowExecution.RunID,
    )

    //  check if the current options are good enough for indexing the block
    retry := &temporal.RetryPolicy{
        InitialInterval:    100 * time.Millisecond, // Start quickly for block propagation delays
        BackoffCoefficient: 1.2,                    // Grow moderately (200ms, 300ms, 450ms, 675ms, 1s...)
        MaximumInterval:    2 * time.Second,        // Cap at 2s for faster retries when block isn't ready
        MaximumAttempts:    0,                      // zero means unlimited - we want it keeps trying to index a block until it succeeds
    }

    ao := workflow.ActivityOptions{
        // NOTE: this should never reach 2 minutes to index a block, but WHO knows...
        StartToCloseTimeout: 2 * time.Minute,
        RetryPolicy:         retry,
        // Use the same task queue as the workflow (live or historical)
        TaskQueue: taskQueue,
    }
    ctx = workflow.WithActivityOptions(ctx, ao)

    // 1. PrepareIndexBlock - check if block needs indexing TODO durability plan required for partial writes (2 phase commitment doesn't fix that)
    var prepareOut types.ActivityPrepareIndexBlockOutput
    //localPrepareCtx := workflow.WithLocalActivityOptions(ctx, workflow.LocalActivityOptions{
    //    StartToCloseTimeout: 1 * time.Minute,
    //    RetryPolicy:         retry,
    //})
    if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.PrepareIndexBlock, types.ActivityIndexBlockInput{
        Height:      in.Height,
        PriorityKey: in.PriorityKey,
        Reindex:     in.Reindex,
    }).Get(ctx, &prepareOut); err != nil {
        return err
    }

    // If a block should be skipped, record timing and exit early
    if prepareOut.Skip {
        // Even for skipped blocks, we record that we checked them
        // Only TimingPrepareIndexMs is populated for skipped blocks
        recordInput := types.ActivityRecordIndexedInput{
            Height:               in.Height,
            IndexingTimeMs:       prepareOut.DurationMs,
            TimingPrepareIndexMs: prepareOut.DurationMs,
        }
        return workflow.ExecuteActivity(ctx, wc.ActivityContext.RecordIndexed, recordInput).Get(ctx, nil)
    }

    blobCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
        StartToCloseTimeout: 10 * time.Minute,
        RetryPolicy:         retry,
    })

    indexFromBlobInput := types.ActivityIndexBlockFromBlobInput{
        Height:            in.Height,
        PrepareDurationMs: prepareOut.DurationMs,
    }

    indexBlockResult := types.ActivityIndexBlockFromBlobOutput{}
    if err := workflow.ExecuteActivity(blobCtx, wc.ActivityContext.IndexBlockFromBlob, indexFromBlobInput).Get(ctx, &indexBlockResult); err != nil {
        return err
    }

    // Populate individual timing fields for columnar storage in ClickHouse
    recordInput := types.ActivityRecordIndexedInput{
        Height:                    in.Height,
        IndexingTimeMs:            indexBlockResult.IndexingTimeMs,
        BlockHash:                 indexBlockResult.BlockHash,
        BlockTime:                 indexBlockResult.BlockTime,
        BlockProposerAddress:      indexBlockResult.BlockProposerAddress,
        TimingFetchBlockMs:        indexBlockResult.IndexingDetail["fetch_block_ms"],
        TimingPrepareIndexMs:      indexBlockResult.IndexingDetail["prepare_index_ms"],
        TimingIndexAccountsMs:     indexBlockResult.IndexingDetail["index_accounts_ms"],
        TimingIndexCommitteesMs:   indexBlockResult.IndexingDetail["index_committees_ms"],
        TimingIndexDexBatchMs:     indexBlockResult.IndexingDetail["index_dex_batch_ms"],
        TimingIndexDexPricesMs:    indexBlockResult.IndexingDetail["index_dex_prices_ms"],
        TimingIndexEventsMs:       indexBlockResult.IndexingDetail["index_events_ms"],
        TimingIndexOrdersMs:       indexBlockResult.IndexingDetail["index_orders_ms"],
        TimingIndexParamsMs:       indexBlockResult.IndexingDetail["index_params_ms"],
        TimingIndexPoolsMs:        indexBlockResult.IndexingDetail["index_pools_ms"],
        TimingIndexSupplyMs:       indexBlockResult.IndexingDetail["index_supply_ms"],
        TimingIndexTransactionsMs: indexBlockResult.IndexingDetail["index_transactions_ms"],
        TimingIndexValidatorsMs:   indexBlockResult.IndexingDetail["index_validators_ms"],
        TimingSaveBlockMs:         indexBlockResult.IndexingDetail["save_block_ms"],
        TimingSaveBlockSummaryMs:  indexBlockResult.IndexingDetail["save_block_summary_ms"],
    }

    return workflow.ExecuteActivity(ctx, wc.ActivityContext.RecordIndexed, recordInput).Get(ctx, &indexBlockResult)
}

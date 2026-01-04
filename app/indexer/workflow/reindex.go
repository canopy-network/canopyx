package workflow

import (
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	sdktemporal "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// ReindexSchedulerWorkflow handles batch scheduling for large reindex ranges.
// Processes blocks from newest to oldest in batches with ContinueAsNew for large ranges.
// Routes all workflows to dedicated reindex queue.
func (wc *Context) ReindexSchedulerWorkflow(ctx workflow.Context, input types.WorkflowReindexSchedulerInput) (types.WorkflowReindexSchedulerOutput, error) {
	logger := workflow.GetLogger(ctx)
	startTime := workflow.Now(ctx)

	logger.Info("ReindexSchedulerWorkflow started",
		"chain_id", input.ChainID,
		"start_height", input.StartHeight,
		"end_height", input.EndHeight,
		"requested_by", input.RequestedBy,
		"processed_so_far", input.ProcessedSoFar,
	)

	// Activity options for batch scheduling
	// Use regular activities (not local) to distribute load across workers
	// Timeout increased to 2 minutes to accommodate larger batch sizes
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    200 * time.Millisecond,
			BackoffCoefficient: 1.2,
			MaximumInterval:    2 * time.Second,
			MaximumAttempts:    0, // Unlimited retries
		},
		TaskQueue: wc.ChainClient.OpsQueue,
	}
	activityCtx := workflow.WithActivityOptions(ctx, ao)

	currentHeight := input.EndHeight // Start from newest
	startHeight := input.StartHeight
	processed := input.ProcessedSoFar
	totalScheduled := uint64(0)
	totalFailed := uint64(0)

	// Get batch size from config (default: 5000)
	batchLimit := wc.Config.SchedulerBatchSize
	if batchLimit == 0 {
		batchLimit = 5000
	}

	// Process blocks in batches from NEWEST to OLDEST
	for currentHeight >= startHeight {
		// Determine batch start height (going backwards) without uint64 underflow
		remaining := currentHeight - startHeight + 1
		var batchStartHeight uint64

		if remaining <= batchLimit {
			batchStartHeight = startHeight
		} else {
			batchStartHeight = currentHeight - batchLimit + 1
		}

		batchSize := int(currentHeight - batchStartHeight + 1)

		// Execute batch scheduling via activity
		batchInput := types.ActivityReindexBatchInput{
			ChainID:     input.ChainID,
			StartHeight: batchStartHeight,
			EndHeight:   currentHeight,
			Reindex:     true,            // Mark as reindex operation
			RequestID:   input.RequestID, // Use workflow's unique request ID
		}

		var batchResult types.ActivityReindexBatchOutput
		err := workflow.ExecuteActivity(activityCtx, wc.ActivityContext.StartReindexWorkflowBatch, batchInput).Get(activityCtx, &batchResult)
		if err != nil {
			logger.Error("Failed to schedule reindex batch",
				"start", batchStartHeight,
				"end", currentHeight,
				"batch_size", batchSize,
				"error", err)
			// Continue with next batch even on error (idempotent activity)
		} else {
			totalScheduled += uint64(batchResult.Scheduled)
			totalFailed += uint64(batchResult.Failed)
		}

		processed += uint64(batchSize)

		// Safe decrement - prevent uint64 underflow when batchStartHeight is 0
		if batchStartHeight == 0 {
			break // Reached height 0, all blocks processed
		}
		currentHeight = batchStartHeight - 1

		// Log progress every 5k blocks
		if processed%5000 == 0 {
			logger.Info("ReindexSchedulerWorkflow progress",
				"chain_id", input.ChainID,
				"processed", processed,
				"current_height", currentHeight,
				"scheduled", totalScheduled,
				"failed", totalFailed,
			)
		}

		// Check for ContinueAsNew threshold (every 5k blocks)
		if processed >= uint64(schedulerContinueThreshold) && currentHeight >= startHeight {
			logger.Info("Triggering ContinueAsNew for reindex scheduler",
				"processed_so_far", processed,
				"remaining_height", currentHeight,
				"scheduled", totalScheduled,
				"failed", totalFailed,
			)

			return types.WorkflowReindexSchedulerOutput{}, workflow.NewContinueAsNewError(ctx, wc.ReindexSchedulerWorkflow, types.WorkflowReindexSchedulerInput{
				ChainID:        input.ChainID,
				StartHeight:    startHeight,
				EndHeight:      currentHeight,
				RequestedBy:    input.RequestedBy,
				RequestID:      input.RequestID,
				ProcessedSoFar: 0,
			})
		}
	}

	durationMs := float64(workflow.Now(ctx).Sub(startTime).Microseconds()) / 1000.0

	logger.Info("ReindexSchedulerWorkflow completed",
		"chain_id", input.ChainID,
		"total_blocks", input.EndHeight-input.StartHeight+1,
		"total_scheduled", totalScheduled,
		"total_failed", totalFailed,
		"duration_ms", durationMs,
	)

	return types.WorkflowReindexSchedulerOutput{
		TotalBlocks:    input.EndHeight - input.StartHeight + 1,
		TotalScheduled: totalScheduled,
		TotalFailed:    totalFailed,
		DurationMs:     durationMs,
	}, nil
}

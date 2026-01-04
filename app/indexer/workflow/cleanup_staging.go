package workflow

import (
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/zap"
)

// CleanupStagingWorkflow cleans up staging tables by removing data for heights
// that have been confirmed indexed with a safety buffer.
//
// This workflow is designed to run periodically (e.g., hourly) and is much more
// efficient than per-block cleanup because it:
// 1. Queries index_progress ONCE to get all cleanable heights
// 2. Deletes from each staging table using WHERE height IN (...) - single mutation per table
//
// The workflow cleans staging data for heights indexed within the last lookbackHours (default 2).
func (wc *Context) CleanupStagingWorkflow(ctx workflow.Context, input types.WorkflowCleanupStagingInput) (types.WorkflowCleanupStagingOutput, error) {
	start := time.Now()
	logger := workflow.GetLogger(ctx)

	// Apply defaults - 2 hour lookback means we clean heights indexed in the last 2 hours
	lookbackHours := input.LookbackHours
	if lookbackHours == 0 {
		lookbackHours = 2
	}

	logger.Info("Starting cleanup staging workflow",
		zap.Uint64("chainId", wc.ChainID),
		zap.Int("lookbackHours", lookbackHours))

	// Activity options for cleanup operations
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	// Step 1: Get cleanable heights from index_progress
	var heightsOutput types.ActivityGetCleanableHeightsOutput
	err := workflow.ExecuteActivity(ctx, wc.ActivityContext.GetCleanableHeights, types.ActivityGetCleanableHeightsInput{
		LookbackHours: lookbackHours,
	}).Get(ctx, &heightsOutput)

	if err != nil {
		logger.Error("Failed to get cleanable heights",
			zap.Uint64("chainId", wc.ChainID),
			zap.Error(err))
		return types.WorkflowCleanupStagingOutput{}, err
	}

	// If no heights to clean, we're done
	if len(heightsOutput.Heights) == 0 {
		logger.Info("No heights to clean",
			zap.Uint64("chainId", wc.ChainID))
		return types.WorkflowCleanupStagingOutput{
			HeightsCleaned:  0,
			EntitiesCleaned: 0,
			DurationMs:      float64(time.Since(start).Milliseconds()),
		}, nil
	}

	// Step 2: Batch clean staging tables
	var cleanOutput types.ActivityCleanStagingBatchOutput
	err = workflow.ExecuteActivity(ctx, wc.ActivityContext.CleanStagingBatch, types.ActivityCleanStagingBatchInput{
		Heights: heightsOutput.Heights,
	}).Get(ctx, &cleanOutput)

	if err != nil {
		logger.Error("Failed to clean staging batch",
			zap.Uint64("chainId", wc.ChainID),
			zap.Int("heights", len(heightsOutput.Heights)),
			zap.Error(err))
		return types.WorkflowCleanupStagingOutput{}, err
	}

	durationMs := float64(time.Since(start).Milliseconds())

	logger.Info("Cleanup staging workflow completed",
		zap.Uint64("chainId", wc.ChainID),
		zap.Int("heightsCleaned", cleanOutput.HeightsCleaned),
		zap.Int("entitiesCleaned", cleanOutput.EntitiesCleaned),
		zap.Float64("durationMs", durationMs))

	return types.WorkflowCleanupStagingOutput{
		HeightsCleaned:  cleanOutput.HeightsCleaned,
		EntitiesCleaned: cleanOutput.EntitiesCleaned,
		DurationMs:      durationMs,
	}, nil
}

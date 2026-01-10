package activity

import (
	"context"
	"errors"
	"time"

	"sync/atomic"

	"github.com/alitto/pond/v2"
	"github.com/canopy-network/canopyx/app/indexer/types"
	indexerworkflow "github.com/canopy-network/canopyx/pkg/temporal/indexer"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	sdktemporal "go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// StartReindexWorkflowBatch schedules IndexBlock workflows with reindex=true.
// Routes all workflows to the dedicated reindex queue.
// Uses parallel dispatch with worker pools for efficient batching.
func (ac *Context) StartReindexWorkflowBatch(ctx context.Context, in types.ActivityReindexBatchInput) (types.ActivityReindexBatchOutput, error) {
	logger := activity.GetLogger(ctx)

	logger.Info("Starting reindex batch scheduling",
		zap.Uint64("chain_id", in.ChainID),
		zap.Uint64("start_height", in.StartHeight),
		zap.Uint64("end_height", in.EndHeight),
		zap.Int("batch_size", int(in.EndHeight-in.StartHeight+1)),
	)

	totalHeights := int(in.EndHeight - in.StartHeight + 1)
	if totalHeights <= 0 {
		logger.Info("Reindex batch scheduling completed (empty batch)",
			zap.Uint64("chain_id", in.ChainID),
			zap.Int("scheduled", 0),
			zap.Int("failed", 0),
		)
		return types.ActivityReindexBatchOutput{
			Scheduled: 0,
			Failed:    0,
		}, nil
	}

	// All reindex workflows go to dedicated reindex queue
	reindexQueue := ac.ChainClient.ReindexQueue

	logger.Info("Batch routing to reindex queue",
		zap.String("task_queue", reindexQueue),
		zap.Uint64("start", in.StartHeight),
		zap.Uint64("end", in.EndHeight),
	)

	return ac.scheduleReindexBatchToQueue(ctx, in, reindexQueue)
}

// scheduleReindexBatchToQueue schedules reindex workflows in parallel to the specified queue.
// Uses worker pools for efficient parallel dispatch.
func (ac *Context) scheduleReindexBatchToQueue(ctx context.Context, in types.ActivityReindexBatchInput, taskQueue string) (types.ActivityReindexBatchOutput, error) {
	logger := activity.GetLogger(ctx)
	start := time.Now()

	var scheduled atomic.Int32
	var failed atomic.Int32

	totalHeights := int(in.EndHeight - in.StartHeight + 1)
	if totalHeights <= 0 {
		return types.ActivityReindexBatchOutput{
			Scheduled: 0,
			Failed:    0,
		}, nil
	}

	pool := ac.WorkerPool(totalHeights)
	group := pool.NewGroupContext(ctx)
	groupCtx := group.Context()

	// Schedule all heights in parallel
	for height := in.StartHeight; height <= in.EndHeight; height++ {
		h := height

		group.Submit(func() {
			if err := groupCtx.Err(); err != nil {
				return
			}

			// Use deterministic workflow ID to prevent duplicate reindex workflows
			wfID := ac.ChainClient.GetReindexBlockWorkflowID(h, in.RequestID)
			options := client.StartWorkflowOptions{
				ID:        wfID,
				TaskQueue: taskQueue, // Reindex queue
				// If workflow already running, return existing handle instead of starting new
				// This prevents duplicate work if operator accidentally triggers reindex twice
				WorkflowIDConflictPolicy: enums.WORKFLOW_ID_CONFLICT_POLICY_USE_EXISTING,
				// Only allow reindex if previous attempt failed (don't re-run successful blocks)
				WorkflowIDReusePolicy: enums.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE_FAILED_ONLY,
				RetryPolicy: &sdktemporal.RetryPolicy{
					InitialInterval:    time.Second,
					BackoffCoefficient: 1.2,
					MaximumInterval:    5 * time.Second,
					MaximumAttempts:    0, // Unlimited retries
				},
			}

			_, err := ac.ChainClient.TClient.ExecuteWorkflow(groupCtx, options, indexerworkflow.IndexBlockWorkflowName, types.WorkflowIndexBlockInput{
				Height:  h,
				Reindex: true, // Mark as reindex operation
			})
			if err != nil {
				var alreadyStarted *serviceerror.WorkflowExecutionAlreadyStarted
				if errors.As(err, &alreadyStarted) {
					scheduled.Add(1) // Count as scheduled (idempotent)
					return
				}
				logger.Warn("failed to schedule reindex workflow in batch",
					zap.Uint64("height", h),
					zap.String("queue", taskQueue),
					zap.Error(err),
				)
				failed.Add(1)
				return
			}
			scheduled.Add(1)
		})
	}

	if err := group.Wait(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, pond.ErrGroupStopped) {
		logger.Warn("reindex batch scheduling group encountered error",
			zap.Uint64("chain_id", in.ChainID),
			zap.String("queue", taskQueue),
			zap.Error(err),
		)
	}

	scheduledValue := int(scheduled.Load())
	failedValue := int(failed.Load())
	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	totalProcessed := scheduledValue + failedValue
	var rate float64
	if durationMs > 0 {
		rate = float64(totalProcessed) / (durationMs / 1000.0)
	}

	parallelism := ac.WorkerPoolSize()
	if totalHeights < parallelism {
		parallelism = totalHeights
	}

	logger.Info("Reindex batch scheduling to queue completed",
		zap.Uint64("chain_id", in.ChainID),
		zap.String("queue", taskQueue),
		zap.Int("scheduled", scheduledValue),
		zap.Int("failed", failedValue),
		zap.Float64("duration_ms", durationMs),
		zap.Float64("workflows_per_sec", rate),
		zap.Int("parallelism", parallelism),
	)

	return types.ActivityReindexBatchOutput{
		Scheduled: scheduledValue,
		Failed:    failedValue,
	}, nil
}

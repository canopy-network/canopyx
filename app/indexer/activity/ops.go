package activity

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/utils"

	"github.com/alitto/pond/v2"
	"github.com/canopy-network/canopyx/app/indexer/types"
	adminmodels "github.com/canopy-network/canopyx/pkg/db/models/admin"
	indexerworkflow "github.com/canopy-network/canopyx/pkg/temporal/indexer"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	sdktemporal "go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// HeadResult summarizes the latest and last indexed heights produced by HeadScan.
type HeadResult struct {
	Start  uint64
	End    uint64
	Head   uint64
	Latest uint64
}

// PrepareIndexBlock determines whether a block needs reindexing and performs cleanup if required.
// Returns output containing skip flag and execution duration in milliseconds.
func (ac *Context) PrepareIndexBlock(ctx context.Context, in types.ActivityIndexBlockInput) (types.ActivityPrepareIndexBlockOutput, error) {
	start := time.Now()

	chainDb, err := ac.GetChainDb(ctx, ac.ChainID)
	if err != nil {
		return types.ActivityPrepareIndexBlockOutput{}, sdktemporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", err)
	}

	exists, err := chainDb.HasBlock(ctx, in.Height)
	if err != nil {
		return types.ActivityPrepareIndexBlockOutput{}, sdktemporal.NewApplicationErrorWithCause("block_lookup_failed", "chain_db_error", err)
	}

	// Skip if block already exists and this is not a reindex operation
	if !in.Reindex && exists {
		durationMs := float64(time.Since(start).Microseconds()) / 1000.0
		return types.ActivityPrepareIndexBlockOutput{Skip: true, DurationMs: durationMs}, nil
	}

	// For reindex, proceed with indexing (no delete step needed)
	// ReplacingMergeTree automatically deduplicates based on ClickHouse internal metadata
	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.ActivityPrepareIndexBlockOutput{Skip: false, DurationMs: durationMs}, nil
}

// GetLastIndexed returns the last indexed block height for the given chain.
func (ac *Context) GetLastIndexed(ctx context.Context) (uint64, error) {
	return ac.AdminDB.LastIndexed(ctx, ac.ChainID)
}

// GetLatestHead returns the latest block height for the given chain by checking ALL RPC endpoints in parallel.
// It updates the health status for each individual endpoint and the overall chain health.
// Returns the maximum height found across all healthy endpoints.
// NOTE: /v1/height may return blocks before they're ready, so we verify block availability.
// Uses lightweight GetChainRPCEndpoints query (2 columns instead of 25).
func (ac *Context) GetLatestHead(ctx context.Context) (uint64, error) {
	logger := activity.GetLogger(ctx)

	// Get chain RPC endpoints only (lightweight query)
	chain, err := ac.AdminDB.GetChainRPCEndpoints(ctx, ac.ChainID)
	if err != nil {
		return 0, fmt.Errorf("failed to get chain: %w", err)
	}

	endpoints := chain.RPCEndpoints
	if len(endpoints) == 0 {
		return 0, fmt.Errorf("no RPC endpoints configured for chain %d", ac.ChainID)
	}

	// Result type for parallel endpoint checks
	type endpointResult struct {
		endpoint  string
		height    uint64
		latencyMs float64
		err       error
	}

	results := make(chan endpointResult, len(endpoints))

	// Query all endpoints in parallel
	for _, ep := range endpoints {
		go func(endpoint string) {
			// Check for context cancellation before making RPC calls
			if ctx.Err() != nil {
				results <- endpointResult{endpoint: endpoint, err: ctx.Err()}
				return
			}

			start := time.Now()
			factory := ac.RPCFactory
			if factory == nil {
				results <- endpointResult{endpoint: endpoint, err: fmt.Errorf("RPC factory not initialized")}
				return
			}
			cli := factory.NewClient([]string{endpoint})
			height, err := cli.ChainHead(ctx)
			latencyMs := float64(time.Since(start).Milliseconds())

			// If we got a height, verify the block is actually available
			if err == nil && height > 0 {
				_, blockErr := cli.BlockByHeight(ctx, height)
				if blockErr != nil {
					// Head block not available yet, adjust height
					logger.Debug("Head block not yet available at endpoint, adjusting",
						zap.String("endpoint", endpoint),
						zap.Uint64("reported_head", height),
						zap.Uint64("adjusted_head", height-1),
					)
					height = height - 1
				}
			}

			results <- endpointResult{
				endpoint:  endpoint,
				height:    height,
				latencyMs: latencyMs,
				err:       err,
			}
		}(ep)
	}

	// Collect all results first, then batch update database
	var maxHeight uint64
	var healthyCount int
	healthUpdates := make([]*adminmodels.RPCEndpoint, 0, len(endpoints))

	for i := 0; i < len(endpoints); i++ {
		r := <-results

		status := "healthy"
		errMsg := ""
		if r.err != nil {
			status = "unreachable"
			errMsg = r.err.Error()
		}

		// Collect health update (will batch upsert below)
		healthUpdates = append(healthUpdates, &adminmodels.RPCEndpoint{
			ChainID:   ac.ChainID,
			Endpoint:  r.endpoint,
			Status:    status,
			Height:    r.height,
			LatencyMs: r.latencyMs,
			Error:     errMsg,
			UpdatedAt: time.Now().UTC(),
		})

		if r.err == nil {
			healthyCount++
			if r.height > maxHeight {
				maxHeight = r.height
			}
		}
	}

	// Batch upsert all endpoint health records in parallel for fast responses
	pool := pond.NewPool(len(healthUpdates))
	for _, update := range healthUpdates {
		update := update // capture loop variable
		pool.Submit(func() {
			if upsertErr := ac.AdminDB.UpsertEndpointHealth(ctx, update); upsertErr != nil {
				logger.Warn("failed to update endpoint health",
					zap.String("endpoint", update.Endpoint),
					zap.Error(upsertErr),
				)
			}
		})
	}
	pool.StopAndWait()

	// Update chain-level RPC health (aggregate status)
	if healthyCount == 0 {
		healthErr := ac.AdminDB.UpdateRPCHealth(
			ctx,
			ac.ChainID,
			"unreachable",
			"All RPC endpoints failed",
		)
		if healthErr != nil {
			logger.Warn("failed to update chain RPC health status",
				zap.Uint64("chain_id", ac.ChainID),
				zap.Error(healthErr),
			)
		}
		return 0, fmt.Errorf("all RPC endpoints unreachable for chain %d", ac.ChainID)
	}

	msg := fmt.Sprintf("%d/%d endpoints healthy, max height %d", healthyCount, len(endpoints), maxHeight)
	healthErr := ac.AdminDB.UpdateRPCHealth(ctx, ac.ChainID, "healthy", msg)
	if healthErr != nil {
		logger.Warn("failed to update chain RPC health status",
			zap.Uint64("chain_id", ac.ChainID),
			zap.Error(healthErr),
		)
	}

	logger.Debug("GetLatestHead completed",
		zap.Uint64("chain_id", ac.ChainID),
		zap.Int("healthy_endpoints", healthyCount),
		zap.Int("total_endpoints", len(endpoints)),
		zap.Uint64("max_height", maxHeight),
	)

	return maxHeight, nil
}

// FindGaps identifies and retrieves missing height ranges (gaps) in the indexing progress for a chain.
func (ac *Context) FindGaps(ctx context.Context) ([]adminmodels.Gap, error) {
	return ac.AdminDB.FindGaps(ctx, ac.ChainID)
}

// StartIndexWorkflow starts (or resumes) a top-level indexing workflow for a given height.
// Routes workflows to either the live or historical queue based on block age.
// NOTE: This is a local activity, so it accepts a value type (not pointer) for proper serialization.
func (ac *Context) StartIndexWorkflow(ctx context.Context, in types.ActivityIndexBlockInput) error {
	logger := activity.GetLogger(ctx)

	// Get latest head to determine queue routing
	latest, err := ac.GetLatestHead(ctx)
	if err != nil {
		logger.Warn("Failed to get latest head for queue routing, using historical queue",
			zap.Error(err),
		)
		latest = 0 // Default to historical queue on error
	}

	// Determine target queue based on block age
	var taskQueue string
	if utils.IsLiveBlock(latest, in.Height) {
		taskQueue = ac.ChainClient.LiveQueue
		logger.Debug("Routing to live queue",
			zap.String("task_queue", taskQueue),
			zap.Uint64("latest", latest),
			zap.Uint64("height", in.Height),
		)
	} else {
		taskQueue = ac.ChainClient.HistoricalQueue
		logger.Debug("Routing to historical queue",
			zap.String("task_queue", taskQueue),
			zap.Uint64("latest", latest),
			zap.Uint64("height", in.Height),
		)
	}

	wfID := ac.ChainClient.GetIndexBlockWorkflowID(in.Height)
	options := client.StartWorkflowOptions{
		ID:        wfID,
		TaskQueue: taskQueue, // UPDATED: Dynamic queue selection based on block age
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 1.2,
			MaximumInterval:    5 * time.Second,
			MaximumAttempts:    0,
		},
	}
	if in.PriorityKey != "" {
		if pk, parseErr := strconv.Atoi(in.PriorityKey); parseErr == nil && pk > 0 {
			options.Priority = sdktemporal.Priority{PriorityKey: pk}
		}
	}

	_, err = ac.ChainClient.TClient.ExecuteWorkflow(ctx, options, indexerworkflow.IndexBlockWorkflow2Name, in)
	if err != nil {
		var alreadyStarted *serviceerror.WorkflowExecutionAlreadyStarted
		if errors.As(err, &alreadyStarted) {
			return nil
		}
		logger.Error("failed to start workflow", zap.Error(err))
		return err
	}
	return nil
}

// IsSchedulerWorkflowRunning checks if the SchedulerWorkflow is currently running for a given chain.
// This activity uses Temporal's DescribeWorkflowExecution API to check the workflow status.
// Returns true if the workflow is running, false otherwise.
func (ac *Context) IsSchedulerWorkflowRunning(ctx context.Context) (bool, error) {
	logger := activity.GetLogger(ctx)

	wfID := ac.ChainClient.SchedulerWorkflowID

	// Query the workflow execution status
	desc, err := ac.ChainClient.TClient.DescribeWorkflowExecution(ctx, wfID, "")
	if err != nil {
		// If workflow not found, it's not running
		var notFound *serviceerror.NotFound
		if errors.As(err, &notFound) {
			return false, nil
		}
		logger.Warn("failed to describe scheduler workflow",
			zap.Uint64("chain_id", ac.ChainID),
			zap.String("workflow_id", wfID),
			zap.Error(err),
		)
		return false, err
	}

	// Check if workflow is still running
	workflowInfo := desc.WorkflowExecutionInfo
	if workflowInfo == nil {
		return false, nil
	}

	// Workflow is running if it doesn't have a close time
	isRunning := workflowInfo.CloseTime == nil

	logger.Debug("Scheduler workflow status checked",
		"chain_id", ac.ChainID,
		"workflow_id", wfID,
		"is_running", isRunning,
	)

	return isRunning, nil
}

// StartIndexWorkflowBatch schedules multiple IndexBlock workflows in a batch.
// Routes batches to appropriate queues (live or historical) based on block age.
// Splits batches that cross the live/historical boundary.
// This activity reduces Temporal API overhead by batching workflow start calls.
// Returns summary of scheduled/failed workflows and execution duration.
func (ac *Context) StartIndexWorkflowBatch(ctx context.Context, in types.ActivityBatchScheduleInput) (types.ActivityBatchScheduleOutput, error) {
	logger := activity.GetLogger(ctx)
	start := time.Now()

	logger.Info("Starting batch workflow scheduling",
		zap.Uint64("chain_id", ac.ChainID),
		zap.Uint64("start_height", in.StartHeight),
		zap.Uint64("end_height", in.EndHeight),
		zap.Int("batch_size", int(in.EndHeight-in.StartHeight+1)),
		zap.Int("priority", in.PriorityKey),
	)

	totalHeights := int(in.EndHeight - in.StartHeight + 1)
	if totalHeights <= 0 {
		durationMs := float64(time.Since(start).Microseconds()) / 1000.0
		logger.Info("Batch workflow scheduling completed",
			zap.Uint64("chain_id", ac.ChainID),
			zap.Int("scheduled", 0),
			zap.Int("failed", 0),
			zap.Float64("duration_ms", durationMs),
			zap.Float64("workflows_per_sec", 0),
		)

		return types.ActivityBatchScheduleOutput{
			Scheduled:  0,
			Failed:     0,
			DurationMs: durationMs,
		}, nil
	}

	// Get latest head to determine queue routing
	latest, err := ac.GetLatestHead(ctx)
	if err != nil {
		logger.Warn("Failed to get latest head for batch routing, using historical queue",
			zap.Error(err),
		)
		latest = 0 // Default to historical queue on error
	}

	// Determine if this batch is entirely live, entirely historical, or mixed
	batchIsLive := utils.IsLiveBlock(latest, in.StartHeight) && utils.IsLiveBlock(latest, in.EndHeight)
	batchIsHistorical := !utils.IsLiveBlock(latest, in.StartHeight) && !utils.IsLiveBlock(latest, in.EndHeight)

	// Handle mixed batches - split at the boundary
	if !batchIsLive && !batchIsHistorical {
		logger.Info("Mixed batch detected, splitting by queue",
			zap.Uint64("latest", latest),
			zap.Uint64("start", in.StartHeight),
			zap.Uint64("end", in.EndHeight),
		)

		// Find the boundary: latest - LiveBlockThreshold
		var boundary uint64
		if latest > utils.LiveBlockThreshold {
			boundary = latest - utils.LiveBlockThreshold
		} else {
			boundary = 0
		}

		var totalScheduled, totalFailed int

		// Schedule the historical portion (start to boundary) if it exists
		if in.StartHeight <= boundary && boundary < in.EndHeight {
			histEnd := boundary
			if histEnd > in.EndHeight {
				histEnd = in.EndHeight
			}

			histBatch := types.ActivityBatchScheduleInput{
				StartHeight: in.StartHeight,
				EndHeight:   histEnd,
				PriorityKey: in.PriorityKey,
			}

			logger.Info("Scheduling historical portion",
				zap.Uint64("start", histBatch.StartHeight),
				zap.Uint64("end", histBatch.EndHeight),
				zap.Int("count", int(histBatch.EndHeight-histBatch.StartHeight+1)),
			)

			histResult, err := ac.scheduleBatchToQueue(ctx, histBatch, ac.ChainClient.HistoricalQueue)
			if err != nil {
				logger.Error("Failed to schedule historical portion", zap.Error(err))
			} else {
				totalScheduled += histResult.Scheduled
				totalFailed += histResult.Failed
			}
		}

		// Schedule live portion (boundary+1 to end) if it exists
		if in.EndHeight > boundary {
			liveStart := boundary + 1
			if liveStart < in.StartHeight {
				liveStart = in.StartHeight
			}

			liveBatch := types.ActivityBatchScheduleInput{
				StartHeight: liveStart,
				EndHeight:   in.EndHeight,
				PriorityKey: in.PriorityKey,
			}

			logger.Info("Scheduling live portion",
				zap.Uint64("start", liveBatch.StartHeight),
				zap.Uint64("end", liveBatch.EndHeight),
				zap.Int("count", int(liveBatch.EndHeight-liveBatch.StartHeight+1)),
			)

			liveResult, err := ac.scheduleBatchToQueue(ctx, liveBatch, ac.ChainClient.LiveQueue)
			if err != nil {
				logger.Error("Failed to schedule live portion", zap.Error(err))
			} else {
				totalScheduled += liveResult.Scheduled
				totalFailed += liveResult.Failed
				// TODO: validate total duration times to ensure we report the times properly
				// totalDuration += liveResult.DurationMs
			}
		}

		overallDuration := float64(time.Since(start).Microseconds()) / 1000.0
		logger.Info("Split batch workflow scheduling completed",
			zap.Uint64("chain_id", ac.ChainID),
			zap.Int("total_scheduled", totalScheduled),
			zap.Int("total_failed", totalFailed),
			zap.Float64("overall_duration_ms", overallDuration),
		)

		return types.ActivityBatchScheduleOutput{
			Scheduled:  totalScheduled,
			Failed:     totalFailed,
			DurationMs: overallDuration,
		}, nil
	}

	// Single-queue batch (either entirely live or entirely historical)
	var taskQueue string
	if batchIsLive {
		taskQueue = ac.ChainClient.LiveQueue
		logger.Info("Batch routing to live queue",
			zap.String("task_queue", taskQueue),
			zap.Uint64("start", in.StartHeight),
			zap.Uint64("end", in.EndHeight),
		)
	} else {
		taskQueue = ac.ChainClient.HistoricalQueue
		logger.Info("Batch routing to historical queue",
			zap.String("task_queue", taskQueue),
			zap.Uint64("start", in.StartHeight),
			zap.Uint64("end", in.EndHeight),
		)
	}

	return ac.scheduleBatchToQueue(ctx, in, taskQueue)
}

// scheduleBatchToQueue is a helper function that schedules a batch of workflows to a specific queue.
// This function contains the core batch scheduling logic extracted from StartIndexWorkflowBatch.
func (ac *Context) scheduleBatchToQueue(ctx context.Context, in types.ActivityBatchScheduleInput, taskQueue string) (types.ActivityBatchScheduleOutput, error) {
	logger := activity.GetLogger(ctx)
	start := time.Now()

	var scheduled atomic.Int32
	var failed atomic.Int32

	totalHeights := int(in.EndHeight - in.StartHeight + 1)
	if totalHeights <= 0 {
		return types.ActivityBatchScheduleOutput{
			Scheduled:  0,
			Failed:     0,
			DurationMs: 0,
		}, nil
	}

	pool := ac.WorkerPool(totalHeights)
	group := pool.NewGroupContext(ctx)
	groupCtx := group.Context()

	for height := in.StartHeight; height <= in.EndHeight; height++ {
		h := height

		group.Submit(func() {
			if err := groupCtx.Err(); err != nil {
				return
			}

			wfID := ac.ChainClient.GetIndexBlockWorkflowID(h)
			options := client.StartWorkflowOptions{
				ID:        wfID,
				TaskQueue: taskQueue, // Use the provided task queue
				RetryPolicy: &sdktemporal.RetryPolicy{
					InitialInterval:    time.Second,
					BackoffCoefficient: 1.2,
					MaximumInterval:    5 * time.Second,
					MaximumAttempts:    0,
				},
			}
			if in.PriorityKey > 0 {
				options.Priority = sdktemporal.Priority{PriorityKey: in.PriorityKey}
			}

			_, err := ac.ChainClient.TClient.ExecuteWorkflow(groupCtx, options, indexerworkflow.IndexBlockWorkflow2Name, types.WorkflowIndexBlockInput{
				Height:      h,
				PriorityKey: strconv.Itoa(in.PriorityKey),
			})
			if err != nil {
				var alreadyStarted *serviceerror.WorkflowExecutionAlreadyStarted
				if errors.As(err, &alreadyStarted) {
					scheduled.Add(1) // Count as scheduled (idempotent)
					return
				}
				logger.Warn("failed to schedule workflow in batch",
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
		logger.Warn("batch scheduling group encountered error",
			zap.Uint64("chain_id", ac.ChainID),
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

	logger.Info("Batch workflow scheduling to queue completed",
		zap.Uint64("chain_id", ac.ChainID),
		zap.String("queue", taskQueue),
		zap.Int("scheduled", scheduledValue),
		zap.Int("failed", failedValue),
		zap.Float64("duration_ms", durationMs),
		zap.Float64("workflows_per_sec", rate),
		zap.Int("parallelism", parallelism),
	)

	return types.ActivityBatchScheduleOutput{
		Scheduled:  scheduledValue,
		Failed:     failedValue,
		DurationMs: durationMs,
	}, nil
}

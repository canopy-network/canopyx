package workflow

import (
	"strconv"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/activity"
	"github.com/canopy-network/canopyx/app/indexer/types"
	adminstore "github.com/canopy-network/canopyx/pkg/db/admin"
	indexer "github.com/canopy-network/canopyx/pkg/workflows/indexer"
	"go.temporal.io/api/enums/v1"
	sdktemporal "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	// Workflow names from contracts package
	HeadScanWorkflowName       = indexer.HeadScanWorkflowName
	GapScanWorkflowName        = indexer.GapScanWorkflowName
	IndexBlockWorkflowName     = indexer.IndexBlockWorkflowName
	SchedulerWorkflowName      = indexer.SchedulerWorkflowName
	CleanupStagingWorkflowName = indexer.CleanupStagingWorkflowName

	// --- Priority levels (time-based calculation)

	PriorityUltraHigh = 5 // Live blocks from HeadScan
	PriorityHigh      = 4 // Last 24 hours
	PriorityMedium    = 3 // 24-48 hours
	PriorityLow       = 2 // 48-72 hours
	PriorityUltraLow  = 1 // Older than 72 hours

	// --- Live Block Threshold (determines queue routing)

	// LiveBlockThreshold defines the number of blocks from the chain head
	// that are considered "live" for queue routing purposes.
	// Blocks within this threshold are routed to the live queue for low-latency indexing.
	// Blocks beyond this threshold are routed to the historical queue for bulk processing.
	LiveBlockThreshold = 200 // Blocks within 200 of head are considered "live"
)

// IsLiveBlock determines if a block should be routed to the live queue.
// Blocks within LiveBlockThreshold of the chain head are considered live.
// This function is used to route blocks to either the live queue (for low-latency indexing)
// or the historical queue (for bulk processing).
func IsLiveBlock(latest, height uint64) bool {
	return indexer.IsLiveBlock(latest, height)
}

var schedulerContinueThreshold = 5000 // ContinueAsNew after 5k blocks (more frequent to avoid large histories)

// SetSchedulerContinueThresholdForTesting allows tests to override the ContinueAsNew threshold.
// It returns a cleanup function that restores the previous value.
func SetSchedulerContinueThresholdForTesting(threshold int) func() {
	prev := schedulerContinueThreshold
	schedulerContinueThreshold = threshold
	return func() {
		schedulerContinueThreshold = prev
	}
}

// HeadScanInput extends ChainIdInput with continuation support for large ranges
type HeadScanInput struct {
	ChainID        uint64
	ResumeFrom     uint64 // If > 0, resume from this height (for ContinueAsNew)
	TargetLatest   uint64 // If > 0, use this as latest instead of querying (for ContinueAsNew)
	ProcessedSoFar uint64 // Track how many heights we've processed in this execution
}

// HeadScan scans the chain for missing blocks and starts indexing them.
// Uses adaptive scheduling: direct scheduling for small ranges, SchedulerWorkflow for large backlogs.
func (wc *Context) HeadScan(ctx workflow.Context, in HeadScanInput) (*activity.HeadResult, error) {
	logger := workflow.GetLogger(ctx)

	// Local activity options for fast DB queries and RPC calls
	localAo := workflow.LocalActivityOptions{
		ScheduleToCloseTimeout: 20 * time.Second,
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    500 * time.Millisecond,
			BackoffCoefficient: 2.0,
			MaximumInterval:    5 * time.Second,
			MaximumAttempts:    5,
		},
	}
	localCtx := workflow.WithLocalActivityOptions(ctx, localAo)

	// Regular activity options for IsSchedulerWorkflowRunning (Temporal API call)
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    10 * time.Second,
			MaximumAttempts:    5,
		},
		TaskQueue: wc.TemporalClient.GetIndexerOpsQueue(in.ChainID),
	}
	regularCtx := workflow.WithActivityOptions(ctx, ao)

	var latest, last uint64

	// Only query if not resuming from a continuation
	if in.TargetLatest > 0 {
		latest = in.TargetLatest
	} else {
		if err := workflow.ExecuteLocalActivity(localCtx, wc.ActivityContext.GetLatestHead, &types.ChainIdInput{ChainID: in.ChainID}).Get(localCtx, &latest); err != nil {
			return nil, err
		}
	}

	if in.ResumeFrom > 0 {
		// Resuming from continuation - use ResumeFrom as the starting point
		last = in.ResumeFrom - 1
	} else {
		if err := workflow.ExecuteLocalActivity(localCtx, wc.ActivityContext.GetLastIndexed, &types.ChainIdInput{ChainID: in.ChainID}).Get(localCtx, &last); err != nil {
			return nil, err
		}
	}

	if latest == 0 {
		return nil, sdktemporal.NewApplicationError("unable to get head block", "no_blocks_found", nil)
	}

	if latest <= last {
		return &activity.HeadResult{
			Start:  last,
			End:    last,
			Head:   last,
			Latest: latest,
		}, nil
	}

	rangeStart := last + 1
	rangeEnd := latest
	totalToProcess := rangeEnd - rangeStart + 1

	logger.Info("HeadScan starting",
		"chain_id", in.ChainID,
		"range_start", rangeStart,
		"range_end", rangeEnd,
		"total_heights", totalToProcess,
		"is_continuation", in.ResumeFrom > 0,
	)

	// Determine mode: normal (<1000 blocks) vs catch-up (>=1000 blocks)
	if totalToProcess < wc.Config.CatchupThreshold {
		// Normal mode: Direct scheduling with ultra high priority
		logger.Info("HeadScan using direct scheduling (normal mode)",
			"chain_id", in.ChainID,
			"total_blocks", totalToProcess,
		)
		if err := wc.scheduleDirectly(ctx, strconv.FormatUint(in.ChainID, 10), rangeStart, rangeEnd); err != nil {
			return nil, err
		}
	} else {
		// Catch-up mode: Check if SchedulerWorkflow is already running
		var isRunning bool
		if err := workflow.ExecuteActivity(regularCtx, wc.ActivityContext.IsSchedulerWorkflowRunning, &types.ChainIdInput{ChainID: in.ChainID}).Get(regularCtx, &isRunning); err != nil {
			logger.Warn("Failed to check if scheduler is running, proceeding with trigger",
				"chain_id", in.ChainID,
				"error", err.Error(),
			)
			isRunning = false
		}

		if isRunning {
			logger.Info("SchedulerWorkflow already running, skipping trigger",
				"chain_id", in.ChainID,
				"total_blocks", totalToProcess,
			)
		} else {
			// Trigger SchedulerWorkflow for large backlog
			logger.Info("HeadScan triggering SchedulerWorkflow (catch-up mode)",
				"chain_id", in.ChainID,
				"total_blocks", totalToProcess,
			)

			// Start SchedulerWorkflow asynchronously
			schedulerInput := types.SchedulerInput{
				ChainID:      in.ChainID,
				StartHeight:  rangeStart,
				EndHeight:    rangeEnd,
				LatestHeight: latest,
			}

			wfID := wc.TemporalClient.GetSchedulerWorkflowID(in.ChainID)
			wfOptions := workflow.ChildWorkflowOptions{
				WorkflowID:               wfID,
				TaskQueue:                wc.TemporalClient.GetIndexerOpsQueue(in.ChainID),
				WorkflowExecutionTimeout: 0,                                 // No timeout - workflow can run indefinitely
				WorkflowTaskTimeout:      10 * time.Minute,                  // 10 minute task timeout to prevent heartbeat errors
				ParentClosePolicy:        enums.PARENT_CLOSE_POLICY_ABANDON, // Don't terminate when parent completes
			}
			childCtx := workflow.WithChildOptions(ctx, wfOptions)

			// Start child workflow without waiting for completion
			future := workflow.ExecuteChildWorkflow(childCtx, wc.SchedulerWorkflow, schedulerInput)

			// Just check if workflow started successfully
			var childExec workflow.Execution
			if err := future.GetChildWorkflowExecution().Get(ctx, &childExec); err != nil {
				logger.Error("Failed to start SchedulerWorkflow",
					"chain_id", in.ChainID,
					"error", err.Error(),
				)
				return nil, err
			}

			logger.Info("SchedulerWorkflow started successfully",
				"chain_id", in.ChainID,
				"workflow_id", childExec.ID,
				"run_id", childExec.RunID,
			)
		}
	}

	logger.Info("HeadScan completed",
		"chain_id", in.ChainID,
		"total_processed", totalToProcess,
		"range_start", rangeStart,
		"range_end", rangeEnd,
	)

	return &activity.HeadResult{
		Start:  rangeStart,
		End:    rangeEnd,
		Head:   last,
		Latest: latest,
	}, nil
}

// GapScanInput is the input for GapScanWorkflow
type GapScanInput struct {
	ChainID uint64
}

// GapScanWorkflow periodically checks for missing heights and schedules their indexing.
// Uses adaptive scheduling: SchedulerWorkflow for large ranges (>=1000 blocks), direct for small ranges.
func (wc *Context) GapScanWorkflow(ctx workflow.Context, in GapScanInput) error {
	logger := workflow.GetLogger(ctx)

	// Local activity options for fast DB queries and RPC calls
	localAo := workflow.LocalActivityOptions{
		ScheduleToCloseTimeout: 20 * time.Second,
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    500 * time.Millisecond,
			BackoffCoefficient: 2.0,
			MaximumInterval:    5 * time.Second,
			MaximumAttempts:    5,
		},
	}
	localCtx := workflow.WithLocalActivityOptions(ctx, localAo)

	// Regular activity options for IsSchedulerWorkflowRunning (Temporal API call)
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    10 * time.Second,
			MaximumAttempts:    5,
		},
		TaskQueue: wc.TemporalClient.GetIndexerOpsQueue(in.ChainID),
	}
	regularCtx := workflow.WithActivityOptions(ctx, ao)

	// Query for gaps and the latest head
	var latest uint64
	if err := workflow.ExecuteLocalActivity(localCtx, wc.ActivityContext.GetLatestHead, &types.ChainIdInput{ChainID: in.ChainID}).Get(localCtx, &latest); err != nil {
		return err
	}

	var gaps []adminstore.Gap
	if err := workflow.ExecuteLocalActivity(localCtx, wc.ActivityContext.FindGaps, &types.ChainIdInput{ChainID: in.ChainID}).Get(localCtx, &gaps); err != nil {
		return err
	}

	logger.Info("GapScan starting",
		"chain_id", in.ChainID,
		"gaps_count", len(gaps),
		"latest", latest,
	)

	if len(gaps) == 0 {
		logger.Info("GapScan completed - no gaps found",
			"chain_id", in.ChainID,
		)
		return nil
	}

	// Calculate total blocks across all gaps
	var totalBlocks uint64
	for _, gap := range gaps {
		totalBlocks += gap.To - gap.From + 1
	}

	logger.Info("Gap analysis complete",
		"chain_id", in.ChainID,
		"total_gaps", len(gaps),
		"total_blocks", totalBlocks,
	)

	// Determine mode: normal (<1000 blocks) vs catch-up (>=1000 blocks)
	if totalBlocks < wc.Config.CatchupThreshold {
		// Normal mode: Process gaps directly with high priority
		logger.Info("GapScan using direct scheduling (small gaps)",
			"chain_id", in.ChainID,
			"total_blocks", totalBlocks,
		)

		for _, gap := range gaps {
			logger.Info("Scheduling gap directly",
				"chain_id", in.ChainID,
				"from", gap.From,
				"to", gap.To,
				"count", gap.To-gap.From+1,
			)

			if err := wc.scheduleDirectly(ctx, strconv.FormatUint(in.ChainID, 10), gap.From, gap.To); err != nil {
				return err
			}
		}
	} else {
		// Catch-up mode: Check if SchedulerWorkflow is already running
		var isRunning bool
		if err := workflow.ExecuteActivity(regularCtx, wc.ActivityContext.IsSchedulerWorkflowRunning, &types.ChainIdInput{ChainID: in.ChainID}).Get(regularCtx, &isRunning); err != nil {
			logger.Warn("Failed to check if scheduler is running, proceeding with trigger",
				"chain_id", in.ChainID,
				"error", err.Error(),
			)
			isRunning = false
		}

		if isRunning {
			logger.Info("SchedulerWorkflow already running, skipping trigger",
				"chain_id", in.ChainID,
				"total_blocks", totalBlocks,
			)
		} else {
			// Trigger SchedulerWorkflow for large gaps
			logger.Info("GapScan triggering SchedulerWorkflow (large gaps)",
				"chain_id", in.ChainID,
				"total_blocks", totalBlocks,
			)

			// Combine all gaps into a single range for scheduler
			// Find minimum From and maximum To across all gaps
			minHeight := gaps[0].From
			maxHeight := gaps[0].To
			for _, gap := range gaps[1:] {
				if gap.From < minHeight {
					minHeight = gap.From
				}
				if gap.To > maxHeight {
					maxHeight = gap.To
				}
			}

			schedulerInput := types.SchedulerInput{
				ChainID:      in.ChainID,
				StartHeight:  minHeight,
				EndHeight:    maxHeight,
				LatestHeight: latest,
			}

			wfID := wc.TemporalClient.GetSchedulerWorkflowID(in.ChainID)
			wfOptions := workflow.ChildWorkflowOptions{
				WorkflowID:               wfID,
				TaskQueue:                wc.TemporalClient.GetIndexerOpsQueue(in.ChainID),
				WorkflowExecutionTimeout: 0,                                 // No timeout - workflow can run indefinitely
				WorkflowTaskTimeout:      10 * time.Minute,                  // 10 minute task timeout to prevent heartbeat errors
				ParentClosePolicy:        enums.PARENT_CLOSE_POLICY_ABANDON, // Don't terminate when parent completes
			}
			childCtx := workflow.WithChildOptions(ctx, wfOptions)

			// Start child workflow without waiting for completion
			future := workflow.ExecuteChildWorkflow(childCtx, wc.SchedulerWorkflow, schedulerInput)

			// Just check if workflow started successfully
			var childExec workflow.Execution
			if err := future.GetChildWorkflowExecution().Get(ctx, &childExec); err != nil {
				logger.Error("Failed to start SchedulerWorkflow",
					"chain_id", in.ChainID,
					"error", err.Error(),
				)
				return err
			}

			logger.Info("SchedulerWorkflow started successfully",
				"chain_id", in.ChainID,
				"workflow_id", childExec.ID,
				"run_id", childExec.RunID,
			)
		}
	}

	logger.Info("GapScan completed",
		"chain_id", in.ChainID,
		"total_gaps", len(gaps),
		"total_blocks", totalBlocks,
	)

	return nil
}

// CalculateBlockPriority determines priority based on block age.
// Time-based priority calculation: blockAge = (latest - height) × blockTimeSeconds
// Priority levels:
// - Ultra High (5): Live blocks from HeadScan
// - High (4): Last 24 hours (4,320 blocks)
// - Medium (3): 24-48 hours (4,320 blocks)
// - Low (2): 48-72 hours (4,320 blocks)
// - Ultra Low (1): Older than 72 hours
func CalculateBlockPriority(latest, height, blockTimeSeconds uint64, isLive bool) int {
	if isLive {
		return PriorityUltraHigh
	}

	if height >= latest {
		return PriorityHigh
	}

	blockAge := (latest - height) * blockTimeSeconds // in seconds
	hoursAgo := blockAge / 3600

	switch {
	case hoursAgo <= 24:
		return PriorityHigh
	case hoursAgo <= 48:
		return PriorityMedium
	case hoursAgo <= 72:
		return PriorityLow
	default:
		return PriorityUltraLow
	}
}

// SchedulerWorkflow handles batch scheduling for large block ranges (e.g., 700k blocks).
// Optimized for Temporal stability:
// - Batches workflow starts to reduce API load
// - Uses activities for batches to reduce workflow history
// - ContinueAsNew every 5k blocks to avoid history bloat
func (wc *Context) SchedulerWorkflow(ctx workflow.Context, input types.SchedulerInput) error {
	logger := workflow.GetLogger(ctx)

	// Activity options for batched StartIndexWorkflow calls
	// Use regular activities (not local) to distribute load across workers
	// Timeout increased to 2 minutes to accommodate larger batch sizes (5000+ workflows)
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    200 * time.Millisecond,
			BackoffCoefficient: 1.2,
			MaximumInterval:    2 * time.Second,
			MaximumAttempts:    0,
		},
		TaskQueue: wc.TemporalClient.GetIndexerOpsQueue(input.ChainID),
	}
	activityCtx := workflow.WithActivityOptions(ctx, ao)

	startHeight := input.StartHeight
	endHeight := input.EndHeight
	processed := input.ProcessedSoFar

	logger.Info("SchedulerWorkflow starting",
		"chain_id", input.ChainID,
		"start", startHeight,
		"end", endHeight,
		"total", endHeight-startHeight+1,
		"processed_so_far", processed,
	)

	// Process blocks in batches from NEWEST to OLDEST (highest priority first)
	currentHeight := endHeight

	for currentHeight >= startHeight {
		// Calculate priority for current chunk
		priority := CalculateBlockPriority(input.LatestHeight, currentHeight, wc.Config.BlockTimeSeconds, false)

		// Determine batch start height (going backwards) without an uint64 underflow
		remaining := currentHeight - startHeight + 1
		var batchStartHeight uint64
		batchLimit := wc.Config.SchedulerBatchSize
		if batchLimit == 0 {
			batchLimit = 1
		}

		if remaining <= batchLimit {
			batchStartHeight = startHeight
		} else {
			batchStartHeight = currentHeight - batchLimit + 1
		}

		batchSize := int(currentHeight - batchStartHeight + 1)

		// Execute batch scheduling via activity
		// This moves the Temporal API calls into worker process, reducing workflow history
		var batchResult interface{}
		batchInput := types.BatchScheduleInput{
			ChainID:     input.ChainID,
			StartHeight: batchStartHeight,
			EndHeight:   currentHeight,
			PriorityKey: priority,
		}
		err := workflow.ExecuteActivity(activityCtx, wc.ActivityContext.StartIndexWorkflowBatch, batchInput).Get(activityCtx, &batchResult)
		if err != nil {
			logger.Error("Failed to schedule batch",
				"start_height", batchStartHeight,
				"end_height", currentHeight,
				"batch_size", batchSize,
				"error", err,
			)
			// Continue to next batch even on error (idempotent activity)
		}

		processed += uint64(batchSize)
		currentHeight = batchStartHeight - 1

		// Log progress every 500 blocks
		if processed%500 == 0 {
			logger.Info("SchedulerWorkflow progress",
				"chain_id", input.ChainID,
				"processed", processed,
				"current_height", currentHeight,
				"remaining", currentHeight-startHeight+1,
			)
		}

		// Check for ContinueAsNew threshold
		if processed >= uint64(schedulerContinueThreshold) && currentHeight >= startHeight {
			logger.Info("Triggering ContinueAsNew",
				"processed", processed,
				"next_height", currentHeight,
				"remaining", currentHeight-startHeight+1,
			)

			return workflow.NewContinueAsNewError(ctx, wc.SchedulerWorkflow, types.SchedulerInput{
				ChainID:        input.ChainID,
				StartHeight:    startHeight,
				EndHeight:      currentHeight,
				LatestHeight:   input.LatestHeight,
				ProcessedSoFar: 0, // Reset for new execution
			})
		}
	}

	logger.Info("SchedulerWorkflow completed",
		"total_processed", processed,
		"chain_id", input.ChainID,
	)

	return nil
}

// scheduleDirectly schedules small block ranges directly without SchedulerWorkflow.
// Used for ranges < CatchupThreshold blocks. Schedules in parallel batches with Ultra High priority.
// Uses local activities for performance with parallel execution via batched futures.
func (wc *Context) scheduleDirectly(ctx workflow.Context, chainID string, start, end uint64) error {
	logger := workflow.GetLogger(ctx)

	totalBlocks := end - start + 1
	logger.Info("Scheduling blocks directly",
		"chain_id", chainID,
		"start", start,
		"end", end,
		"total", totalBlocks,
	)

	// Local activity options
	localAo := workflow.LocalActivityOptions{
		ScheduleToCloseTimeout: 20 * time.Second,
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    500 * time.Millisecond,
			BackoffCoefficient: 2.0,
			MaximumInterval:    5 * time.Second,
			MaximumAttempts:    5,
		},
	}
	localCtx := workflow.WithLocalActivityOptions(ctx, localAo)

	// Parallel scheduling configuration
	// Process in chunks to control concurrency and avoid overwhelming the workflow
	const maxConcurrentPerChunk uint64 = 50

	batchSize := wc.Config.DirectScheduleBatchSize
	if batchSize == 0 {
		batchSize = 1
	}

	// Use maxConcurrentPerChunk as the effective batch size for parallel processing
	effectiveBatchSize := maxConcurrentPerChunk
	if batchSize > effectiveBatchSize {
		effectiveBatchSize = batchSize
	}

	var totalScheduled uint64
	var totalErrors int

	// Schedule in parallel chunks
	for currentHeight := start; currentHeight <= end; {
		chunkEnd := currentHeight + effectiveBatchSize - 1
		if chunkEnd > end {
			chunkEnd = end
		}
		chunkSize := chunkEnd - currentHeight + 1

		// Submit all heights in this chunk in parallel by collecting futures
		// ExecuteLocalActivity returns immediately with a Future, allowing parallel execution
		futures := make([]workflow.Future, 0, chunkSize)

		for h := currentHeight; h <= chunkEnd; h++ {
			height := h // Capture loop variable for closure

			// Start local activity asynchronously - returns immediately with Future
			// Convert chainID string to uint64 for the struct
			chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
			if parseErr != nil {
				logger.Error("Failed to parse chainID",
					"chain_id", chainID,
					"error", parseErr,
				)
				totalErrors++
				continue
			}
			future := workflow.ExecuteLocalActivity(localCtx, wc.ActivityContext.StartIndexWorkflow, types.IndexBlockInput{
				ChainID:     chainIDUint,
				Height:      height,
				PriorityKey: PriorityUltraHigh,
			})
			futures = append(futures, future)
		}

		// Wait for all futures in this chunk to complete
		for i, future := range futures {
			var result interface{}
			if err := future.Get(localCtx, &result); err != nil {
				height := currentHeight + uint64(i)
				logger.Error("Failed to schedule block directly",
					"height", height,
					"error", err,
				)
				totalErrors++
			} else {
				totalScheduled++
			}
		}

		// Log progress for larger ranges
		if totalBlocks > 100 && totalScheduled%100 == 0 {
			logger.Info("Direct scheduling progress",
				"chain_id", chainID,
				"scheduled", totalScheduled,
				"total", totalBlocks,
				"progress_pct", int((float64(totalScheduled)/float64(totalBlocks))*100),
			)
		}

		currentHeight = chunkEnd + 1
	}

	logger.Info("Direct scheduling completed",
		"chain_id", chainID,
		"total_scheduled", totalScheduled,
		"total_errors", totalErrors,
		"total_blocks", totalBlocks,
	)

	return nil
}

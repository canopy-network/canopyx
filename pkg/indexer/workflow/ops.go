package workflow

import (
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/indexer/activity"
	"github.com/canopy-network/canopyx/pkg/indexer/types"
	sdktemporal "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	HeadScanWorkflowName   = "HeadScanWorkflow"
	GapScanWorkflowName    = "GapScanWorkflow"
	IndexBlockWorkflowName = "IndexBlockWorkflow"
	SchedulerWorkflowName  = "SchedulerWorkflow"

	// --- Priority levels (time-based calculation)

	PriorityUltraHigh = 5 // Live blocks from HeadScan
	PriorityHigh      = 4 // Last 24 hours
	PriorityMedium    = 3 // 24-48 hours
	PriorityLow       = 2 // 48-72 hours
	PriorityUltraLow  = 1 // Older than 72 hours

	// --- Thresholds

	catchupThreshold        = 1000 // Switch to SchedulerWorkflow at this many blocks
	directScheduleBatchSize = 50   // Batch size for direct scheduling

	// --- SchedulerWorkflow settings

	schedulerBatchSize         = 500             // Schedule 100 workflows at a time
	schedulerBatchDelay        = 1 * time.Second // 1s between batches = 100 wf/sec
	schedulerContinueThreshold = 10000           // ContinueAsNew after 10k blocks

	// --- Block timing

	blockTime = 20 // seconds per block
)

// HeadScanInput extends ChainIdInput with continuation support for large ranges
type HeadScanInput struct {
	ChainID        string
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
	if totalToProcess < catchupThreshold {
		// Normal mode: Direct scheduling with ultra high priority
		logger.Info("HeadScan using direct scheduling (normal mode)",
			"chain_id", in.ChainID,
			"total_blocks", totalToProcess,
		)
		if err := wc.scheduleDirectly(ctx, in.ChainID, rangeStart, rangeEnd); err != nil {
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
				WorkflowExecutionTimeout: 0,                // No timeout - workflow can run indefinitely
				WorkflowTaskTimeout:      10 * time.Minute, // 10 minute task timeout to prevent heartbeat errors
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
	ChainID string
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

	var gaps []db.Gap
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
	if totalBlocks < catchupThreshold {
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

			if err := wc.scheduleDirectly(ctx, in.ChainID, gap.From, gap.To); err != nil {
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
				WorkflowExecutionTimeout: 0,                // No timeout - workflow can run indefinitely
				WorkflowTaskTimeout:      10 * time.Minute, // 10 minute task timeout to prevent heartbeat errors
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
// Time-based priority calculation: blockAge = (latest - height) × 20 seconds
// Priority levels:
// - Ultra High (5): Live blocks from HeadScan
// - High (4): Last 24 hours (4,320 blocks)
// - Medium (3): 24-48 hours (4,320 blocks)
// - Low (2): 48-72 hours (4,320 blocks)
// - Ultra Low (1): Older than 72 hours
func CalculateBlockPriority(latest, height uint64, isLive bool) int {
	if isLive {
		return PriorityUltraHigh
	}

	if height >= latest {
		return PriorityHigh
	}

	blockAge := (latest - height) * blockTime // in seconds
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
// Instead of building huge priority buckets in memory, it processes blocks sequentially
// in chunks, calculating priority dynamically for each chunk.
// Rate limiting: 100 workflows/sec (1s delay per 100 blocks).
// Uses ContinueAsNew every 10k blocks to avoid workflow history limits.
func (wc *Context) SchedulerWorkflow(ctx workflow.Context, input types.SchedulerInput) error {
	logger := workflow.GetLogger(ctx)

	// Local activity options for StartIndexWorkflow
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

	// Process blocks sequentially in chunks instead of building huge priority buckets
	// This avoids memory issues with large ranges (e.g., 700k blocks)
	currentHeight := startHeight

	for currentHeight <= endHeight {
		// Calculate priority for current chunk
		priority := CalculateBlockPriority(input.LatestHeight, currentHeight, false)

		// Process batch of blocks with same priority
		batchEnd := currentHeight + schedulerBatchSize - 1
		if batchEnd > endHeight {
			batchEnd = endHeight
		}

		// Schedule batch of blocks using local activities
		for height := currentHeight; height <= batchEnd; height++ {
			var result interface{}
			err := workflow.ExecuteLocalActivity(localCtx, wc.ActivityContext.StartIndexWorkflow, &types.IndexBlockInput{
				ChainID:     input.ChainID,
				Height:      height,
				PriorityKey: priority,
			}).Get(localCtx, &result)
			if err != nil {
				// Log error but continue - StartIndexWorkflow handles idempotency
				logger.Error("Failed to schedule block", "height", height, "error", err)
			}
			processed++
		}

		currentHeight = batchEnd + 1

		// Rate limiting: 100 workflows/sec (1s delay per batch)
		err := workflow.Sleep(ctx, schedulerBatchDelay)
		if err != nil {
			logger.Error("Failed to sleep", "error", err)
		}

		// Check for ContinueAsNew threshold
		if processed >= schedulerContinueThreshold && currentHeight <= endHeight {
			logger.Info("Triggering ContinueAsNew",
				"processed", processed,
				"next_height", currentHeight,
				"remaining", endHeight-currentHeight+1,
			)

			return workflow.NewContinueAsNewError(ctx, wc.SchedulerWorkflow, types.SchedulerInput{
				ChainID:        input.ChainID,
				StartHeight:    currentHeight,
				EndHeight:      endHeight,
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
// Used for ranges < 1000 blocks. Schedules in batches of 50 with Ultra High priority.
// Uses local activities for performance.
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

	// Schedule in batches
	for currentHeight := start; currentHeight <= end; {
		batchEnd := currentHeight + directScheduleBatchSize - 1
		if batchEnd > end {
			batchEnd = end
		}

		// Schedule batch with Ultra High priority (live blocks)
		for h := currentHeight; h <= batchEnd; h++ {
			var result interface{}
			err := workflow.ExecuteLocalActivity(localCtx, wc.ActivityContext.StartIndexWorkflow, &types.IndexBlockInput{
				ChainID:     chainID,
				Height:      h,
				PriorityKey: PriorityUltraHigh,
			}).Get(localCtx, &result)
			if err != nil {
				logger.Error("Failed to schedule block directly", "height", h, "error", err)
			}
		}

		currentHeight = batchEnd + 1
	}

	logger.Info("Direct scheduling completed",
		"chain_id", chainID,
		"total_scheduled", totalBlocks,
	)

	return nil
}

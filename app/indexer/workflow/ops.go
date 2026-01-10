package workflow

import (
    "strconv"
    "strings"
    "time"

    adminmodels "github.com/canopy-network/canopyx/pkg/db/models/admin"

    "github.com/canopy-network/canopyx/app/indexer/activity"
    "github.com/canopy-network/canopyx/app/indexer/types"
    "go.temporal.io/api/enums/v1"
    sdktemporal "go.temporal.io/sdk/temporal"
    "go.temporal.io/sdk/workflow"
)

const (
    // --- Priority levels (time-based calculation)

    PriorityUltraHigh = 5 // Live blocks from HeadScan
    PriorityHigh      = 4 // Last 24 hours
    PriorityMedium    = 3 // 24-48 hours
    PriorityLow       = 2 // 48-72 hours
    PriorityUltraLow  = 1 // Older than 72 hours
)

var schedulerContinueThreshold = 5000 // ContinueAsNew after 5k blocks (more frequent to avoid large histories)

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

// HeadScan scans the chain for missing blocks and starts indexing them.
// Uses adaptive scheduling: direct scheduling for small ranges, SchedulerWorkflow for large backlogs.
func (wc *Context) HeadScan(ctx workflow.Context, in types.WorkflowHeadScanInput) (*activity.HeadResult, error) {
    logger := workflow.GetLogger(ctx)

    // Local activity options for DB queries and RPC calls
    // Uses StartToCloseTimeout (not ScheduleToClose) since local activities run in-process without queue delay
    localAo := workflow.LocalActivityOptions{
        StartToCloseTimeout: 2 * time.Minute,
        RetryPolicy: &sdktemporal.RetryPolicy{
            InitialInterval:    500 * time.Millisecond,
            BackoffCoefficient: 2.0,
            MaximumInterval:    10 * time.Second,
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
        TaskQueue: wc.ChainClient.OpsQueue,
    }
    regularCtx := workflow.WithActivityOptions(ctx, ao)

    var latest, last uint64

    // Only query if not resuming from a continuation
    if in.TargetLatest > 0 {
        latest = in.TargetLatest
    } else {
        if err := workflow.ExecuteLocalActivity(localCtx, wc.ActivityContext.GetLatestHead).Get(localCtx, &latest); err != nil {
            return nil, err
        }
    }

    if in.ResumeFrom > 0 {
        // Resuming from continuation - use ResumeFrom as the starting point
        last = in.ResumeFrom - 1
    } else {
        if err := workflow.ExecuteLocalActivity(localCtx, wc.ActivityContext.GetLastIndexed).Get(localCtx, &last); err != nil {
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
        "chain_id", wc.ChainID,
        "range_start", rangeStart,
        "range_end", rangeEnd,
        "total_heights", totalToProcess,
        "is_continuation", in.ResumeFrom > 0,
    )

    // Determine mode: normal (<1000 blocks) vs catch-up (>=1000 blocks)
    if totalToProcess < wc.Config.CatchupThreshold {
        // Normal mode: Direct scheduling with ultra high priority
        logger.Info("HeadScan using direct scheduling (normal mode)",
            "chain_id", wc.ChainID,
            "total_blocks", totalToProcess,
        )
        if err := wc.scheduleDirectly(ctx, wc.ChainID, rangeStart, rangeEnd); err != nil {
            return nil, err
        }
    } else {
        // Catch-up mode: Check if SchedulerWorkflow is already running
        var isRunning bool
        if err := workflow.ExecuteLocalActivity(
            regularCtx,
            wc.ActivityContext.IsSchedulerWorkflowRunning,
        ).Get(regularCtx, &isRunning); err != nil {
            logger.Warn("Failed to check if scheduler is running, proceeding with trigger",
                "chain_id", wc.ChainID,
                "error", err.Error(),
            )
            isRunning = false
        }

        if isRunning {
            logger.Info("SchedulerWorkflow already running, skipping trigger",
                "chain_id", wc.ChainID,
                "total_blocks", totalToProcess,
            )
        } else {
            // Trigger SchedulerWorkflow for large backlog
            logger.Info("HeadScan triggering SchedulerWorkflow (catch-up mode)",
                "chain_id", wc.ChainID,
                "total_blocks", totalToProcess,
            )

            // Start SchedulerWorkflow asynchronously
            schedulerInput := types.WorkflowSchedulerInput{
                StartHeight:  rangeStart,
                EndHeight:    rangeEnd,
                LatestHeight: latest,
            }

            wfID := wc.ChainClient.SchedulerWorkflowID
            wfOptions := workflow.ChildWorkflowOptions{
                WorkflowID:               wfID,
                TaskQueue:                wc.ChainClient.OpsQueue,
                WorkflowExecutionTimeout: 0,                                 // No timeout - workflow can run indefinitely
                WorkflowTaskTimeout:      10 * time.Minute,                  // 10-minute task timeout to prevent heartbeat errors
                ParentClosePolicy:        enums.PARENT_CLOSE_POLICY_ABANDON, // Don't terminate when the parent completes
            }
            childCtx := workflow.WithChildOptions(ctx, wfOptions)

            // Start child workflow without waiting for completion
            future := workflow.ExecuteChildWorkflow(childCtx, wc.SchedulerWorkflow, schedulerInput)

            // Just check if workflow started successfully
            var childExec workflow.Execution
            if err := future.GetChildWorkflowExecution().Get(ctx, &childExec); err != nil {
                // Check if error is "already started" - this is not a failure, it means scheduler is already running
                if strings.Contains(err.Error(), "ChildWorkflowExecutionAlreadyStartedError") ||
                        strings.Contains(err.Error(), "child workflow execution already started") {
                    logger.Info("SchedulerWorkflow already running (concurrent start detected)",
                        "chain_id", wc.ChainID,
                    )
                    // Not an error - scheduler is already doing what we want
                } else {
                    logger.Error("Failed to start SchedulerWorkflow",
                        "chain_id", wc.ChainID,
                        "error", err.Error(),
                    )
                    return nil, err
                }
            } else {
                logger.Info("SchedulerWorkflow started successfully",
                    "chain_id", wc.ChainID,
                    "workflow_id", childExec.ID,
                    "run_id", childExec.RunID,
                )
            }
        }
    }

    logger.Info("HeadScan completed",
        "chain_id", wc.ChainID,
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

// GapScanWorkflow periodically checks for missing heights and schedules their indexing.
// For each gap, triggers SchedulerWorkflow (same as HeadScan) which handles batching and ContinueAsNew.
func (wc *Context) GapScanWorkflow(ctx workflow.Context) error {
    logger := workflow.GetLogger(ctx)

    // Local activity options for DB queries
    // Uses StartToCloseTimeout (not ScheduleToClose) since local activities run in-process without queue delay
    localAo := workflow.LocalActivityOptions{
        StartToCloseTimeout: 2 * time.Minute,
        RetryPolicy: &sdktemporal.RetryPolicy{
            InitialInterval:    500 * time.Millisecond,
            BackoffCoefficient: 2.0,
            MaximumInterval:    10 * time.Second,
            MaximumAttempts:    5,
        },
    }
    localCtx := workflow.WithLocalActivityOptions(ctx, localAo)

    // Query for gaps and latest height
    var latest uint64
    if err := workflow.ExecuteLocalActivity(localCtx, wc.ActivityContext.GetLatestHead).Get(localCtx, &latest); err != nil {
        return err
    }

    var gaps []adminmodels.Gap
    if err := workflow.ExecuteLocalActivity(localCtx, wc.ActivityContext.FindGaps).Get(localCtx, &gaps); err != nil {
        return err
    }

    logger.Info("GapScan starting",
        "chain_id", wc.ChainID,
        "gaps_count", len(gaps),
        "latest", latest,
    )

    if len(gaps) == 0 {
        logger.Info("GapScan completed - no gaps found",
            "chain_id", wc.ChainID,
        )
        return nil
    }

    // Calculate total blocks across all gaps
    var totalBlocks uint64
    for _, gap := range gaps {
        totalBlocks += gap.To - gap.From + 1
    }

    logger.Info("Gap analysis complete",
        "chain_id", wc.ChainID,
        "total_gaps", len(gaps),
        "total_blocks", totalBlocks,
    )

    // Trigger SchedulerWorkflow for each gap - same mechanism as HeadScan
    // SchedulerWorkflow handles batching, ContinueAsNew, and all the heavy lifting
    for i, gap := range gaps {
        gapSize := gap.To - gap.From + 1
        logger.Info("Triggering SchedulerWorkflow for gap",
            "chain_id", wc.ChainID,
            "gap_index", i+1,
            "total_gaps", len(gaps),
            "from", gap.From,
            "to", gap.To,
            "count", gapSize,
        )

        schedulerInput := types.WorkflowSchedulerInput{
            StartHeight:  gap.From,
            EndHeight:    gap.To,
            LatestHeight: latest,
        }

        // Use unique workflow ID per gap to allow parallel processing
        wfID := wc.ChainClient.GapSchedulerWorkflowID(gap.From, gap.To)
        wfOptions := workflow.ChildWorkflowOptions{
            WorkflowID:               wfID,
            TaskQueue:                wc.ChainClient.OpsQueue,
            WorkflowExecutionTimeout: 0,                                 // No timeout
            WorkflowTaskTimeout:      10 * time.Minute,                  // 10 minute task timeout
            ParentClosePolicy:        enums.PARENT_CLOSE_POLICY_ABANDON, // Don't terminate when parent completes
        }
        childCtx := workflow.WithChildOptions(ctx, wfOptions)

        // Start child workflow without waiting for completion
        future := workflow.ExecuteChildWorkflow(childCtx, wc.SchedulerWorkflow, schedulerInput)

        // Just check if workflow started successfully
        var childExec workflow.Execution
        if err := future.GetChildWorkflowExecution().Get(ctx, &childExec); err != nil {
            if strings.Contains(err.Error(), "ChildWorkflowExecutionAlreadyStartedError") ||
                    strings.Contains(err.Error(), "child workflow execution already started") {
                logger.Info("Gap SchedulerWorkflow already running",
                    "chain_id", wc.ChainID,
                    "gap_from", gap.From,
                    "gap_to", gap.To,
                )
            } else {
                logger.Error("Failed to start gap SchedulerWorkflow",
                    "chain_id", wc.ChainID,
                    "gap_from", gap.From,
                    "gap_to", gap.To,
                    "error", err.Error(),
                )
                return err
            }
        } else {
            logger.Info("Gap SchedulerWorkflow started",
                "chain_id", wc.ChainID,
                "workflow_id", childExec.ID,
                "gap_from", gap.From,
                "gap_to", gap.To,
            )
        }
    }

    logger.Info("GapScan completed",
        "chain_id", wc.ChainID,
        "total_gaps", len(gaps),
        "total_blocks", totalBlocks,
    )

    return nil
}

// SchedulerWorkflow handles batch scheduling for large block ranges (e.g., 700k blocks).
// Optimized for Temporal stability:
// - Batches workflow starts to reduce API load
// - Uses activities for batches to reduce workflow history
// - ContinueAsNew every 5k blocks to avoid history bloat
func (wc *Context) SchedulerWorkflow(ctx workflow.Context, input types.WorkflowSchedulerInput) error {
    logger := workflow.GetLogger(ctx)

    // Activity options for batched StartIndexWorkflow calls
    // Use regular activities (not local) to distribute load across workers
    // Timeout increased to 2 minutes to accommodate larger batch sizes (5000+ workflows)
    ao := workflow.LocalActivityOptions{
        StartToCloseTimeout: 2 * time.Minute,
        RetryPolicy: &sdktemporal.RetryPolicy{
            InitialInterval:    200 * time.Millisecond,
            BackoffCoefficient: 1.2,
            MaximumInterval:    2 * time.Second,
            MaximumAttempts:    0,
        },
    }
    activityCtx := workflow.WithLocalActivityOptions(ctx, ao)

    startHeight := input.StartHeight
    endHeight := input.EndHeight
    processed := input.ProcessedSoFar

    logger.Info("SchedulerWorkflow starting",
        "chain_id", wc.ChainID,
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
        // This moves the Temporal API calls into a worker process, reducing workflow history
        var batchResult interface{}
        batchInput := types.ActivityBatchScheduleInput{
            StartHeight: batchStartHeight,
            EndHeight:   currentHeight,
            PriorityKey: priority,
        }
        err := workflow.ExecuteLocalActivity(activityCtx, wc.ActivityContext.StartIndexWorkflowBatch, batchInput).Get(activityCtx, &batchResult)
        if err != nil {
            logger.Error("Failed to schedule batch",
                "start_height", batchStartHeight,
                "end_height", currentHeight,
                "batch_size", batchSize,
                "error", err,
            )
            // Continue to the next batch even on error (idempotent activity)
        }

        processed += uint64(batchSize)

        // Safe decrement - prevent uint64 underflow when batchStartHeight is 0
        if batchStartHeight == 0 {
            break // Reached height 0, all blocks processed
        }
        currentHeight = batchStartHeight - 1

        // Log progress every 500 blocks
        if processed%100 == 0 {
            logger.Info("SchedulerWorkflow progress",
                "chain_id", wc.ChainID,
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

            return workflow.NewContinueAsNewError(ctx, wc.SchedulerWorkflow, types.WorkflowSchedulerInput{
                StartHeight:    startHeight,
                EndHeight:      currentHeight,
                LatestHeight:   input.LatestHeight,
                ProcessedSoFar: 0, // Reset for new execution
            })
        }
    }

    logger.Info("SchedulerWorkflow completed",
        "total_processed", processed,
        "chain_id", wc.ChainID,
    )

    return nil
}

// scheduleDirectly schedules small block ranges directly without SchedulerWorkflow.
// Used for ranges < CatchupThreshold blocks. Schedules in parallel batches with Ultra High priority.
// Uses local activities for performance with parallel execution via batched futures.
func (wc *Context) scheduleDirectly(ctx workflow.Context, chainID uint64, start, end uint64) error {
    logger := workflow.GetLogger(ctx)

    totalBlocks := end - start + 1
    logger.Info("Scheduling blocks directly",
        "chain_id", chainID,
        "start", start,
        "end", end,
        "total", totalBlocks,
    )

    // Local activity options for StartIndexWorkflow calls
    // Uses StartToCloseTimeout (not ScheduleToClose) since local activities run in-process without queue delay
    localAo := workflow.LocalActivityOptions{
        StartToCloseTimeout: 2 * time.Minute,
        RetryPolicy: &sdktemporal.RetryPolicy{
            InitialInterval:    500 * time.Millisecond,
            BackoffCoefficient: 2.0,
            MaximumInterval:    10 * time.Second,
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
            future := workflow.ExecuteLocalActivity(localCtx, wc.ActivityContext.StartIndexWorkflow, types.ActivityIndexBlockInput{
                Height:      height,
                PriorityKey: strconv.Itoa(PriorityUltraHigh),
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

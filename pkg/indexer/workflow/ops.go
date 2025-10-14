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

    highPriorityKey   = 1
    mediumPriorityKey = 2
    lowPriorityKey    = 4
    newBlockWindow    = 100
    recentBlockWindow = 5000

    // Batching and continuation constants
    activityBatchSize        = 100  // Schedule 100 heights per batch
    continueAsNewThreshold   = 5000 // Continue as new after processing 5000 heights
    maxWorkflowHistoryEvents = 30000
)

// HeadScanInput extends ChainIdInput with continuation support for large ranges
type HeadScanInput struct {
    ChainID        string
    ResumeFrom     uint64 // If > 0, resume from this height (for ContinueAsNew)
    TargetLatest   uint64 // If > 0, use this as latest instead of querying (for ContinueAsNew)
    ProcessedSoFar uint64 // Track how many heights we've processed in this execution
}

// HeadScan scans the chain for missing blocks and starts indexing them.
// Uses asynchronous activity execution with batching and ContinueAsNew for large ranges.
func (wc *Context) HeadScan(ctx workflow.Context, in HeadScanInput) (*activity.HeadResult, error) {
    logger := workflow.GetLogger(ctx)
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
    ctx = workflow.WithActivityOptions(ctx, ao)

    var latest, last uint64
    var err error

    // Only query if not resuming from a continuation
    if in.TargetLatest > 0 {
        latest = in.TargetLatest
    } else {
        if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.GetLatestHead, &types.ChainIdInput{ChainID: in.ChainID}).Get(ctx, &latest); err != nil {
            return nil, err
        }
    }

    if in.ResumeFrom > 0 {
        // Resuming from continuation - use ResumeFrom as the starting point
        last = in.ResumeFrom - 1
    } else {
        if err = workflow.ExecuteActivity(ctx, wc.ActivityContext.GetLastIndexed, &types.ChainIdInput{ChainID: in.ChainID}).Get(ctx, &last); err != nil {
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

    processed := in.ProcessedSoFar
    currentHeight := rangeStart

    // Process in batches to avoid blocking on individual activities
    for currentHeight <= rangeEnd {
        batchEnd := currentHeight + activityBatchSize - 1
        if batchEnd > rangeEnd {
            batchEnd = rangeEnd
        }

        // Schedule a batch of heights asynchronously
        if err := wc.scheduleHeightBatch(ctx, in.ChainID, currentHeight, batchEnd, latest); err != nil {
            return nil, err
        }

        processed += batchEnd - currentHeight + 1
        currentHeight = batchEnd + 1

        // Check if we should continue as new to avoid history limits
        if processed >= continueAsNewThreshold && currentHeight <= rangeEnd {
            logger.Info("HeadScan continuing as new",
                "chain_id", in.ChainID,
                "processed_so_far", processed,
                "resume_from", currentHeight,
                "remaining", rangeEnd-currentHeight+1,
            )

            // Continue as new with updated state
            return nil, workflow.NewContinueAsNewError(ctx, wc.HeadScan, HeadScanInput{
                ChainID:        in.ChainID,
                ResumeFrom:     currentHeight,
                TargetLatest:   latest,
                ProcessedSoFar: 0, // Reset counter for new execution
            })
        }
    }

    logger.Info("HeadScan completed",
        "chain_id", in.ChainID,
        "total_processed", processed,
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

// GapScanInput extends ChainIdInput with continuation support for gap filling
type GapScanInput struct {
    ChainID        string
    ResumeGapIndex int      // Index of gap to resume from (for ContinueAsNew)
    ResumeHeight   uint64   // Height within gap to resume from (for ContinueAsNew)
    CachedGaps     []db.Gap // Cached gaps list (for ContinueAsNew)
    CachedLatest   uint64   // Cached latest height (for ContinueAsNew)
    ProcessedSoFar uint64   // Track how many heights we've processed
}

// GapScanWorkflow periodically checks for missing heights and (re)starts their indexing workflows.
// Uses asynchronous activity execution with batching and ContinueAsNew for large gap ranges.
func (wc *Context) GapScanWorkflow(ctx workflow.Context, in GapScanInput) error {
    logger := workflow.GetLogger(ctx)
    ao := workflow.ActivityOptions{
        StartToCloseTimeout: 2 * time.Minute,
        RetryPolicy: &sdktemporal.RetryPolicy{
            InitialInterval:    time.Second,
            BackoffCoefficient: 2.0,
            MaximumInterval:    30 * time.Second,
            MaximumAttempts:    10,
        },
        TaskQueue: wc.TemporalClient.GetIndexerOpsQueue(in.ChainID),
    }
    ctx = workflow.WithActivityOptions(ctx, ao)

    var latest, last uint64
    var gaps []db.Gap

    // Use cached values if resuming, otherwise query fresh data
    if len(in.CachedGaps) > 0 && in.CachedLatest > 0 {
        gaps = in.CachedGaps
        latest = in.CachedLatest
        // For last, we need to query fresh since it may have changed
        if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.GetLastIndexed, types.ChainIdInput{ChainID: in.ChainID}).Get(ctx, &last); err != nil {
            return err
        }
    } else {
        // Fresh query
        if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.GetLatestHead, types.ChainIdInput{ChainID: in.ChainID}).Get(ctx, &latest); err != nil {
            return err
        }
        if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.GetLastIndexed, types.ChainIdInput{ChainID: in.ChainID}).Get(ctx, &last); err != nil {
            return err
        }
        if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.FindGaps, types.ChainIdInput{ChainID: in.ChainID}).Get(ctx, &gaps); err != nil {
            return err
        }
    }

    logger.Info("GapScan starting",
        "chain_id", in.ChainID,
        "gaps_count", len(gaps),
        "latest", latest,
        "last_indexed", last,
        "is_continuation", in.ResumeGapIndex > 0 || in.ResumeHeight > 0,
    )

    processed := in.ProcessedSoFar
    startGapIndex := in.ResumeGapIndex

    // Process gaps
    for gapIdx := startGapIndex; gapIdx < len(gaps); gapIdx++ {
        gap := gaps[gapIdx]

        // Determine starting height within this gap
        startHeight := gap.From
        if gapIdx == startGapIndex && in.ResumeHeight > 0 {
            startHeight = in.ResumeHeight
        }

        logger.Info("Processing gap",
            "chain_id", in.ChainID,
            "gap_index", gapIdx,
            "gap_from", gap.From,
            "gap_to", gap.To,
            "start_height", startHeight,
        )

        // Process gap in batches
        for currentHeight := startHeight; currentHeight <= gap.To; {
            batchEnd := currentHeight + activityBatchSize - 1
            if batchEnd > gap.To {
                batchEnd = gap.To
            }

            if err := wc.scheduleHeightBatch(ctx, in.ChainID, currentHeight, batchEnd, latest); err != nil {
                return err
            }

            processed += (batchEnd - currentHeight + 1)
            currentHeight = batchEnd + 1

            // Check if we should continue as new
            if processed >= continueAsNewThreshold {
                logger.Info("GapScan continuing as new (within gap)",
                    "chain_id", in.ChainID,
                    "processed_so_far", processed,
                    "resume_gap_index", gapIdx,
                    "resume_height", currentHeight,
                )

                return workflow.NewContinueAsNewError(ctx, wc.GapScanWorkflow, GapScanInput{
                    ChainID:        in.ChainID,
                    ResumeGapIndex: gapIdx,
                    ResumeHeight:   currentHeight,
                    CachedGaps:     gaps,
                    CachedLatest:   latest,
                    ProcessedSoFar: 0,
                })
            }
        }
    }

    // Process any new blocks after last indexed
    if latest > last {
        logger.Info("Processing new blocks beyond last indexed",
            "chain_id", in.ChainID,
            "last_indexed", last,
            "latest", latest,
            "count", latest-last,
        )

        for currentHeight := last + 1; currentHeight <= latest; {
            batchEnd := currentHeight + activityBatchSize - 1
            if batchEnd > latest {
                batchEnd = latest
            }

            if err := wc.scheduleHeightBatch(ctx, in.ChainID, currentHeight, batchEnd, latest); err != nil {
                return err
            }

            processed += (batchEnd - currentHeight + 1)
            currentHeight = batchEnd + 1

            // Check if we should continue as new
            if processed >= continueAsNewThreshold && currentHeight <= latest {
                logger.Info("GapScan continuing as new (new blocks)",
                    "chain_id", in.ChainID,
                    "processed_so_far", processed,
                    "resume_height", currentHeight,
                )

                return workflow.NewContinueAsNewError(ctx, wc.GapScanWorkflow, GapScanInput{
                    ChainID:        in.ChainID,
                    ResumeGapIndex: len(gaps), // All gaps done, just continue with new blocks
                    ResumeHeight:   currentHeight,
                    CachedGaps:     nil, // No more gaps to process
                    CachedLatest:   latest,
                    ProcessedSoFar: 0,
                })
            }
        }
    }

    logger.Info("GapScan completed",
        "chain_id", in.ChainID,
        "total_processed", processed,
        "gaps_filled", len(gaps),
    )

    return nil
}

func priorityKeyForHeight(latest, height uint64) int {
    if height >= latest {
        return highPriorityKey
    }

    var diff uint64
    if latest > height {
        diff = latest - height
    }

    if diff <= newBlockWindow {
        return highPriorityKey
    }

    if diff <= recentBlockWindow {
        return mediumPriorityKey
    }

    return lowPriorityKey
}

// scheduleHeightBatch schedules a batch of heights for indexing.
// Uses workflow.Go() to execute activities concurrently within the workflow context.
// This ensures activities are properly tracked and won't be orphaned when the workflow completes.
func (wc *Context) scheduleHeightBatch(ctx workflow.Context, chainID string, start, end, latest uint64) error {
    logger := workflow.GetLogger(ctx)

    batchSize := end - start + 1
    logger.Debug("Scheduling height batch",
        "chain_id", chainID,
        "start", start,
        "end", end,
        "batch_size", batchSize,
    )

    // Schedule all activities and collect futures
    futures := make([]workflow.Future, 0, batchSize)

    for h := start; h <= end; h++ {
        height := h // Capture loop variable

        // Execute activity and collect the future
        future := workflow.ExecuteActivity(
            ctx,
            wc.ActivityContext.StartIndexWorkflow,
            &types.IndexBlockInput{
                ChainID:     chainID,
                Height:      height,
                PriorityKey: priorityKeyForHeight(latest, height),
            },
        )

        futures = append(futures, future)
    }

    // Now wait for all futures to complete
    for _, f := range futures {
        // Ignore errors - StartIndexWorkflow handles idempotency and retries
        // We just need to ensure the activity completes or fails
        _ = f.Get(ctx, nil)
    }

    logger.Debug("Height batch completed",
        "chain_id", chainID,
        "start", start,
        "end", end,
        "batch_size", batchSize,
    )

    return nil
}

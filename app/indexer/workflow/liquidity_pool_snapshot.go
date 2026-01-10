package workflow

import (
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	sdktemporal "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// LPSnapshotWorkflow computes LP position snapshots for a calendar day (UTC).
// This workflow is executed via a scheduled workflow (hourly per chain).
//
// ARCHITECTURAL NOTE: Unlike other indexing workflows, this is NOT height-based.
// It computes snapshots for calendar days by:
// 1. Reading the scheduled time from Temporal search attributes (or workflow.Now())
// 2. Determining the target date from the scheduled time
// 3. Executing ComputeLPSnapshots activity with the target date
//
// The activity queries per-chain pool_points_by_holder tables at the snapshot height
// (highest block with time <= 23:59:59 UTC on target date) and writes snapshots
// directly to the cross-chain lp_position_snapshots_global table.
//
// Scheduled execution:
// - Runs hourly per chain (like HeadScan/GapScan pattern)
// - Recomputes "today" multiple times (ReplacingMergeTree keeps newest)
// - Finalizes "yesterday" after UTC boundary
//
// Backfill execution:
// - Triggered manually via Admin API using Temporal's built-in schedule backfill
// - Each backfilled execution reads its TemporalScheduledStartTime to know which date to compute
// - Temporal executes one workflow per schedule interval in the backfill range
func (wc *Context) LPSnapshotWorkflow(ctx workflow.Context) (types.ActivityComputeLPSnapshotsOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting LP snapshot workflow")

	// Get scheduled time from workflow info
	// For scheduled workflows (both regular and backfilled), Temporal provides the scheduled start time
	// This is critical for backfills to compute snapshots for the correct historical dates
	info := workflow.GetInfo(ctx)
	scheduledTime := info.WorkflowStartTime

	// For scheduled workflows, check if ScheduledTime search attribute is set (more accurate)
	// This is set by Temporal's schedule feature and contains the exact scheduled time
	// Using typed search attributes API (GetTypedSearchAttributes) instead of deprecated info.SearchAttributes
	temporalScheduledStartTimeKey := sdktemporal.NewSearchAttributeKeyTime("TemporalScheduledStartTime")
	searchAttrs := workflow.GetTypedSearchAttributes(ctx)
	if ts, ok := searchAttrs.GetTime(temporalScheduledStartTimeKey); ok {
		scheduledTime = ts
		logger.Info("Using TemporalScheduledStartTime from search attributes",
			"scheduled_time", scheduledTime)
	}

	// Determine target date from scheduled time (calendar day UTC)
	targetDate := time.Date(scheduledTime.Year(), scheduledTime.Month(), scheduledTime.Day(), 0, 0, 0, 0, time.UTC)

	logger.Info("Determined target date",
		"target_date", targetDate,
		"scheduled_time", scheduledTime)

	// Activity options for snapshot computation
	// Allow up to 2 minutes for the activity (database queries can be slow for large datasets)
	// Heartbeat every 30 seconds to track progress through long-running queries
	ao := workflow.LocalActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    500 * time.Millisecond,
			BackoffCoefficient: 2.0,
			MaximumInterval:    10 * time.Second,
			MaximumAttempts:    3, // Limited retries - if snapshot fails, wait for next scheduled run
		},
	}
	ctx = workflow.WithLocalActivityOptions(ctx, ao)

	// Execute ComputeLPSnapshots activity
	var result types.ActivityComputeLPSnapshotsOutput
	err := workflow.ExecuteLocalActivity(ctx, wc.ActivityContext.ComputeLPSnapshots, types.ActivityComputeLPSnapshotsInput{
		TargetDate: targetDate,
	}).Get(ctx, &result)

	if err != nil {
		logger.Error("LP snapshot activity failed",
			"target_date", targetDate,
			"error", err.Error())
		return result, err
	}

	logger.Info("LP snapshot workflow completed",
		"target_date", result.TargetDate,
		"total_snapshots", result.TotalSnapshots,
		"duration_ms", result.DurationMs)

	return result, nil
}

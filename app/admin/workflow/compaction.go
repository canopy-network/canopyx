package workflow

import (
	"time"

	"github.com/canopy-network/canopyx/app/admin/types"

	sdktemporal "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// CompactCrossChainTablesWorkflow orchestrates the compaction of all cross-chain global tables.
// This workflow:
// 1. Executes OPTIMIZE TABLE FINAL on all 11 global tables sequentially
// 2. Logs a summary of results
//
// Runs hourly via Temporal schedule (configured at startup).
func (wc *Context) CompactCrossChainTablesWorkflow(ctx workflow.Context) (types.ActivityCompactAllGlobalTablesOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting cross-chain table compaction workflow")

	// Activity options for compaction
	// Allow up to 30 minutes for all tables (should be much faster, but be conservative)
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Minute,
		HeartbeatTimeout:    2 * time.Minute, // Heartbeat every 2 minutes (reported after each table)
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    1 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3, // Limited retries - if compaction fails, log it and continue
		},
		TaskQueue: "maintenance", // Simplified admin queue name (in canopyx namespace)
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Execute compaction activity
	var result types.ActivityCompactAllGlobalTablesOutput
	err := workflow.ExecuteActivity(ctx, wc.ActivityContext.CompactAllGlobalTables,
		types.ActivityCompactAllGlobalTablesInput{}).Get(ctx, &result)

	if err != nil {
		logger.Error("Cross-chain table compaction activity failed", "error", err.Error())
		return result, err
	}

	// Log summary using local activity (fast, no persistence needed)
	localAo := workflow.LocalActivityOptions{
		ScheduleToCloseTimeout: 10 * time.Second,
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    500 * time.Millisecond,
			BackoffCoefficient: 2.0,
			MaximumInterval:    2 * time.Second,
			MaximumAttempts:    2,
		},
	}
	localCtx := workflow.WithLocalActivityOptions(ctx, localAo)

	err = workflow.ExecuteLocalActivity(localCtx, wc.ActivityContext.LogCompactionSummary, result).Get(localCtx, nil)
	if err != nil {
		logger.Warn("Failed to log compaction summary", "error", err.Error())
		// Non-critical error, don't fail the workflow
	}

	logger.Info("Cross-chain table compaction workflow completed",
		"total_tables", result.TotalTables,
		"success_count", result.SuccessCount,
		"failure_count", result.FailureCount,
		"total_duration", result.TotalDuration.String())

	return result, nil
}

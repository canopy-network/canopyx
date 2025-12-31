package workflow

import (
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"

	sdktemporal "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// PollSnapshotWorkflow captures governance poll data snapshots every 5 minutes.
// This workflow:
// 1. Executes IndexPoll activity to fetch current poll state from RPC
// 2. Inserts snapshots directly to production poll_snapshots table
//
// Runs on a 5-minute schedule via Temporal schedule (configured at startup).
//
// ARCHITECTURAL NOTE: Unlike other indexing workflows, this is NOT height-based.
// The /v1/gov/poll RPC endpoint does not support historical queries, so we capture
// time-series snapshots of the current state.
func (wc *Context) PollSnapshotWorkflow(ctx workflow.Context) (types.ActivityIndexPollOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting poll snapshot workflow")

	// Activity options for poll snapshot
	// Allow up to 30 seconds for the RPC call and database insert
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    500 * time.Millisecond,
			BackoffCoefficient: 2.0,
			MaximumInterval:    5 * time.Second,
			MaximumAttempts:    3, // Limited retries - if snapshot fails, wait for next scheduled run
		},
		TaskQueue: wc.TemporalClient.GetIndexerOpsQueue(wc.ChainID),
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Execute poll snapshot activity
	var result types.ActivityIndexPollOutput
	err := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexPoll).Get(ctx, &result)

	if err != nil {
		logger.Error("Poll snapshot activity failed", "error", err.Error())
		return result, err
	}

	logger.Info("Poll snapshot workflow completed",
		"num_proposals", result.NumProposals,
		"duration_ms", result.DurationMs)

	return result, nil
}

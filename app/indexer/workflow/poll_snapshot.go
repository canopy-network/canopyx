package workflow

import (
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"

	sdktemporal "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// PollSnapshotWorkflow runs on a schedule (every 5 minutes) to capture snapshots of governance polls.
//
// ARCHITECTURAL NOTE: This workflow does NOT take block height as input because the /v1/gov/polls
// RPC endpoint doesn't support historical queries. Instead, we capture time-series snapshots of the
// current state at regular intervals. This differs from height-based indexing workflows.
//
// This workflow:
// 1. Executes IndexPoll activity to fetch the current poll state from RPC
// 2. Inserts snapshots directly to the poll_snapshots table
//
// Retry strategy:
//   - Limited to 5 attempts (snapshots can wait for the next scheduled run)
//   - Exponential backoff (500ms initial, 2.0 coefficient, 5s max)
//   - 30-second timeout per activity execution
func (wc *Context) PollSnapshotWorkflow(ctx workflow.Context) (types.ActivityIndexPollOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting poll snapshot workflow")

	// Activity options for poll snapshot
	// Allow up to 30 seconds for the RPC call and database insert
	ao := workflow.LocalActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    500 * time.Millisecond,
			BackoffCoefficient: 2.0,
			MaximumInterval:    5 * time.Second,
			MaximumAttempts:    5, // Limited retries - if the snapshot fails, wait for the next scheduled run
		},
	}
	ctx = workflow.WithLocalActivityOptions(ctx, ao)

	// Execute poll snapshot activity
	var result types.ActivityIndexPollOutput
	if err := workflow.ExecuteLocalActivity(ctx, wc.ActivityContext.IndexPoll).Get(ctx, &result); err != nil {
		return types.ActivityIndexPollOutput{}, err
	}

	logger.Info("Poll snapshot workflow completed",
		"num_proposals", result.NumProposals,
		"duration_ms", result.DurationMs)

	return result, nil
}

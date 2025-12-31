package workflow

import (
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	sdktemporal "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// ProposalSnapshotWorkflow runs on a schedule (every 5 minutes) to capture snapshots of governance proposals.
//
// ARCHITECTURAL NOTE: This workflow does NOT take block height as input because the /v1/gov/proposals
// RPC endpoint doesn't support historical queries. Instead, we capture time-series snapshots of the
// current state at regular intervals. This differs from height-based indexing workflows.
//
// The workflow:
// 1. Executes the IndexProposals activity to fetch current proposals and insert snapshots
// 2. Returns the number of proposals captured
//
// Retry strategy:
//   - Limited to 3 attempts (snapshots can wait for next scheduled run)
//   - Exponential backoff (500ms initial, 2.0 coefficient, 5s max)
//   - 30-second timeout per activity execution
func (wc *Context) ProposalSnapshotWorkflow(ctx workflow.Context) (types.ActivityIndexProposalsOutput, error) {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    500 * time.Millisecond,
			BackoffCoefficient: 2.0,
			MaximumInterval:    5 * time.Second,
			MaximumAttempts:    5, // Limited retries (snapshot can wait for the next run)
		},
	}

	ctx = workflow.WithActivityOptions(ctx, ao)

	// Execute the IndexProposals activity
	var output types.ActivityIndexProposalsOutput
	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexProposals).Get(ctx, &output); err != nil {
		return types.ActivityIndexProposalsOutput{}, err
	}

	return output, nil
}

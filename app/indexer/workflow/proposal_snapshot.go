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
// 1. Executes the IndexProposals activity to fetch the current proposals state from RPC
// 2. Inserts snapshots directly to the proposal_snapshots table
//
// Retry strategy:
//   - Limited to 5 attempts (snapshots can wait for the next scheduled run)
//   - Exponential backoff (500ms initial, 2.0 coefficient, 5s max)
//   - 30-second timeout per activity execution
func (wc *Context) ProposalSnapshotWorkflow(ctx workflow.Context) (types.ActivityIndexProposalsOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting proposal snapshot workflow")

	ao := workflow.LocalActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    500 * time.Millisecond,
			BackoffCoefficient: 2.0,
			MaximumInterval:    5 * time.Second,
			MaximumAttempts:    5, // Limited retries (snapshot can wait for the next run)
		},
	}

	ctx = workflow.WithLocalActivityOptions(ctx, ao)

	// Execute the IndexProposals activity
	var result types.ActivityIndexProposalsOutput
	if err := workflow.ExecuteLocalActivity(ctx, wc.ActivityContext.IndexProposals).Get(ctx, &result); err != nil {
		return types.ActivityIndexProposalsOutput{}, err
	}

	logger.Info("Proposal snapshot workflow completed",
		"num_proposals", result.NumProposals,
		"duration_ms", result.DurationMs)

	return result, nil
}

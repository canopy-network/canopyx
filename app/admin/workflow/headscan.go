package workflow

import (
	"time"

	"github.com/canopy-network/canopyx/app/admin/activity"

	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// HeadScan scans the chain for missing blocks and starts indexing them.
func (wc *Context) HeadScan(ctx workflow.Context, in types.ChainIdInput) (*activity.HeadResult, error) {
	act := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    10 * time.Second,
			MaximumAttempts:    5,
		},
		TaskQueue: wc.TemporalClient.ManagerQueue,
	}
	ctx = workflow.WithActivityOptions(ctx, act)

	var latest, last uint64

	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.GetLatestHead, &in).Get(ctx, &latest); err != nil {
		return nil, err
	}
	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.GetLastIndexed, &in).Get(ctx, &last); err != nil {
		return nil, err
	}

	if latest == 0 {
		return nil, temporal.NewApplicationError("unable to get head block", "no_blocks_found", nil)
	}

	if latest <= last {
		return &activity.HeadResult{
			Latest: latest,
			Head:   last,
		}, nil
	}

	rangeStart := last + 1
	for h := rangeStart; h <= latest; h++ {
		if err := workflow.ExecuteActivity(
			ctx,
			wc.ActivityContext.StartIndexWorkflow,
			types.IndexBlockInput{ChainID: in.ChainID, Height: h},
		).Get(ctx, nil); err != nil {
			return nil, err
		}
	}

	return &activity.HeadResult{
		Latest: latest,
		Head:   last,
	}, nil
}

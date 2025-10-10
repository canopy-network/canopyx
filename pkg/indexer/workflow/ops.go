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
	HeadScanWorkflowName = "HeadScanWorkflow"
	GapScanWorkflowName  = "GapScanWorkflow"

	highPriorityKey   = 1
	mediumPriorityKey = 2
	lowPriorityKey    = 4
	newBlockWindow    = 100
	recentBlockWindow = 5000
)

// HeadScan scans the chain for missing blocks and starts indexing them.
func (wc *Context) HeadScan(ctx workflow.Context, in types.ChainIdInput) (*activity.HeadResult, error) {
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

	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.GetLatestHead, &in).Get(ctx, &latest); err != nil {
		return nil, err
	}
	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.GetLastIndexed, &in).Get(ctx, &last); err != nil {
		return nil, err
	}

	if latest == 0 {
		return nil, sdktemporal.NewApplicationError("unable to get head block", "no_blocks_found", nil)
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
			&types.IndexBlockInput{ChainID: in.ChainID, Height: h, PriorityKey: priorityKeyForHeight(latest, h)},
		).Get(ctx, nil); err != nil {
			return nil, err
		}
	}

	return &activity.HeadResult{
		Latest: latest,
		Head:   last,
	}, nil
}

// GapScanWorkflow periodically checks for missing heights and (re)starts their indexing workflows.
func (wc *Context) GapScanWorkflow(ctx workflow.Context, in types.ChainIdInput) error {
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
	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.GetLatestHead, in).Get(ctx, &latest); err != nil {
		return err
	}
	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.GetLastIndexed, in).Get(ctx, &last); err != nil {
		return err
	}

	var gaps []db.Gap
	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.FindGaps, in).Get(ctx, &gaps); err != nil {
		return err
	}

	for _, g := range gaps {
		for h := g.From; h <= g.To; h++ {
			if err := workflow.ExecuteActivity(
				ctx,
				wc.ActivityContext.StartIndexWorkflow,
				&types.IndexBlockInput{ChainID: in.ChainID, Height: h, PriorityKey: priorityKeyForHeight(latest, h)},
			).Get(ctx, nil); err != nil {
				return err
			}
		}
	}

	if latest > last {
		for h := last + 1; h <= latest; h++ {
			if err := workflow.ExecuteActivity(
				ctx,
				wc.ActivityContext.StartIndexWorkflow,
				&types.IndexBlockInput{ChainID: in.ChainID, Height: h, PriorityKey: priorityKeyForHeight(latest, h)},
			).Get(ctx, nil); err != nil {
				return err
			}
		}
	}
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

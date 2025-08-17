package workflow

import (
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// GapScanWorkflow periodically checks for missing heights and (re)starts their indexing workflows.
func (wc *Context) GapScanWorkflow(ctx workflow.Context, in types.ChainIdInput) error {
	// activity defaults
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    10,
		},
		TaskQueue: wc.TemporalClient.ManagerQueue,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Get current head and last indexed (for a tail gap)
	var latest, last uint64
	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.GetLatestHead, in).Get(ctx, &latest); err != nil {
		return err
	}
	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.GetLastIndexed, in).Get(ctx, &last); err != nil {
		return err
	}

	// Ask DB for internal gaps from audit log
	var gaps []db.Gap
	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.FindGaps, in).Get(ctx, &gaps); err != nil {
		return err
	}

	// Emit all heights in gaps
	for _, g := range gaps {
		for h := g.From; h <= g.To; h++ {
			if err := workflow.ExecuteActivity(
				ctx,
				wc.ActivityContext.StartIndexWorkflow,
				types.IndexBlockInput{
					ChainID: in.ChainID,
					Height:  h,
				},
			).Get(ctx, nil); err != nil {
				return err
			}
		}
	}

	// Tail gap from last indexed to the latest head
	if latest > last {
		for h := last + 1; h <= latest; h++ {
			if err := workflow.ExecuteActivity(
				ctx,
				wc.ActivityContext.StartIndexWorkflow,
				types.IndexBlockInput{
					ChainID: in.ChainID,
					Height:  h,
				},
			).Get(ctx, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

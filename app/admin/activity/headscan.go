package activity

import (
	"context"
	"errors"
	"time"

	"go.temporal.io/sdk/temporal"

	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
)

type HeadResult struct {
	Start  uint64
	End    uint64
	Head   uint64
	Latest uint64
}

// GetLastIndexed returns the last indexed block height for the given chain.
func (c *Context) GetLastIndexed(ctx context.Context, in *types.ChainIdInput) (uint64, error) {
	return c.IndexerDB.LastIndexed(ctx, in.ChainID)
}

// GetLatestHead returns the latest block height for the given chain.
func (c *Context) GetLatestHead(ctx context.Context, in *types.ChainIdInput) (uint64, error) {
	ch, err := c.IndexerDB.GetChain(ctx, in.ChainID)
	if err != nil {
		return uint64(0), err
	}

	cli := rpc.NewHTTPWithOpts(rpc.Opts{
		Endpoints: ch.RPCEndpoints,
		RPS:       20, Burst: 40, BreakerFailures: 3, BreakerCooldown: 5 * time.Second,
	})
	return cli.ChainHead(ctx)
}

// StartIndexWorkflow starts a top-level workflow for a given height (idempotent by WorkflowID).
func (c *Context) StartIndexWorkflow(ctx context.Context, in *types.IndexBlockInput) error {
	logger := activity.GetLogger(ctx)
	// TODO: ensure chain exists at least before trigger workflows to index it.

	wfID := c.TemporalClient.GetIndexBlockWorkflowId(in.ChainID, in.Height)
	_, err := c.TemporalClient.TClient.ExecuteWorkflow(
		ctx,
		client.StartWorkflowOptions{
			ID:        wfID,
			TaskQueue: c.TemporalClient.GetIndexerQueue(in.ChainID),
			RetryPolicy: &temporal.RetryPolicy{
				InitialInterval:    time.Second,
				BackoffCoefficient: 1.2,
				MaximumInterval:    5 * time.Second, // we need it fast since blocks are the 20s apart
				MaximumAttempts:    0,               // zero means unlimited - we want it keeps trying to index a block until it succeeds
			},
			// TODO: double check the right options for the workflow here, like re-use policy, timeouts, retries etc.
		},
		// NOTE: reference by name otherwise we get cyclic imports
		"IndexBlockWorkflow",
		in,
	)

	if err != nil {
		var workflowExecutionAlreadyStarted *serviceerror.WorkflowExecutionAlreadyStarted
		if errors.As(err, &workflowExecutionAlreadyStarted) {
			return nil
		}
		logger.Error("failed to start workflow", zap.Error(err))
		return err
	}

	return nil
}

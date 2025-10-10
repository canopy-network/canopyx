package activity

import (
	"context"
	"errors"
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	sdktemporal "go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// HeadResult summarizes the latest and last indexed heights produced by HeadScan.
type HeadResult struct {
	Start  uint64
	End    uint64
	Head   uint64
	Latest uint64
}

// PrepareIndexBlock determines whether a block needs reindexing and performs cleanup if required.
// Returns true when the caller should skip further indexing work.
func (c *Context) PrepareIndexBlock(ctx context.Context, in types.IndexBlockInput) (bool, error) {
	chainDb, err := c.NewChainDb(ctx, in.ChainID)
	if err != nil {
		return false, sdktemporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", err)
	}

	exists, err := chainDb.HasBlock(ctx, in.Height)
	if err != nil {
		return false, sdktemporal.NewApplicationErrorWithCause("block_lookup_failed", "chain_db_error", err)
	}

	if !in.Reindex && exists {
		return true, nil
	}

	if in.Reindex {
		if err := chainDb.DeleteTransactions(ctx, in.Height); err != nil {
			return false, sdktemporal.NewApplicationErrorWithCause("delete_transactions_failed", "chain_db_error", err)
		}
		if err := chainDb.DeleteBlock(ctx, in.Height); err != nil {
			return false, sdktemporal.NewApplicationErrorWithCause("delete_block_failed", "chain_db_error", err)
		}
	}

	return false, nil
}

// GetLastIndexed returns the last indexed block height for the given chain.
func (c *Context) GetLastIndexed(ctx context.Context, in *types.ChainIdInput) (uint64, error) {
	return c.IndexerDB.LastIndexed(ctx, in.ChainID)
}

// GetLatestHead returns the latest block height for the given chain by hitting RPC endpoints.
func (c *Context) GetLatestHead(ctx context.Context, in *types.ChainIdInput) (uint64, error) {
	ch, err := c.IndexerDB.GetChain(ctx, in.ChainID)
	if err != nil {
		return 0, err
	}

	cli := c.rpcClient(ch.RPCEndpoints)
	return cli.ChainHead(ctx)
}

// FindGaps identifies and retrieves missing height ranges (gaps) in the indexing progress for a chain.
func (c *Context) FindGaps(ctx context.Context, in types.ChainIdInput) ([]db.Gap, error) {
	return c.IndexerDB.FindGaps(ctx, in.ChainID)
}

// StartIndexWorkflow starts (or resumes) a top-level indexing workflow for a given height.
func (c *Context) StartIndexWorkflow(ctx context.Context, in *types.IndexBlockInput) error {
	logger := activity.GetLogger(ctx)
	// TODO: ensure chain exists before triggering workflows.

	wfID := c.TemporalClient.GetIndexBlockWorkflowId(in.ChainID, in.Height)
	options := client.StartWorkflowOptions{
		ID:        wfID,
		TaskQueue: c.TemporalClient.GetIndexerQueue(in.ChainID),
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 1.2,
			MaximumInterval:    5 * time.Second,
			MaximumAttempts:    0,
		},
	}
	if in.PriorityKey > 0 {
		options.Priority = sdktemporal.Priority{PriorityKey: in.PriorityKey}
	}

	_, err := c.TemporalClient.TClient.ExecuteWorkflow(ctx, options, "IndexBlockWorkflow", in)
	if err != nil {
		var alreadyStarted *serviceerror.WorkflowExecutionAlreadyStarted
		if errors.As(err, &alreadyStarted) {
			return nil
		}
		logger.Error("failed to start workflow", zap.Error(err))
		return err
	}
	return nil
}

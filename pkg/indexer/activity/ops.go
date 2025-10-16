package activity

import (
	"context"
	"errors"
	"fmt"
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
// Returns output containing skip flag and execution duration in milliseconds.
func (c *Context) PrepareIndexBlock(ctx context.Context, in types.IndexBlockInput) (types.PrepareIndexBlockOutput, error) {
	start := time.Now()

	chainDb, err := c.NewChainDb(ctx, in.ChainID)
	if err != nil {
		return types.PrepareIndexBlockOutput{}, sdktemporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", err)
	}

	exists, err := chainDb.HasBlock(ctx, in.Height)
	if err != nil {
		return types.PrepareIndexBlockOutput{}, sdktemporal.NewApplicationErrorWithCause("block_lookup_failed", "chain_db_error", err)
	}

	if !in.Reindex && exists {
		durationMs := float64(time.Since(start).Microseconds()) / 1000.0
		return types.PrepareIndexBlockOutput{Skip: true, DurationMs: durationMs}, nil
	}

	if in.Reindex {
		if err := chainDb.DeleteTransactions(ctx, in.Height); err != nil {
			return types.PrepareIndexBlockOutput{}, sdktemporal.NewApplicationErrorWithCause("delete_transactions_failed", "chain_db_error", err)
		}
		if err := chainDb.DeleteBlock(ctx, in.Height); err != nil {
			return types.PrepareIndexBlockOutput{}, sdktemporal.NewApplicationErrorWithCause("delete_block_failed", "chain_db_error", err)
		}
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.PrepareIndexBlockOutput{Skip: false, DurationMs: durationMs}, nil
}

// GetLastIndexed returns the last indexed block height for the given chain.
func (c *Context) GetLastIndexed(ctx context.Context, in *types.ChainIdInput) (uint64, error) {
	return c.IndexerDB.LastIndexed(ctx, in.ChainID)
}

// GetLatestHead returns the latest block height for the given chain by hitting RPC endpoints.
// It also updates the RPC health status in the database based on the success/failure of the RPC call.
func (c *Context) GetLatestHead(ctx context.Context, in *types.ChainIdInput) (uint64, error) {
	logger := activity.GetLogger(ctx)

	ch, err := c.IndexerDB.GetChain(ctx, in.ChainID)
	if err != nil {
		return 0, err
	}

	cli := c.rpcClient(ch.RPCEndpoints)
	head, err := cli.ChainHead(ctx)

	// Update RPC health status based on the result
	if err != nil {
		// RPC call failed - mark as unreachable
		healthErr := c.IndexerDB.UpdateRPCHealth(
			ctx,
			in.ChainID,
			"unreachable",
			fmt.Sprintf("RPC endpoints failed x: %v", err),
		)
		if healthErr != nil {
			logger.Warn("failed to update RPC health status",
				zap.String("chain_id", in.ChainID),
				zap.Error(healthErr),
			)
		}
		return 0, err
	}

	// RPC call succeeded - mark as healthy
	healthErr := c.IndexerDB.UpdateRPCHealth(
		ctx,
		in.ChainID,
		"healthy",
		fmt.Sprintf("RPC endpoints responding, head at block %d", head),
	)
	if healthErr != nil {
		logger.Warn("failed to update RPC health status",
			zap.String("chain_id", in.ChainID),
			zap.Error(healthErr),
		)
	}

	return head, nil
}

// FindGaps identifies and retrieves missing height ranges (gaps) in the indexing progress for a chain.
func (c *Context) FindGaps(ctx context.Context, in *types.ChainIdInput) ([]db.Gap, error) {
	return c.IndexerDB.FindGaps(ctx, in.ChainID)
}

// StartIndexWorkflow starts (or resumes) a top-level indexing workflow for a given height.
// NOTE: This is a local activity, so it accepts a value type (not pointer) for proper serialization.
func (c *Context) StartIndexWorkflow(ctx context.Context, in types.IndexBlockInput) error {
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

// IsSchedulerWorkflowRunning checks if the SchedulerWorkflow is currently running for a given chain.
// This activity uses Temporal's DescribeWorkflowExecution API to check the workflow status.
// Returns true if the workflow is running, false otherwise.
func (c *Context) IsSchedulerWorkflowRunning(ctx context.Context, in *types.ChainIdInput) (bool, error) {
	logger := activity.GetLogger(ctx)

	wfID := c.TemporalClient.GetSchedulerWorkflowID(in.ChainID)

	// Query the workflow execution status
	desc, err := c.TemporalClient.TClient.DescribeWorkflowExecution(ctx, wfID, "")
	if err != nil {
		// If workflow not found, it's not running
		var notFound *serviceerror.NotFound
		if errors.As(err, &notFound) {
			return false, nil
		}
		logger.Warn("failed to describe scheduler workflow",
			zap.String("chain_id", in.ChainID),
			zap.String("workflow_id", wfID),
			zap.Error(err),
		)
		return false, err
	}

	// Check if workflow is still running
	workflowInfo := desc.WorkflowExecutionInfo
	if workflowInfo == nil {
		return false, nil
	}

	// Workflow is running if it doesn't have a close time
	isRunning := workflowInfo.CloseTime == nil

	logger.Debug("Scheduler workflow status checked",
		"chain_id", in.ChainID,
		"workflow_id", wfID,
		"is_running", isRunning,
	)

	return isRunning, nil
}

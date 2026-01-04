package workflow

import (
	"time"

	"github.com/canopy-network/canopyx/app/admin/types"
	"github.com/canopy-network/canopyx/pkg/temporal"
	sdktemporal "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// DeleteChainWorkflow permanently removes all data for a chain.
// Runs 3 activities in parallel:
// 1. CleanCrossChainData - removes data from global cross-chain tables
// 2. DropChainDatabase - drops the chain_X database
// 3. CleanAdminTables - removes records from admin tables (endpoints, index_progress, etc.)
func (wc *Context) DeleteChainWorkflow(ctx workflow.Context, in types.DeleteChainWorkflowInput) (types.DeleteChainWorkflowOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting delete chain workflow", "chain_id", in.ChainID)

	result := types.DeleteChainWorkflowOutput{ChainID: in.ChainID}

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		HeartbeatTimeout:    1 * time.Minute,
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    1 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    0,
		},
		TaskQueue: temporal.QueueMaintenance,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Run all 3 activities in parallel
	var crossChainFuture, dropDbFuture, adminTablesFuture workflow.Future

	crossChainFuture = workflow.ExecuteActivity(ctx, wc.ActivityContext.CleanCrossChainData,
		types.CleanCrossChainDataInput(in))

	dropDbFuture = workflow.ExecuteActivity(ctx, wc.ActivityContext.DropChainDatabase,
		types.DropChainDatabaseInput(in))

	adminTablesFuture = workflow.ExecuteActivity(ctx, wc.ActivityContext.CleanAdminTables,
		types.CleanAdminTablesInput(in))

	// Wait for all activities to complete
	var crossChainResult types.CleanCrossChainDataOutput
	if err := crossChainFuture.Get(ctx, &crossChainResult); err != nil {
		logger.Error("CleanCrossChainData failed", "error", err.Error())
		result.Errors = append(result.Errors, "cross-chain: "+err.Error())
	} else {
		result.CrossChainCleaned = crossChainResult.Success
	}

	var dropDbResult types.DropChainDatabaseOutput
	if err := dropDbFuture.Get(ctx, &dropDbResult); err != nil {
		logger.Error("DropChainDatabase failed", "error", err.Error())
		result.Errors = append(result.Errors, "drop-db: "+err.Error())
	} else {
		result.DatabaseDropped = dropDbResult.Success
	}

	var adminTablesResult types.CleanAdminTablesOutput
	if err := adminTablesFuture.Get(ctx, &adminTablesResult); err != nil {
		logger.Error("CleanAdminTables failed", "error", err.Error())
		result.Errors = append(result.Errors, "admin-tables: "+err.Error())
	} else {
		result.AdminTablesCleaned = adminTablesResult.Success
	}

	if len(result.Errors) > 0 {
		logger.Warn("Delete chain workflow completed with errors",
			"chain_id", in.ChainID,
			"errors", result.Errors)
	} else {
		logger.Info("Delete chain workflow completed successfully", "chain_id", in.ChainID)
	}

	return result, nil
}

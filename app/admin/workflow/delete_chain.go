package workflow

import (
    "time"

    "github.com/canopy-network/canopyx/app/admin/types"
    sdktemporal "go.temporal.io/sdk/temporal"
    "go.temporal.io/sdk/workflow"
)

// DeleteChainWorkflow permanently removes all data for a chain.
// Runs 3 activities in parallel:
// 1. DeleteChainData - deletes all chain data from the global database
// 2. CleanAdminTables - removes records from admin tables (endpoints, index_progress, etc.)
// 3. WaitForNamespaceCleanup - waits for the Temporal namespace to be fully cleaned up
func (wc *Context) DeleteChainWorkflow(ctx workflow.Context, in types.DeleteChainWorkflowInput) (types.DeleteChainWorkflowOutput, error) {
    logger := workflow.GetLogger(ctx)
    logger.Info("Starting delete chain workflow",
        "chain_id", in.ChainID,
        "renamed_namespace", in.RenamedNamespace)

    result := types.DeleteChainWorkflowOutput{ChainID: in.ChainID}

    // Activity options for DB operations (shorter timeout)
    dbActivityOpts := workflow.LocalActivityOptions{
        StartToCloseTimeout: 10 * time.Minute,
        RetryPolicy: &sdktemporal.RetryPolicy{
            InitialInterval:    1 * time.Second,
            BackoffCoefficient: 2.0,
            MaximumInterval:    30 * time.Second,
            MaximumAttempts:    0,
        },
    }
    dbCtx := workflow.WithLocalActivityOptions(ctx, dbActivityOpts)

    // Activity options for namespace cleanup (longer timeout - can take several minutes)
    namespaceActivityOpts := workflow.LocalActivityOptions{
        StartToCloseTimeout: 10 * time.Minute,
        RetryPolicy: &sdktemporal.RetryPolicy{
            InitialInterval:    5 * time.Second,
            BackoffCoefficient: 1.5,
            MaximumInterval:    30 * time.Second,
            MaximumAttempts:    0,
        },
    }
    nsCtx := workflow.WithLocalActivityOptions(ctx, namespaceActivityOpts)

    // Run all 3 activities in parallel
    deleteDataFuture := workflow.ExecuteLocalActivity(dbCtx, wc.ActivityContext.DeleteChainData,
        types.DeleteChainDataInput{ChainID: in.ChainID})

    adminTablesFuture := workflow.ExecuteLocalActivity(dbCtx, wc.ActivityContext.CleanAdminTables,
        types.CleanAdminTablesInput{ChainID: in.ChainID})

    namespaceCleanupFuture := workflow.ExecuteLocalActivity(nsCtx, wc.ActivityContext.WaitForNamespaceCleanup,
        types.WaitForNamespaceCleanupInput{RenamedNamespace: in.RenamedNamespace})

    // Wait for all activities to complete
    var deleteDataResult types.DeleteChainDataOutput
    if err := deleteDataFuture.Get(ctx, &deleteDataResult); err != nil {
        logger.Error("DeleteChainData failed", "error", err.Error())
        result.Errors = append(result.Errors, "delete-data: "+err.Error())
    } else {
        result.ChainDataDeleted = deleteDataResult.Success
    }

    var adminTablesResult types.CleanAdminTablesOutput
    if err := adminTablesFuture.Get(ctx, &adminTablesResult); err != nil {
        logger.Error("CleanAdminTables failed", "error", err.Error())
        result.Errors = append(result.Errors, "admin-tables: "+err.Error())
    } else {
        result.AdminTablesCleaned = adminTablesResult.Success
    }

    var namespaceCleanupResult types.WaitForNamespaceCleanupOutput
    if err := namespaceCleanupFuture.Get(ctx, &namespaceCleanupResult); err != nil {
        logger.Error("WaitForNamespaceCleanup failed", "error", err.Error())
        result.Errors = append(result.Errors, "namespace-cleanup: "+err.Error())
    } else {
        result.NamespaceCleanupComplete = namespaceCleanupResult.Success
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

package activity

import (
    "context"

    "github.com/canopy-network/canopyx/app/admin/types"
    "go.uber.org/zap"
)

// DeleteChainData deletes all data for a chain from the global database.
func (ac *Context) DeleteChainData(ctx context.Context, in types.DeleteChainDataInput) (types.DeleteChainDataOutput, error) {
    ac.Logger.Info("Deleting chain data", zap.Uint64("chain_id", in.ChainID))

    if err := ac.GlobalDB.DeleteChainData(ctx, in.ChainID); err != nil {
        ac.Logger.Error("Failed to delete chain data", zap.Uint64("chain_id", in.ChainID), zap.Error(err))
        return types.DeleteChainDataOutput{ChainID: in.ChainID, Success: false}, err
    }

    ac.Logger.Info("Chain data deleted", zap.Uint64("chain_id", in.ChainID))
    return types.DeleteChainDataOutput{ChainID: in.ChainID, Success: true}, nil
}

// CleanAdminTables removes all admin table records for a chain.
func (ac *Context) CleanAdminTables(ctx context.Context, in types.CleanAdminTablesInput) (types.CleanAdminTablesOutput, error) {
    ac.Logger.Info("Cleaning admin tables", zap.Uint64("chain_id", in.ChainID))

    // Delete endpoints
    if err := ac.AdminDB.DeleteEndpointsForChain(ctx, in.ChainID); err != nil {
        ac.Logger.Warn("Failed to delete endpoints", zap.Uint64("chain_id", in.ChainID), zap.Error(err))
    }

    // Delete index progress
    if err := ac.AdminDB.DeleteIndexProgressForChain(ctx, in.ChainID); err != nil {
        ac.Logger.Warn("Failed to delete index progress", zap.Uint64("chain_id", in.ChainID), zap.Error(err))
    }

    // Delete reindex requests
    if err := ac.AdminDB.DeleteReindexRequestsForChain(ctx, in.ChainID); err != nil {
        ac.Logger.Warn("Failed to delete reindex requests", zap.Uint64("chain_id", in.ChainID), zap.Error(err))
    }

    // Delete chain record (must be last)
    if err := ac.AdminDB.HardDeleteChain(ctx, in.ChainID); err != nil {
        ac.Logger.Error("Failed to delete chain record", zap.Uint64("chain_id", in.ChainID), zap.Error(err))
        return types.CleanAdminTablesOutput{ChainID: in.ChainID, Success: false, Error: err.Error()}, err
    }

    ac.Logger.Info("Admin tables cleaned", zap.Uint64("chain_id", in.ChainID))
    return types.CleanAdminTablesOutput{ChainID: in.ChainID, Success: true}, nil
}

// WaitForNamespaceCleanup waits for a renamed Temporal namespace to be fully deleted.
// This ensures that when the chain is re-added, there's no stale task queue data
// that could cause workflow tasks to be undeliverable.
func (ac *Context) WaitForNamespaceCleanup(ctx context.Context, in types.WaitForNamespaceCleanupInput) (types.WaitForNamespaceCleanupOutput, error) {
    if in.RenamedNamespace == "" {
        ac.Logger.Debug("No renamed namespace to wait for")
        return types.WaitForNamespaceCleanupOutput{Success: true}, nil
    }

    ac.Logger.Info("Waiting for namespace cleanup", zap.String("renamed_namespace", in.RenamedNamespace))

    if err := ac.TemporalManager.WaitForNamespaceCleanup(ctx, in.RenamedNamespace); err != nil {
        ac.Logger.Error("Namespace cleanup failed or timed out",
            zap.String("renamed_namespace", in.RenamedNamespace),
            zap.Error(err))
        return types.WaitForNamespaceCleanupOutput{Success: false, Error: err.Error()}, err
    }

    ac.Logger.Info("Namespace cleanup completed", zap.String("renamed_namespace", in.RenamedNamespace))
    return types.WaitForNamespaceCleanupOutput{Success: true}, nil
}

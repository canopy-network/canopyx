package activity

import (
	"context"

	"github.com/canopy-network/canopyx/app/admin/types"
	"go.uber.org/zap"
)

// CleanCrossChainData removes all cross-chain data for a chain from global tables.
func (c *Context) CleanCrossChainData(ctx context.Context, in types.CleanCrossChainDataInput) (types.CleanCrossChainDataOutput, error) {
	c.Logger.Info("Cleaning cross-chain data", zap.Uint64("chain_id", in.ChainID))

	if c.CrossChainDB == nil {
		c.Logger.Warn("CrossChainDB not configured, skipping cross-chain cleanup", zap.Uint64("chain_id", in.ChainID))
		return types.CleanCrossChainDataOutput{ChainID: in.ChainID, Success: true}, nil
	}

	if err := c.CrossChainDB.RemoveChainSync(ctx, in.ChainID); err != nil {
		c.Logger.Error("Failed to clean cross-chain data", zap.Uint64("chain_id", in.ChainID), zap.Error(err))
		return types.CleanCrossChainDataOutput{ChainID: in.ChainID, Success: false, Error: err.Error()}, err
	}

	c.Logger.Info("Cross-chain data cleaned", zap.Uint64("chain_id", in.ChainID))
	return types.CleanCrossChainDataOutput{ChainID: in.ChainID, Success: true}, nil
}

// DropChainDatabase drops the chain-specific database.
func (c *Context) DropChainDatabase(ctx context.Context, in types.DropChainDatabaseInput) (types.DropChainDatabaseOutput, error) {
	c.Logger.Info("Dropping chain database", zap.Uint64("chain_id", in.ChainID))

	if err := c.AdminDB.DropChainDatabase(ctx, in.ChainID); err != nil {
		c.Logger.Error("Failed to drop chain database", zap.Uint64("chain_id", in.ChainID), zap.Error(err))
		return types.DropChainDatabaseOutput{ChainID: in.ChainID, Success: false, Error: err.Error()}, err
	}

	c.Logger.Info("Chain database dropped", zap.Uint64("chain_id", in.ChainID))
	return types.DropChainDatabaseOutput{ChainID: in.ChainID, Success: true}, nil
}

// CleanAdminTables removes all admin table records for a chain.
func (c *Context) CleanAdminTables(ctx context.Context, in types.CleanAdminTablesInput) (types.CleanAdminTablesOutput, error) {
	c.Logger.Info("Cleaning admin tables", zap.Uint64("chain_id", in.ChainID))

	// Delete endpoints
	if err := c.AdminDB.DeleteEndpointsForChain(ctx, in.ChainID); err != nil {
		c.Logger.Warn("Failed to delete endpoints", zap.Uint64("chain_id", in.ChainID), zap.Error(err))
	}

	// Delete index progress
	if err := c.AdminDB.DeleteIndexProgressForChain(ctx, in.ChainID); err != nil {
		c.Logger.Warn("Failed to delete index progress", zap.Uint64("chain_id", in.ChainID), zap.Error(err))
	}

	// Delete reindex requests
	if err := c.AdminDB.DeleteReindexRequestsForChain(ctx, in.ChainID); err != nil {
		c.Logger.Warn("Failed to delete reindex requests", zap.Uint64("chain_id", in.ChainID), zap.Error(err))
	}

	// Delete chain record (must be last)
	if err := c.AdminDB.HardDeleteChain(ctx, in.ChainID); err != nil {
		c.Logger.Error("Failed to delete chain record", zap.Uint64("chain_id", in.ChainID), zap.Error(err))
		return types.CleanAdminTablesOutput{ChainID: in.ChainID, Success: false, Error: err.Error()}, err
	}

	c.Logger.Info("Admin tables cleaned", zap.Uint64("chain_id", in.ChainID))
	return types.CleanAdminTablesOutput{ChainID: in.ChainID, Success: true}, nil
}

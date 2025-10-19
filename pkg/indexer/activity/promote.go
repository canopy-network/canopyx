package activity

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/canopy-network/canopyx/pkg/db/entities"
	"github.com/canopy-network/canopyx/pkg/indexer/types"
)

// PromoteData promotes single entity data from staging to production table.
// This is a generic activity that works for ALL entities defined in the entities package.
// The operation is idempotent and safe to retry on failure.
func (c *Context) PromoteData(ctx context.Context, in types.PromoteDataInput) (types.PromoteDataOutput, error) {
	start := time.Now()

	// Validate and convert entity name to type-safe Entity
	entity, err := entities.FromString(in.Entity)
	if err != nil {
		return types.PromoteDataOutput{}, fmt.Errorf("invalid entity: %w", err)
	}

	// Get the chain database connection
	chainDb, err := c.NewChainDb(ctx, in.ChainID)
	if err != nil {
		return types.PromoteDataOutput{}, fmt.Errorf("get chain db: %w", err)
	}

	c.Logger.Info("Promoting entity data from staging to production",
		zap.String("chainId", in.ChainID),
		zap.String("entity", entity.String()),
		zap.Uint64("height", in.Height))

	// Perform the promotion using the type-safe entity
	if err := chainDb.PromoteEntity(ctx, entity, in.Height); err != nil {
		return types.PromoteDataOutput{}, fmt.Errorf("promote %s at height %d: %w", entity, in.Height, err)
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	c.Logger.Debug("Successfully promoted entity data",
		zap.String("chainId", in.ChainID),
		zap.String("entity", entity.String()),
		zap.Uint64("height", in.Height),
		zap.Float64("durationMs", durationMs))

	return types.PromoteDataOutput{
		Entity:     entity.String(),
		Height:     in.Height,
		DurationMs: durationMs,
	}, nil
}

// CleanPromotedData removes promoted data from staging table after successful promotion.
// This is a generic activity that works for ALL entities defined in the entities package.
// This operation is optional and non-critical - failures are logged but don't fail the workflow.
// The operation is idempotent and safe to retry on failure.
func (c *Context) CleanPromotedData(ctx context.Context, in types.CleanPromotedDataInput) (types.CleanPromotedDataOutput, error) {
	start := time.Now()

	// Validate and convert entity name to type-safe Entity
	entity, err := entities.FromString(in.Entity)
	if err != nil {
		return types.CleanPromotedDataOutput{}, fmt.Errorf("invalid entity: %w", err)
	}

	// Get the chain database connection
	chainDb, err := c.NewChainDb(ctx, in.ChainID)
	if err != nil {
		return types.CleanPromotedDataOutput{}, fmt.Errorf("get chain db: %w", err)
	}

	c.Logger.Debug("Cleaning promoted data from staging table",
		zap.String("chainId", in.ChainID),
		zap.String("entity", entity.String()),
		zap.Uint64("height", in.Height))

	// Attempt to clean the staging table
	// Note: CleanEntityStaging logs warnings internally for failures
	if err := chainDb.CleanEntityStaging(ctx, entity, in.Height); err != nil {
		// Log the error but don't fail the activity - cleanup is non-critical
		c.Logger.Warn("Failed to clean staging data (non-critical)",
			zap.String("chainId", in.ChainID),
			zap.String("entity", entity.String()),
			zap.Uint64("height", in.Height),
			zap.Error(err))
		// Continue and return success with the timing data
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	return types.CleanPromotedDataOutput{
		Entity:     entity.String(),
		Height:     in.Height,
		DurationMs: durationMs,
	}, nil
}
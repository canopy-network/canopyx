package activity

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"github.com/canopy-network/canopyx/pkg/db/entities"
)

// PromoteData promotes single entity data from staging to production table.
// This is a generic activity that works for ALL entities defined in the entities package.
// The operation is idempotent and safe to retry on failure.
func (ac *Context) PromoteData(ctx context.Context, in types.ActivityPromoteDataInput) (types.ActivityPromoteDataOutput, error) {
	start := time.Now()

	// Validate and convert entity name to type-safe Entity
	entity, err := entities.FromString(in.Entity)
	if err != nil {
		return types.ActivityPromoteDataOutput{}, fmt.Errorf("invalid entity: %w", err)
	}

	// Get the chain database connection
	chainDb, err := ac.GetChainDb(ctx, ac.ChainID)
	if err != nil {
		return types.ActivityPromoteDataOutput{}, fmt.Errorf("get chain db: %w", err)
	}

	ac.Logger.Info("Promoting entity data from staging to production",
		zap.Uint64("chainId", ac.ChainID),
		zap.String("entity", entity.String()),
		zap.Uint64("height", in.Height))

	// Perform the promotion using the type-safe entity
	if err := chainDb.PromoteEntity(ctx, entity, in.Height); err != nil {
		return types.ActivityPromoteDataOutput{}, fmt.Errorf("promote %s at height %d: %w", entity, in.Height, err)
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	ac.Logger.Debug("Successfully promoted entity data",
		zap.Uint64("chainId", ac.ChainID),
		zap.String("entity", entity.String()),
		zap.Uint64("height", in.Height),
		zap.Float64("durationMs", durationMs))

	return types.ActivityPromoteDataOutput{
		Entity:     entity.String(),
		Height:     in.Height,
		DurationMs: durationMs,
	}, nil
}

// CleanPromotedData removes promoted data from staging table after successful promotion.
// This is a generic activity that works for ALL entities defined in the entities package.
// This operation is optional and non-critical - failures are logged but don't fail the workflow.
// The operation is idempotent and safe to retry on failure.
func (ac *Context) CleanPromotedData(ctx context.Context, in types.ActivityCleanPromotedDataInput) (types.ActivityCleanPromotedDataOutput, error) {
	start := time.Now()

	// Validate and convert entity name to type-safe Entity
	entity, err := entities.FromString(in.Entity)
	if err != nil {
		return types.ActivityCleanPromotedDataOutput{}, fmt.Errorf("invalid entity: %w", err)
	}

	// Get the chain database connection
	chainDb, err := ac.GetChainDb(ctx, ac.ChainID)
	if err != nil {
		return types.ActivityCleanPromotedDataOutput{}, fmt.Errorf("get chain db: %w", err)
	}

	ac.Logger.Debug("Cleaning promoted data from staging table",
		zap.Uint64("chainId", ac.ChainID),
		zap.String("entity", entity.String()),
		zap.Uint64("height", in.Height))

	// Attempt to clean the staging table
	// Note: CleanEntityStaging logs warnings internally for failures
	if err := chainDb.CleanEntityStaging(ctx, entity, in.Height); err != nil {
		// Log the error but don't fail the activity - cleanup is non-critical
		ac.Logger.Warn("Failed to clean staging data (non-critical)",
			zap.Uint64("chainId", ac.ChainID),
			zap.String("entity", entity.String()),
			zap.Uint64("height", in.Height),
			zap.Error(err))
		// Continue and return success with the timing data
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	return types.ActivityCleanPromotedDataOutput{
		Entity:     entity.String(),
		Height:     in.Height,
		DurationMs: durationMs,
	}, nil
}

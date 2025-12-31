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
// Failures will be retried by Temporal according to the configured retry policy.
// The operation is idempotent and safe to retry on failure.

// CleanAllPromotedData cleans all staging tables using PARALLEL WITH for maximum performance.
// This is significantly faster than cleaning entities one by one.
func (ac *Context) CleanAllPromotedData(ctx context.Context, in types.ActivityCleanAllPromotedDataInput) (types.ActivityCleanAllPromotedDataOutput, error) {
	start := time.Now()

	// Convert entity strings to type-safe Entities
	entitiesToClean := make([]entities.Entity, 0, len(in.Entities))
	for _, entityStr := range in.Entities {
		entity, err := entities.FromString(entityStr)
		if err != nil {
			return types.ActivityCleanAllPromotedDataOutput{}, fmt.Errorf("invalid entity %q: %w", entityStr, err)
		}
		entitiesToClean = append(entitiesToClean, entity)
	}

	// Get the chain database connection
	chainDb, err := ac.GetChainDb(ctx, ac.ChainID)
	if err != nil {
		return types.ActivityCleanAllPromotedDataOutput{}, fmt.Errorf("get chain db: %w", err)
	}

	ac.Logger.Debug("Cleaning all promoted data with PARALLEL WITH",
		zap.Uint64("chainId", ac.ChainID),
		zap.Int("entity_count", len(entitiesToClean)),
		zap.Uint64("height", in.Height))

	// Clean all staging tables in a single PARALLEL WITH batch
	if err := chainDb.CleanAllEntitiesStaging(ctx, entitiesToClean, in.Height); err != nil {
		return types.ActivityCleanAllPromotedDataOutput{}, fmt.Errorf("clean staging batch at height %d: %w", in.Height, err)
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	return types.ActivityCleanAllPromotedDataOutput{
		EntityCount: len(entitiesToClean),
		DurationMs:  durationMs,
	}, nil
}

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
	// Return error on failure so Temporal retries the activity
	if err := chainDb.CleanEntityStaging(ctx, entity, in.Height); err != nil {
		return types.ActivityCleanPromotedDataOutput{}, fmt.Errorf("clean staging for %s at height %d: %w", entity, in.Height, err)
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	return types.ActivityCleanPromotedDataOutput{
		Entity:     entity.String(),
		Height:     in.Height,
		DurationMs: durationMs,
	}, nil
}

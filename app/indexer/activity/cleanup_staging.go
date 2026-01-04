package activity

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"go.uber.org/zap"
)

// GetCleanableHeights queries index_progress for heights that have been indexed
// and are safe to clean from staging tables.
//
// Returns heights indexed within the last lookbackHours.
// Heights in index_progress = successfully promoted = staging can be cleaned.
//
// Example: With lookbackHours=2, at 4pm returns heights indexed between 2pm and 4pm.
func (ac *Context) GetCleanableHeights(ctx context.Context, in types.ActivityGetCleanableHeightsInput) (types.ActivityGetCleanableHeightsOutput, error) {
	start := time.Now()

	// Apply defaults
	lookbackHours := in.LookbackHours
	if lookbackHours == 0 {
		lookbackHours = 2
	}

	// Query admin DB for heights indexed within the last lookbackHours
	heights, err := ac.AdminDB.GetCleanableHeights(ctx, ac.ChainID, lookbackHours)
	if err != nil {
		return types.ActivityGetCleanableHeightsOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	ac.Logger.Info("Found cleanable heights",
		zap.Uint64("chainId", ac.ChainID),
		zap.Int("heights", len(heights)),
		zap.Int("lookbackHours", lookbackHours),
		zap.Float64("durationMs", durationMs))

	return types.ActivityGetCleanableHeightsOutput{
		Heights:    heights,
		DurationMs: durationMs,
	}, nil
}

// CleanStagingBatch removes data for the specified heights from all staging tables.
// Uses a single DELETE ... WHERE height IN (...) per table, creating only ONE mutation
// per table instead of one per height. This is much more efficient than per-height cleanup.
//
// This activity is designed to be called by the hourly cleanup workflow after
// GetCleanableHeights has determined which heights are safe to clean.
func (ac *Context) CleanStagingBatch(ctx context.Context, in types.ActivityCleanStagingBatchInput) (types.ActivityCleanStagingBatchOutput, error) {
	start := time.Now()

	if len(in.Heights) == 0 {
		return types.ActivityCleanStagingBatchOutput{
			HeightsCleaned:  0,
			EntitiesCleaned: 0,
			DurationMs:      0,
		}, nil
	}

	// Get chain DB
	chainDb, err := ac.GetChainDb(ctx, ac.ChainID)
	if err != nil {
		return types.ActivityCleanStagingBatchOutput{}, err
	}

	// Batch clean all staging tables
	entitiesCleaned, err := chainDb.CleanStagingBatch(ctx, in.Heights)
	if err != nil {
		return types.ActivityCleanStagingBatchOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	ac.Logger.Info("Batch cleaned staging tables",
		zap.Uint64("chainId", ac.ChainID),
		zap.Int("heights", len(in.Heights)),
		zap.Int("entitiesCleaned", entitiesCleaned),
		zap.Float64("durationMs", durationMs))

	return types.ActivityCleanStagingBatchOutput{
		HeightsCleaned:  len(in.Heights),
		EntitiesCleaned: entitiesCleaned,
		DurationMs:      durationMs,
	}, nil
}

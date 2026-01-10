package activity

import (
    "context"
    "fmt"
    "time"

    "github.com/canopy-network/canopyx/app/indexer/types"
    "github.com/canopy-network/canopyx/pkg/db/models/indexer"
    "go.temporal.io/sdk/activity"
    "go.temporal.io/sdk/temporal"
    "go.uber.org/zap"
)

// ComputeLPSnapshots computes LP position snapshots for a specific target date (calendar day UTC).
//
// ARCHITECTURAL NOTE: This activity is executed via a scheduled workflow (hourly per chain).
// It queries per-chain pool_points_by_holder tables directly (NOT cross-chain materialized views)
// to avoid race conditions when indexing new chains.
//
// The activity:
// 1. Finds the snapshot height (highest block with time <= 23:59:59 UTC on target date)
// 2. Queries pool_points_by_holder at that height
// 3. Queries pools table for total_points to compute percentages
// 4. Tracks position lifecycle (created_date, closed_date, active status)
// 5. Writes snapshots directly to cross-chain lp_position_snapshots_global table
//
// Returns output containing the number of snapshots computed and execution duration.
func (ac *Context) ComputeLPSnapshots(ctx context.Context, input types.ActivityComputeLPSnapshotsInput) (types.ActivityComputeLPSnapshotsOutput, error) {
    start := time.Now()
    logger := ac.Logger.With(
        zap.Time("target_date", input.TargetDate),
        zap.Uint64("chain_id", ac.ChainID))

    logger.Info("Starting LP snapshot computation")

    // Get chain database connection
    chainDb, err := ac.GetGlobalDb(ctx)
    if err != nil {
        return types.ActivityComputeLPSnapshotsOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", err)
    }

    // Find snapshot height: highest block with time <= 23:59:59.999999999 UTC on target date
    endOfDay := time.Date(input.TargetDate.Year(), input.TargetDate.Month(), input.TargetDate.Day(), 23, 59, 59, 999999999, time.UTC)
    snapshotBlock, err := chainDb.GetHighestBlockBeforeTime(ctx, endOfDay)
    if err != nil {
        return types.ActivityComputeLPSnapshotsOutput{}, temporal.NewApplicationErrorWithCause("failed to find snapshot height", "snapshot_height_error", err)
    }
    activity.RecordHeartbeat(ctx, "found_snapshot_height")

    if snapshotBlock == nil {
        // No blocks exist before end of target date - chain hasn't reached this date yet
        logger.Info("No blocks exist before end of target date, skipping snapshot",
            zap.Time("end_of_day", endOfDay))
        return types.ActivityComputeLPSnapshotsOutput{
            TargetDate:     input.TargetDate,
            TotalSnapshots: 0,
            DurationMs:     float64(time.Since(start).Microseconds()) / 1000.0,
        }, nil
    }

    snapshotHeight := snapshotBlock.Height
    logger.Info("Found snapshot height",
        zap.Uint64("height", snapshotHeight),
        zap.Time("block_time", snapshotBlock.Time))

    // Query pool points at snapshot height
    poolPoints, err := ac.queryPoolPointsAtHeight(ctx, ac.ChainID, snapshotHeight)
    if err != nil {
        return types.ActivityComputeLPSnapshotsOutput{}, temporal.NewApplicationErrorWithCause("failed to query pool points", "pool_points_error", err)
    }
    activity.RecordHeartbeat(ctx, "queried_pool_points")

    if len(poolPoints) == 0 {
        // No LP positions exist at this height
        logger.Info("No LP positions found at snapshot height, skipping")
        return types.ActivityComputeLPSnapshotsOutput{
            TargetDate:     input.TargetDate,
            TotalSnapshots: 0,
            DurationMs:     float64(time.Since(start).Microseconds()) / 1000.0,
        }, nil
    }

    logger.Info("Queried pool points",
        zap.Int("count", len(poolPoints)))

    // Get pool total points for percentage calculation
    poolTotalPoints, err := ac.queryPoolTotalPoints(ctx, ac.ChainID, snapshotHeight, poolPoints)
    if err != nil {
        return types.ActivityComputeLPSnapshotsOutput{}, temporal.NewApplicationErrorWithCause("failed to query pool total points", "pool_total_points_error", err)
    }
    activity.RecordHeartbeat(ctx, "queried_pool_total_points")

    // Get position lifecycle data (created dates)
    positionCreatedDates, err := ac.queryPositionCreatedDates(ctx, ac.ChainID)
    if err != nil {
        logger.Warn("Failed to query position created dates, using defaults", zap.Error(err))
        // Continue with empty map - will use safe defaults
        positionCreatedDates = make(map[string]time.Time)
    }
    activity.RecordHeartbeat(ctx, "queried_position_lifecycle")

    // Build snapshots
    snapshots := make([]*indexer.LPPositionSnapshot, 0, len(poolPoints))
    computedAt := time.Now()

    for _, pp := range poolPoints {
        // Compute pool share percentage with 6 decimal precision
        // Formula: (points / total_points) * 100 * 1000000
        var poolSharePercentage uint64
        if totalPoints, exists := poolTotalPoints[pp.PoolID]; exists && totalPoints > 0 {
            // Use 128-bit arithmetic to avoid overflow
            // percentage = (points * 100 * 1000000) / total_points
            percentage := (uint64(pp.Points) * 100 * 1000000) / totalPoints
            poolSharePercentage = percentage
        } else {
            // Pool doesn't exist or has zero total points
            poolSharePercentage = 0
        }

        // Determine position lifecycle
        positionKey := fmt.Sprintf("%s:%d", pp.Address, pp.PoolID)
        createdDate, hasCreatedDate := positionCreatedDates[positionKey]
        if !hasCreatedDate {
            // Default to snapshot date if we don't have creation data
            createdDate = input.TargetDate
        }

        // Position is active if points > 0
        isActive := pp.Points > 0
        // Default closed date to epoch (1970-01-01) for active positions
        // Set to snapshot date when the position is closed (points = 0)
        closedDate := time.Time{} // Zero time is an epoch in ClickHouse
        if !isActive {
            closedDate = input.TargetDate
        }

        snapshot := &indexer.LPPositionSnapshot{
            SourceChainID:       uint16(ac.ChainID),
            Address:             pp.Address,
            PoolID:              pp.PoolID,
            SnapshotDate:        input.TargetDate,
            SnapshotHeight:      snapshotHeight,
            SnapshotBalance:     pp.LiquidityPoolPoints, // Pre-calculated balance
            PoolSharePercentage: poolSharePercentage,
            PositionCreatedDate: createdDate,
            PositionClosedDate:  closedDate,
            IsPositionActive:    boolToUint8(isActive),
            ComputedAt:          computedAt,
            UpdatedAt:           computedAt,
        }

        snapshots = append(snapshots, snapshot)
    }

    logger.Info("Built snapshots",
        zap.Int("count", len(snapshots)))

    if err := ac.GlobalDB.InsertLPPositionSnapshots(ctx, snapshots); err != nil {
        return types.ActivityComputeLPSnapshotsOutput{}, temporal.NewApplicationErrorWithCause("failed to insert snapshots", "insert_error", err)
    }
    activity.RecordHeartbeat(ctx, "inserted_snapshots")

    durationMs := float64(time.Since(start).Microseconds()) / 1000.0
    logger.Info("LP snapshot computation complete",
        zap.Int("snapshots", len(snapshots)),
        zap.Float64("duration_ms", durationMs))

    return types.ActivityComputeLPSnapshotsOutput{
        TargetDate:     input.TargetDate,
        TotalSnapshots: len(snapshots),
        DurationMs:     durationMs,
    }, nil
}

// queryPoolPointsAtHeight queries pool_points_by_holder table for all positions at the given height.
// Returns deduplicated results (latest record per address+pool_id).
func (ac *Context) queryPoolPointsAtHeight(ctx context.Context, chainId uint64, height uint64) ([]poolPointsRow, error) {
    query := fmt.Sprintf(`
		SELECT address, pool_id, points, liquidity_pool_points
		FROM "%s"."pool_points_by_holder" FINAL
        PREWHERE chain_id = ?
		WHERE height <= ?
		ORDER BY address, pool_id, height DESC
	`, ac.GlobalDB.DatabaseName())

    var allRows []poolPointsRow
    err := ac.GlobalDB.Select(ctx, &allRows, query, chainId, height)
    if err != nil {
        return nil, fmt.Errorf("query pool points: %w", err)
    }

    // Deduplicate: keep only the latest row per (address, pool_id)
    seen := make(map[string]bool)
    dedupedRows := make([]poolPointsRow, 0, len(allRows))

    for _, row := range allRows {
        key := fmt.Sprintf("%s:%d", row.Address, row.PoolID)
        if !seen[key] {
            dedupedRows = append(dedupedRows, row)
            seen[key] = true
        }
    }

    return dedupedRows, nil
}

// queryPoolTotalPoints queries the pools table for total_points at the given height.
// Returns a map of pool_id -> total_points for the specified pool IDs.
func (ac *Context) queryPoolTotalPoints(ctx context.Context, chainId uint64, height uint64, poolPoints []poolPointsRow) (map[uint32]uint64, error) {
    // Extract unique pool IDs
    poolIDsMap := make(map[uint32]bool)
    for _, pp := range poolPoints {
        poolIDsMap[pp.PoolID] = true
    }

    poolIDs := make([]uint32, 0, len(poolIDsMap))
    for poolID := range poolIDsMap {
        poolIDs = append(poolIDs, poolID)
    }

    if len(poolIDs) == 0 {
        return make(map[uint32]uint64), nil
    }

    // Query pools table for total_points
    // ClickHouse requires IN clause with explicit values
    query := fmt.Sprintf(`
		SELECT pool_id, total_points
		FROM "%s"."pools" FINAL
        PREWHERE chain_id = ?
		WHERE height <= ? AND pool_id IN ?
	`, ac.GlobalDB.DatabaseName())

    var rows []poolTotalPointsRow
    err := ac.GlobalDB.Select(ctx, &rows, query, chainId, height, poolIDs)
    if err != nil {
        return nil, fmt.Errorf("query pool total points: %w", err)
    }

    // Deduplicate: keep latest total_points per pool_id
    result := make(map[uint32]uint64)
    for _, row := range rows {
        result[row.PoolID] = row.TotalPoints
    }

    return result, nil
}

// queryPositionCreatedDates queries the pool_points_created_height materialized view
// to get the date when each position was first created.
func (ac *Context) queryPositionCreatedDates(ctx context.Context, chainId uint64) (map[string]time.Time, error) {
    // Query the materialized view for first seen heights
    query := fmt.Sprintf(`
		SELECT address, pool_id, created_height
		FROM "%s"."pool_points_created_height" FINAL
        PREWHERE chain_id = ?
	`, ac.GlobalDB.DatabaseName())

    var rows []positionCreatedRow
    err := ac.GlobalDB.Select(ctx, &rows, query, chainId)
    if err != nil {
        return nil, fmt.Errorf("query position created heights: %w", err)
    }

    // Get block times for created heights
    result := make(map[string]time.Time)
    for _, row := range rows {
        block, err := ac.GlobalDB.GetBlock(ctx, row.CreatedHeight)
        if err != nil {
            // Log warning but continue - we'll use default date for this position
            ac.Logger.Warn("Failed to get block for created height",
                zap.String("address", row.Address),
                zap.Uint32("pool_id", row.PoolID),
                zap.Uint64("created_height", row.CreatedHeight),
                zap.Error(err))
            continue
        }

        positionKey := fmt.Sprintf("%s:%d", row.Address, row.PoolID)
        // Convert to calendar date (UTC)
        result[positionKey] = time.Date(block.Time.Year(), block.Time.Month(), block.Time.Day(), 0, 0, 0, 0, time.UTC)
    }

    return result, nil
}

// poolPointsRow represents a row from the pool_points_by_holder table.
type poolPointsRow struct {
    Address             string `ch:"address"`
    PoolID              uint32 `ch:"pool_id"`
    Points              uint64 `ch:"points"`
    LiquidityPoolPoints uint64 `ch:"liquidity_pool_points"` // Pre-calculated balance
}

// poolTotalPointsRow represents a row from the pools table for total_points lookup.
type poolTotalPointsRow struct {
    PoolID      uint32 `ch:"pool_id"`
    TotalPoints uint64 `ch:"total_points"`
}

// positionCreatedRow represents a row from the pool_points_created_height materialized view.
type positionCreatedRow struct {
    Address       string `ch:"address"`
    PoolID        uint32 `ch:"pool_id"`
    CreatedHeight uint64 `ch:"created_height"`
}

// boolToUint8 converts a boolean to uint8 (1 for true, 0 for false).
func boolToUint8(b bool) uint8 {
    if b {
        return 1
    }
    return 0
}

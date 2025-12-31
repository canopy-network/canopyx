package chain

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initPools creates the pools table and its staging table with ReplacingMergeTree engine.
// Uses height as the deduplication version key.
// The table stores pool state snapshots that change at each height.
// Includes calculated pool ID fields for different pool types (liquidity, holding, escrow, reward).
func (db *DB) initPools(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.PoolColumns)

	// Production table: ORDER BY (pool_id, height) for efficient pool_id lookups
	productionQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" %s (
			%s
		) ENGINE = %s
		ORDER BY (pool_id, height)
	`, db.Name, indexermodels.PoolsProductionTableName, db.OnCluster(), schemaSQL, db.Engine(indexermodels.PoolsProductionTableName, "ReplacingMergeTree", "height"))
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.PoolsProductionTableName, err)
	}

	// Staging table: ORDER BY (height, pool_id) for efficient cleanup/promotion WHERE height = ?
	stagingQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" %s (
			%s
		) ENGINE = %s
		ORDER BY (height, pool_id)
	`, db.Name, indexermodels.PoolsStagingTableName, db.OnCluster(), schemaSQL, db.Engine(indexermodels.PoolsStagingTableName, "ReplacingMergeTree", "height"))
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.PoolsStagingTableName, err)
	}

	return nil
}

// InsertPoolsStaging inserts pools into the pools_staging table.
// This follows the two-phase commit pattern for data consistency.
// Note: Calculated pool IDs should be set via pool.CalculatePoolIDs() before calling this method.
func (db *DB) InsertPoolsStaging(ctx context.Context, pools []*indexermodels.Pool) error {
	if len(pools) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."pools_staging" (pool_id, height, chain_id, amount, total_points, lp_count, height_time, liquidity_pool_id, holding_pool_id, escrow_pool_id, reward_pool_id, amount_delta, total_points_delta, lp_count_delta) VALUES`, db.Name)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, pool := range pools {
		err = batch.Append(
			pool.PoolID,
			pool.Height,
			pool.ChainID,
			pool.Amount,
			pool.TotalPoints,
			pool.LPCount,
			pool.HeightTime,
			pool.LiquidityPoolID,
			pool.HoldingPoolID,
			pool.EscrowPoolID,
			pool.RewardPoolID,
			pool.AmountDelta,
			pool.TotalPointsDelta,
			pool.LPCountDelta,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

// QueryPools retrieves a paginated list of pools ordered by height.
// If sortDesc is true, orders by height DESC (newest first), otherwise ASC (oldest first).
// If cursor > 0 and sortDesc is true, only pools with height < cursor are returned.
// If cursor > 0 and sortDesc is false, only pools with height > cursor are returned.
// The limit parameter controls the maximum number of rows returned (+1 for pagination detection).

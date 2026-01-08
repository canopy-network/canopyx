package chain

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"

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
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (pool_id, height)
	`, db.Name, indexermodels.PoolsProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.PoolsProductionTableName, err)
	}

	// Staging table: ORDER BY (height, pool_id) for efficient cleanup/promotion WHERE height = ?
	stagingQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (height, pool_id)
	`, db.Name, indexermodels.PoolsStagingTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.PoolsStagingTableName, err)
	}

	return nil
}

// InsertPoolsStaging inserts pools into the pools_staging table.
// This follows the two-phase commit pattern for data consistency.
// Note: Calculated pool IDs should be set via pool.CalculatePoolIDs() before calling this method.
func (db *DB) InsertPoolsStaging(ctx context.Context, pools []*indexermodels.Pool) error {
	return db.insertPools(ctx, indexermodels.PoolsStagingTableName, pools)
}

// InsertPoolsProduction inserts pools into the pools production table.
func (db *DB) InsertPoolsProduction(ctx context.Context, pools []*indexermodels.Pool) error {
	return db.insertPools(ctx, indexermodels.PoolsProductionTableName, pools)
}

func (db *DB) insertPools(ctx context.Context, tableName string, pools []*indexermodels.Pool) error {
	if len(pools) == 0 {
		return nil
	}

	query := fmt.Sprintf(
		`INSERT INTO "%s"."%s" (pool_id, height, chain_id, amount, total_points, lp_count, height_time, liquidity_pool_id, holding_pool_id, escrow_pool_id, reward_pool_id, amount_delta, total_points_delta, lp_count_delta) VALUES`,
		db.Name,
		tableName,
	)
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

// initPoolCreatedHeightView creates a materialized view to calculate the minimum height
// at which each pool was created. This provides an efficient way to query pool creation
// heights without storing the value in every pool snapshot row.
//
// The materialized view automatically updates as new data is inserted into the pools table.
//
// Query usage: SELECT pool_id, created_height FROM pool_created_height WHERE pool_id = ?
func (db *DB) initPoolCreatedHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."pool_created_height"
		ENGINE = %s
		ORDER BY pool_id
		AS SELECT
			pool_id,
			minSimpleState(height) as created_height
		FROM "%s"."pools"
		GROUP BY pool_id
	`, db.Name, clickhouse.ReplicatedEngine(clickhouse.AggregatingMergeTree, ""), db.Name)

	return db.Exec(ctx, query)
}

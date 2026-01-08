package chain

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initPoolPointsByHolder initializes the pool_points_by_holder table and its staging table.
// This table stores versioned snapshots of liquidity provider pool points using the snapshot-on-change pattern.
// Uses ReplacingMergeTree(height) to deduplicate and keep the latest state at each height.
//
// ORDER BY Optimization:
// pool_id comes before address following ClickHouse best practice of "lower cardinality first".
// - pool_id: lower cardinality (~10-100 pools) - binary search O(log₂ n) is very efficient
// - address: higher cardinality (thousands+ addresses) - secondary column benefits from pool_id narrowing
// See: https://clickhouse.com/resources/engineering/clickhouse-query-optimisation-definitive-guide
func (db *DB) initPoolPointsByHolder(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.PoolPointsByHolderColumns)

	// Production table: ORDER BY (pool_id, address, height)
	// pool_id first (lower cardinality) for optimal index efficiency
	productionQuery := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (pool_id, address, height)
	`, db.Name, indexermodels.PoolPointsByHolderProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.PoolPointsByHolderProductionTableName, err)
	}

	// Staging table: ORDER BY (height, pool_id, address) for efficient cleanup/promotion WHERE height = ?
	stagingQuery := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (height, pool_id, address)
	`, db.Name, indexermodels.PoolPointsByHolderStagingTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.PoolPointsByHolderStagingTableName, err)
	}

	return nil
}

// InsertPoolPointsByHolderStaging inserts pool points snapshots to the staging table.
// This follows the two-phase commit pattern for data consistency.
func (db *DB) InsertPoolPointsByHolderStaging(ctx context.Context, holders []*indexermodels.PoolPointsByHolder) error {
	return db.insertPoolPointsByHolder(ctx, indexermodels.PoolPointsByHolderStagingTableName, holders)
}

// InsertPoolPointsByHolderProduction inserts pool points snapshots to the production table.
func (db *DB) InsertPoolPointsByHolderProduction(ctx context.Context, holders []*indexermodels.PoolPointsByHolder) error {
	return db.insertPoolPointsByHolder(ctx, indexermodels.PoolPointsByHolderProductionTableName, holders)
}

func (db *DB) insertPoolPointsByHolder(ctx context.Context, tableName string, holders []*indexermodels.PoolPointsByHolder) error {
	if len(holders) == 0 {
		return nil
	}

	query := fmt.Sprintf(
		`INSERT INTO "%s"."%s" (address, pool_id, height, height_time, committee, points, liquidity_pool_points, liquidity_pool_id, pool_amount) VALUES`,
		db.Name, tableName,
	)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, holder := range holders {
		err = batch.Append(
			holder.Address,
			holder.PoolID,
			holder.Height,
			holder.HeightTime,
			holder.Committee,
			holder.Points,
			holder.LiquidityPoolPoints,
			holder.LiquidityPoolID,
			holder.PoolAmount,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

// initPoolPointsCreatedHeightView creates a materialized view to calculate the minimum height
// at which each (pool_id, address) pair was created.
//
// The materialized view automatically updates as new data is inserted into the pool_points_by_holder table,
// providing an efficient way to query when a holder first acquired points in a pool.
//
// ORDER BY Optimization:
// pool_id comes before address following ClickHouse best practice of "lower cardinality first".
// This matches the base table ORDER BY for consistent access patterns.
//
// Query usage: SELECT pool_id, address, created_height FROM pool_points_created_height WHERE pool_id = ? AND address = ?
func (db *DB) initPoolPointsCreatedHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."pool_points_created_height"
		ENGINE = %s
		ORDER BY (pool_id, address)
		AS SELECT
			pool_id,
			address,
			minSimpleState(height) as created_height
		FROM "%s"."pool_points_by_holder"
		GROUP BY pool_id, address
	`, db.Name, clickhouse.ReplicatedEngine(clickhouse.AggregatingMergeTree, ""), db.Name)

	return db.Exec(ctx, query)
}

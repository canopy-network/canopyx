package chain

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initDexPoolPointsByHolder initializes the dex_pool_points_by_holder table and its staging table.
// This table stores versioned snapshots of liquidity provider pool points using the snapshot-on-change pattern.
// Uses ReplacingMergeTree(height) to deduplicate and keep the latest state at each height.
func (db *DB) initDexPoolPointsByHolder(ctx context.Context) error {
	queryTemplate := `
        CREATE TABLE IF NOT EXISTS "%s"."%s" (
			address String CODEC(ZSTD(1)),
			pool_id UInt64 CODEC(Delta, ZSTD(1)),
			height UInt64 CODEC(DoubleDelta, LZ4),
			height_time DateTime64(6) CODEC(DoubleDelta, LZ4),
			committee UInt64 CODEC(Delta, ZSTD(1)),
			points UInt64 CODEC(Delta, ZSTD(3)),
			liquidity_pool_points UInt64 CODEC(Delta, ZSTD(3)),
			liquidity_pool_id UInt64 CODEC(Delta, ZSTD(1))
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (address, pool_id, height)
	`

	// Create production table
	productionQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.DexPoolPointsByHolderProductionTableName)
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.DexPoolPointsByHolderProductionTableName, err)
	}

	// Create staging table
	stagingQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.DexPoolPointsByHolderStagingTableName)
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.DexPoolPointsByHolderStagingTableName, err)
	}

	return nil
}

// InsertDexPoolPointsByHolderStaging inserts pool points snapshots to the staging table.
// This follows the two-phase commit pattern for data consistency.
func (db *DB) InsertDexPoolPointsByHolderStaging(ctx context.Context, holders []*indexermodels.DexPoolPointsByHolder) error {
	if len(holders) == 0 {
		return nil
	}

	query := fmt.Sprintf(
		`INSERT INTO %s (address, pool_id, height, height_time, committee, points, liquidity_pool_points, liquidity_pool_id) VALUES`,
		indexermodels.DexPoolPointsByHolderStagingTableName,
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
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

// initDexPoolPointsCreatedHeightView creates a materialized view to calculate the minimum height
// at which each (address, pool_id) pair was created.
//
// The materialized view automatically updates as new data is inserted into the dex_pool_points_by_holder table,
// providing an efficient way to query when a holder first acquired points in a pool.
//
// Query usage: SELECT address, pool_id, created_height FROM dex_pool_points_created_height WHERE address = ? AND pool_id = ?
func (db *DB) initDexPoolPointsCreatedHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."dex_pool_points_created_height"
		ENGINE = AggregatingMergeTree()
		ORDER BY (address, pool_id)
		AS SELECT
			address,
			pool_id,
			min(height) as created_height
		FROM "%s"."dex_pool_points_by_holder"
		GROUP BY address, pool_id
	`, db.Name, db.Name)

	return db.Exec(ctx, query)
}

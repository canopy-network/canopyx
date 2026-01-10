package global

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

func (db *DB) initPoolPointsByHolder(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToGlobalSchemaSQL(indexermodels.PoolPointsByHolderColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, pool_id, address, height)
		SETTINGS index_granularity = 8192
	`, db.Name, indexermodels.PoolPointsByHolderProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	return db.Exec(ctx, query)
}

// InsertPoolPointsByHolder inserts pool points by holder into the table with chain_id.
func (db *DB) InsertPoolPointsByHolder(ctx context.Context, holders []*indexermodels.PoolPointsByHolder) error {
	if len(holders) == 0 {
		return nil
	}

	query := fmt.Sprintf(
		`INSERT INTO "%s"."%s" (chain_id, address, pool_id, height, height_time, committee, points, liquidity_pool_points, liquidity_pool_id, pool_amount) VALUES`,
		db.Name, indexermodels.PoolPointsByHolderProductionTableName,
	)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	// Ensure the batch is closed, especially if not all data is sent immediately
	defer func() { _ = batch.Close() }()

	for _, h := range holders {
		err = batch.Append(
			db.ChainID,
			h.Address,
			h.PoolID,
			h.Height,
			h.HeightTime,
			h.Committee,
			h.Points,
			h.LiquidityPoolPoints,
			h.LiquidityPoolID,
			h.PoolAmount,
		)
		if err != nil {
			_ = batch.Abort()
			return err
		}
	}

	return batch.Send()
}

func (db *DB) initPoolPointsCreatedHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."pool_points_created_height"
		ENGINE = %s
		ORDER BY (chain_id, pool_id, address)
		AS SELECT
			chain_id,
			pool_id,
			address,
			minSimpleState(height) as created_height
		FROM "%s"."pool_points_by_holder"
		GROUP BY chain_id, pool_id, address
	`, db.Name, clickhouse.ReplicatedEngine(clickhouse.AggregatingMergeTree, ""), db.Name)

	return db.Exec(ctx, query)
}

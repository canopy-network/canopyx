package global

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initPools creates the pools table with chain_id support.
func (db *DB) initPools(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToGlobalCrossChainSchemaSQL(indexermodels.PoolColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, pool_id, height)
	`, db.Name, indexermodels.PoolsProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	return db.Exec(ctx, query)
}

// InsertPools inserts pools into the pools table with chain_id.
func (db *DB) InsertPools(ctx context.Context, pools []*indexermodels.Pool) error {
	if len(pools) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, pool_id, height, pool_chain_id, amount, total_points, lp_count, height_time,
		liquidity_pool_id, holding_pool_id, escrow_pool_id, reward_pool_id,
		amount_delta, total_points_delta, lp_count_delta
	) VALUES`, db.Name, indexermodels.PoolsProductionTableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	// Ensure the batch is closed, especially if not all data is sent immediately
	defer func() { _ = batch.Close() }()

	for _, pool := range pools {
		err = batch.Append(
			db.ChainID,
			pool.PoolID,
			pool.Height,
			pool.ChainID, // pool_chain_id
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
			_ = batch.Abort()
			return err
		}
	}

	return batch.Send()
}

func (db *DB) initPoolCreatedHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."pool_created_height"
		ENGINE = %s
		ORDER BY (chain_id, pool_id)
		AS SELECT
			chain_id,
			pool_id,
			minSimpleState(height) as created_height
		FROM "%s"."pools"
		GROUP BY chain_id, pool_id
	`, db.Name, clickhouse.ReplicatedEngine(clickhouse.AggregatingMergeTree, ""), db.Name)

	return db.Exec(ctx, query)
}
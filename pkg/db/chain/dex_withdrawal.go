package chain

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initDexWithdrawals initializes the dex_withdrawals table and its staging table.
// The production table uses aggressive compression for storage optimization.
// The staging table has the same schema but no TTL (TTL only on production).
func (db *DB) initDexWithdrawals(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.DexWithdrawalColumns)

	// Production table: ORDER BY (order_id, height) for efficient order_id lookups
	productionQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" %s (
			%s
		) ENGINE = %s
		ORDER BY (order_id, height)
	`, db.Name, indexermodels.DexWithdrawalsProductionTableName, db.OnCluster(), schemaSQL, db.Engine(indexermodels.DexWithdrawalsProductionTableName, "ReplacingMergeTree", "height"))
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.DexWithdrawalsProductionTableName, err)
	}

	// Staging table: ORDER BY (height, order_id) for efficient cleanup/promotion WHERE height = ?
	stagingQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" %s (
			%s
		) ENGINE = %s
		ORDER BY (height, order_id)
	`, db.Name, indexermodels.DexWithdrawalsStagingTableName, db.OnCluster(), schemaSQL, db.Engine(indexermodels.DexWithdrawalsStagingTableName, "ReplacingMergeTree", "height"))
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.DexWithdrawalsStagingTableName, err)
	}

	return nil
}

// InsertDexWithdrawalsStaging persists staged DEX withdrawal snapshots for the chain.
func (db *DB) InsertDexWithdrawalsStaging(ctx context.Context, withdrawals []*indexermodels.DexWithdrawal) error {
	if len(withdrawals) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."dex_withdrawals_staging" (
		order_id, height, height_time, committee, address,
		percent, state, local_amount, remote_amount, points_burned
	) VALUES`, db.Name)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, withdrawal := range withdrawals {
		err = batch.Append(
			withdrawal.OrderID,
			withdrawal.Height,
			withdrawal.HeightTime,
			withdrawal.Committee,
			withdrawal.Address,
			withdrawal.Percent,
			withdrawal.State,
			withdrawal.LocalAmount,
			withdrawal.RemoteAmount,
			withdrawal.PointsBurned,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

// initDexWithdrawalCreatedHeightView creates a materialized view to calculate the minimum height
// at which each DEX withdrawal was created. This replaces the removed created_height column.
//
// The materialized view automatically updates as new data is inserted into the dex_withdrawals table,
// providing an efficient way to query withdrawal creation heights without storing the value
// in every withdrawal snapshot row.
//
// Query usage: SELECT order_id, created_height FROM dex_withdrawal_created_height WHERE order_id = ?
func (db *DB) initDexWithdrawalCreatedHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."dex_withdrawal_created_height" %s
		ENGINE = %s
		ORDER BY order_id
		AS SELECT
			order_id,
			min(height) as created_height
		FROM "%s"."dex_withdrawals"
		GROUP BY order_id
	`, db.Name, db.OnCluster(), db.Engine("dex_withdrawal_created_height", "ReplacingMergeTree", "created_height"), db.Name)

	return db.Exec(ctx, query)
}

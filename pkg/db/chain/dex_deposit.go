package chain

import (
    "context"
    "fmt"

    "github.com/canopy-network/canopyx/pkg/db/clickhouse"

    "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
    indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initDexDeposits initializes the dex_deposits table and its staging table.
// The production table uses aggressive compression for storage optimization.
// The staging table has the same schema but no TTL (TTL only on production).
func (db *DB) initDexDeposits(ctx context.Context) error {
    schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.DexDepositColumns)

    // Production table: ORDER BY (order_id, height) for efficient order_id lookups
    productionQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (order_id, height)
	`, db.Name, indexermodels.DexDepositsProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
    if err := db.Exec(ctx, productionQuery); err != nil {
        return fmt.Errorf("create %s: %w", indexermodels.DexDepositsProductionTableName, err)
    }

    // Staging table: ORDER BY (height, order_id) for efficient cleanup/promotion WHERE height = ?
    stagingQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (height, order_id)
	`, db.Name, indexermodels.DexDepositsStagingTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
    if err := db.Exec(ctx, stagingQuery); err != nil {
        return fmt.Errorf("create %s: %w", indexermodels.DexDepositsStagingTableName, err)
    }

    return nil
}

// InsertDexDepositsStaging persists staged DEX deposit snapshots for the chain.
func (db *DB) InsertDexDepositsStaging(ctx context.Context, deposits []*indexermodels.DexDeposit) error {
    if len(deposits) == 0 {
        return nil
    }

    query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		order_id, height, height_time, committee, address,
		amount, state, local_origin, points_received
	) VALUES`, db.Name, indexermodels.DexDepositsStagingTableName)

    batch, err := db.PrepareBatch(ctx, query)
    if err != nil {
        return err
    }
    defer func(batch driver.Batch) {
        _ = batch.Abort()
    }(batch)

    for _, deposit := range deposits {
        err = batch.Append(
            deposit.OrderID,
            deposit.Height,
            deposit.HeightTime,
            deposit.Committee,
            deposit.Address,
            deposit.Amount,
            deposit.State,
            deposit.LocalOrigin,
            deposit.PointsReceived,
        )
        if err != nil {
            return err
        }
    }

    return batch.Send()
}

// initDexDepositCreatedHeightView creates a materialized view to calculate the minimum height
// at which each DEX deposit was created. This replaces the removed created_height column.
//
// The materialized view automatically updates as new data is inserted into the dex_deposits table,
// providing an efficient way to query deposit creation heights without storing the value
// in every deposit snapshot row.
//
// Query usage: SELECT order_id, created_height FROM dex_deposit_created_height WHERE order_id = ?
func (db *DB) initDexDepositCreatedHeightView(ctx context.Context) error {
    query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."dex_deposit_created_height"
		ENGINE = %s
		ORDER BY order_id
		AS SELECT
			order_id,
			min(height) as created_height
		FROM "%s"."dex_deposits"
		GROUP BY order_id
	`, db.Name, clickhouse.ReplicatedEngine(clickhouse.AggregatingMergeTree, ""), db.Name)

    return db.Exec(ctx, query)
}

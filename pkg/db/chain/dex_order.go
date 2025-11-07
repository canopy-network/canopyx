package chain

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initDexOrders initializes the dex_orders table and its staging table.
// The production table uses aggressive compression for storage optimization.
// The staging table has the same schema but no TTL (TTL only on production).
func (db *DB) initDexOrders(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.DexOrderColumns)
	queryTemplate := `
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (order_id, height)
	`

	// Create production table
	productionQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.DexOrdersProductionTableName, schemaSQL)
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.DexOrdersProductionTableName, err)
	}

	// Create staging table
	stagingQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.DexOrdersStagingTableName, schemaSQL)
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.DexOrdersStagingTableName, err)
	}

	return nil
}

// InsertDexOrdersStaging persists staged DEX order snapshots for the chain.
func (db *DB) InsertDexOrdersStaging(ctx context.Context, orders []*indexermodels.DexOrder) error {
	if len(orders) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s".dex_orders_staging (
		order_id, height, height_time, committee, address,
		amount_for_sale, requested_amount, state, success,
		sold_amount, bought_amount, local_origin, locked_height
	) VALUES`, db.Name)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, order := range orders {
		err = batch.Append(
			order.OrderID,
			order.Height,
			order.HeightTime,
			order.Committee,
			order.Address,
			order.AmountForSale,
			order.RequestedAmount,
			order.State,
			order.Success,
			order.SoldAmount,
			order.BoughtAmount,
			order.LocalOrigin,
			order.LockedHeight,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

// initDexOrderCreatedHeightView creates a materialized view to calculate the minimum height
// at which each DEX order was created. This replaces the removed created_height column.
//
// The materialized view automatically updates as new data is inserted into the dex_orders table,
// providing an efficient way to query order creation heights without storing the value
// in every order snapshot row.
//
// Query usage: SELECT order_id, created_height FROM dex_order_created_height WHERE order_id = ?
func (db *DB) initDexOrderCreatedHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."dex_order_created_height"
		ENGINE = AggregatingMergeTree()
		ORDER BY order_id
		AS SELECT
			order_id,
			min(height) as created_height
		FROM "%s"."dex_orders"
		GROUP BY order_id
	`, db.Name, db.Name)

	return db.Exec(ctx, query)
}

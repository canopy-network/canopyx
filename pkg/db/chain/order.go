package chain

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initOrders initializes the orders table and its staging table.
// The production table uses aggressive compression for storage optimization.
// The staging table has the same schema but no TTL (TTL only on production).
func (db *DB) initOrders(ctx context.Context) error {
	queryTemplate := `
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			order_id String CODEC(ZSTD(1)),
			height UInt64 CODEC(DoubleDelta, LZ4),
			height_time DateTime64(6) CODEC(DoubleDelta, LZ4),
			committee UInt64 CODEC(Delta, ZSTD(3)),
			amount_for_sale UInt64 CODEC(Delta, ZSTD(3)),
			requested_amount UInt64 CODEC(Delta, ZSTD(3)),
			seller_address String CODEC(ZSTD(1)),
			buyer_address Nullable(String) CODEC(ZSTD(1)),
			deadline Nullable(UInt64) CODEC(Delta, ZSTD(3)),
			status LowCardinality(String)
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (order_id, height)
	`

	// Create production table
	productionQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.OrdersProductionTableName)
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.OrdersProductionTableName, err)
	}

	// Create staging table
	stageQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.OrdersStagingTableName)
	if err := db.Exec(ctx, stageQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.OrdersStagingTableName, err)
	}

	return nil
}

// InsertOrdersStaging persists staged order snapshots for the chain.
func (db *DB) InsertOrdersStaging(ctx context.Context, orders []*indexermodels.Order) error {
	if len(orders) == 0 {
		return nil
	}

	query := `INSERT INTO orders_staging (order_id, height, height_time, committee, amount_for_sale, requested_amount, seller_address, buyer_address, deadline, status) VALUES`
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
			order.AmountForSale,
			order.RequestedAmount,
			order.SellerAddress,
			order.BuyerAddress,
			order.Deadline,
			order.Status,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

// initOrderCreatedHeightView creates a materialized view to calculate the minimum height
// at which each order was created. This replaces the removed created_height column.
//
// The materialized view automatically updates as new data is inserted into the orders table,
// providing an efficient way to query order creation heights without storing the value
// in every order snapshot row.
//
// Query usage: SELECT order_id, created_height FROM order_created_height WHERE order_id = ?
func (db *DB) initOrderCreatedHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."order_created_height"
		ENGINE = AggregatingMergeTree()
		ORDER BY order_id
		AS SELECT
			order_id,
			min(height) as created_height
		FROM "%s"."orders"
		GROUP BY order_id
	`, db.Name, db.Name)

	return db.Exec(ctx, query)
}

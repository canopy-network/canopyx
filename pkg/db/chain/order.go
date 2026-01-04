package chain

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initOrders initializes the orders table and its staging table.
// The production table uses aggressive compression for storage optimization.
// The staging table has the same schema but no TTL (TTL only on production).
func (db *DB) initOrders(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.OrderColumns)

	// Production table: ORDER BY (order_id, height) for efficient order_id lookups
	productionQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (order_id, height)
	`, db.Name, indexermodels.OrdersProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.OrdersProductionTableName, err)
	}

	// Staging table: ORDER BY (height, order_id) for efficient cleanup/promotion WHERE height = ?
	stagingQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (height, order_id)
	`, db.Name, indexermodels.OrdersStagingTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.OrdersStagingTableName, err)
	}

	return nil
}

// InsertOrdersStaging persists staged order snapshots for the chain.
func (db *DB) InsertOrdersStaging(ctx context.Context, orders []*indexermodels.Order) error {
	if len(orders) == 0 {
		return nil
	}

	query := fmt.Sprintf(
		`INSERT INTO "%s"."%s" (order_id, height, height_time, committee, data, amount_for_sale, requested_amount, seller_receive_address, buyer_send_address, buyer_receive_address, buyer_chain_deadline, sellers_send_address, status) VALUES`,
		db.Name,
		indexermodels.OrdersStagingTableName,
	)
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
			order.Data,
			order.AmountForSale,
			order.RequestedAmount,
			order.SellerReceiveAddress,
			order.BuyerSendAddress,
			order.BuyerReceiveAddress,
			order.BuyerChainDeadline,
			order.SellersSendAddress,
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
		ENGINE = %s
		ORDER BY order_id
		AS SELECT
			order_id,
			minSimpleState(height) as created_height
		FROM "%s"."orders"
		GROUP BY order_id
	`, db.Name, clickhouse.ReplicatedEngine(clickhouse.AggregatingMergeTree, ""), db.Name)

	return db.Exec(ctx, query)
}

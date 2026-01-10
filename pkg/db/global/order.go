package global

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

func (db *DB) initOrders(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToGlobalSchemaSQL(indexermodels.OrderColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, order_id, height)
	`, db.Name, indexermodels.OrdersProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	return db.Exec(ctx, query)
}

// InsertOrders inserts orders into the orders table with chain_id.
func (db *DB) InsertOrders(ctx context.Context, orders []*indexermodels.Order) error {
	if len(orders) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, order_id, height, height_time, committee, data, amount_for_sale, requested_amount,
		seller_receive_address, buyer_send_address, buyer_receive_address, buyer_chain_deadline,
		sellers_send_address, status
	) VALUES`, db.Name, indexermodels.OrdersProductionTableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	// Ensure the batch is closed, especially if not all data is sent immediately
	defer func() { _ = batch.Close() }()

	for _, order := range orders {
		err = batch.Append(
			db.ChainID,
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
			_ = batch.Abort()
			return err
		}
	}

	return batch.Send()
}

func (db *DB) initOrderCreatedHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."order_created_height"
		ENGINE = %s
		ORDER BY (chain_id, order_id)
		AS SELECT
			chain_id,
			order_id,
			minSimpleState(height) as created_height
		FROM "%s"."orders"
		GROUP BY chain_id, order_id
	`, db.Name, clickhouse.ReplicatedEngine(clickhouse.AggregatingMergeTree, ""), db.Name)

	return db.Exec(ctx, query)
}
package global

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// --- DEX Prices ---

func (db *DB) initDexPrices(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToGlobalSchemaSQL(indexermodels.DexPriceColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, local_chain_id, remote_chain_id, height)
	`, db.Name, indexermodels.DexPricesProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	return db.Exec(ctx, query)
}

// InsertDexPrices inserts DEX prices into the dex_prices table with chain_id.
func (db *DB) InsertDexPrices(ctx context.Context, prices []*indexermodels.DexPrice) error {
	if len(prices) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, local_chain_id, remote_chain_id, height, local_pool, remote_pool, price_e6, height_time,
		price_delta, local_pool_delta, remote_pool_delta
	) VALUES`, db.Name, indexermodels.DexPricesProductionTableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	// Ensure the batch is closed, especially if not all data is sent immediately
	defer func() { _ = batch.Close() }()

	for _, price := range prices {
		err = batch.Append(
			db.ChainID,
			price.LocalChainID,
			price.RemoteChainID,
			price.Height,
			price.LocalPool,
			price.RemotePool,
			price.PriceE6,
			price.HeightTime,
			price.PriceDelta,
			price.LocalPoolDelta,
			price.RemotePoolDelta,
		)
		if err != nil {
			_ = batch.Abort()
			return err
		}
	}

	return batch.Send()
}

// --- DEX Orders ---

func (db *DB) initDexOrders(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToGlobalSchemaSQL(indexermodels.DexOrderColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, order_id, height)
	`, db.Name, indexermodels.DexOrdersProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	return db.Exec(ctx, query)
}

// InsertDexOrders inserts DEX orders into the dex_orders table with chain_id.
func (db *DB) InsertDexOrders(ctx context.Context, orders []*indexermodels.DexOrder) error {
	if len(orders) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, order_id, height, height_time, committee, address,
		amount_for_sale, requested_amount, state, success,
		sold_amount, bought_amount, local_origin, locked_height
	) VALUES`, db.Name, indexermodels.DexOrdersProductionTableName)

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
			_ = batch.Abort()
			return err
		}
	}

	return batch.Send()
}

func (db *DB) initDexOrderCreatedHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."dex_order_created_height"
		ENGINE = %s
		ORDER BY (chain_id, order_id)
		AS SELECT
			chain_id,
			order_id,
			minSimpleState(height) as created_height
		FROM "%s"."dex_orders"
		GROUP BY chain_id, order_id
	`, db.Name, clickhouse.ReplicatedEngine(clickhouse.AggregatingMergeTree, ""), db.Name)

	return db.Exec(ctx, query)
}

// --- DEX Deposits ---

func (db *DB) initDexDeposits(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToGlobalSchemaSQL(indexermodels.DexDepositColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, order_id, height)
	`, db.Name, indexermodels.DexDepositsProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	return db.Exec(ctx, query)
}

// InsertDexDeposits inserts DEX deposits into the dex_deposits table with chain_id.
func (db *DB) InsertDexDeposits(ctx context.Context, deposits []*indexermodels.DexDeposit) error {
	if len(deposits) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, order_id, height, height_time, committee, address,
		amount, state, local_origin, points_received
	) VALUES`, db.Name, indexermodels.DexDepositsProductionTableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	// Ensure the batch is closed, especially if not all data is sent immediately
	defer func() { _ = batch.Close() }()

	for _, deposit := range deposits {
		err = batch.Append(
			db.ChainID,
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
			_ = batch.Abort()
			return err
		}
	}

	return batch.Send()
}

func (db *DB) initDexDepositCreatedHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."dex_deposit_created_height"
		ENGINE = %s
		ORDER BY (chain_id, order_id)
		AS SELECT
			chain_id,
			order_id,
			minSimpleState(height) as created_height
		FROM "%s"."dex_deposits"
		GROUP BY chain_id, order_id
	`, db.Name, clickhouse.ReplicatedEngine(clickhouse.AggregatingMergeTree, ""), db.Name)

	return db.Exec(ctx, query)
}

// --- DEX Withdrawals ---

func (db *DB) initDexWithdrawals(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToGlobalSchemaSQL(indexermodels.DexWithdrawalColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, order_id, height)
	`, db.Name, indexermodels.DexWithdrawalsProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	return db.Exec(ctx, query)
}

// InsertDexWithdrawals inserts DEX withdrawals into the dex_withdrawals table with chain_id.
func (db *DB) InsertDexWithdrawals(ctx context.Context, withdrawals []*indexermodels.DexWithdrawal) error {
	if len(withdrawals) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, order_id, height, height_time, committee, address,
		percent, state, local_amount, remote_amount, points_burned
	) VALUES`, db.Name, indexermodels.DexWithdrawalsProductionTableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	// Ensure the batch is closed, especially if not all data is sent immediately
	defer func() { _ = batch.Close() }()

	for _, withdrawal := range withdrawals {
		err = batch.Append(
			db.ChainID,
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
			_ = batch.Abort()
			return err
		}
	}

	return batch.Send()
}

func (db *DB) initDexWithdrawalCreatedHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."dex_withdrawal_created_height"
		ENGINE = %s
		ORDER BY (chain_id, order_id)
		AS SELECT
			chain_id,
			order_id,
			minSimpleState(height) as created_height
		FROM "%s"."dex_withdrawals"
		GROUP BY chain_id, order_id
	`, db.Name, clickhouse.ReplicatedEngine(clickhouse.AggregatingMergeTree, ""), db.Name)

	return db.Exec(ctx, query)
}
package chain

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initTransactions creates the transactions table and its staging table with ZSTD compression.
func (db *DB) initTransactions(ctx context.Context) error {
	queryTemplate := `
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			height UInt64,
			tx_hash String,
			time DateTime64(6),
			height_time DateTime64(6),
			message_type LowCardinality(String),
			signer String,
			counterparty Nullable(String),
			amount Nullable(UInt64),
			fee UInt64,
			validator_address Nullable(String),
			commission Nullable(Float64),
			chain_id Nullable(UInt64),
			sell_amount Nullable(UInt64),
			buy_amount Nullable(UInt64),
			liquidity_amount Nullable(UInt64),
			order_id Nullable(String),
			price Nullable(Float64),
			param_key Nullable(String),
			param_value Nullable(String),
			committee_id Nullable(UInt64),
			recipient Nullable(String),
			msg String CODEC(ZSTD(3)),
			public_key Nullable(String) CODEC(ZSTD(3)),
			signature Nullable(String) CODEC(ZSTD(3))
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (height, tx_hash)
	`

	// Create production table
	productionQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.TxsProductionTableName)
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.TxsProductionTableName, err)
	}

	// Create staging table
	stagingQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.TxsStagingTableName)
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.TxsStagingTableName, err)
	}

	return nil
}

// InsertTransactionsStaging persists transactions into the txs_staging table.
// This follows the two-phase commit pattern for data consistency.
// IMPORTANT: Must include all 24 columns to match the staging table schema.
func (db *DB) InsertTransactionsStaging(ctx context.Context, txs []*indexermodels.Transaction) error {
	if len(txs) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s".txs_staging (
		height, tx_hash, time, height_time, message_type, signer, counterparty, amount, fee,
		validator_address, commission, chain_id, sell_amount, buy_amount, liquidity_amount,
		order_id, price, param_key, param_value, committee_id, recipient,
		msg, public_key, signature
	) VALUES`, db.Name)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, tx := range txs {
		err = batch.Append(
			tx.Height,
			tx.TxHash,
			tx.Time,
			tx.HeightTime,
			tx.MessageType,
			tx.Signer,
			tx.Counterparty,
			tx.Amount,
			tx.Fee,
			tx.ValidatorAddress,
			tx.Commission,
			tx.ChainID,
			tx.SellAmount,
			tx.BuyAmount,
			tx.LiquidityAmt,
			tx.OrderID,
			tx.Price,
			tx.ParamKey,
			tx.ParamValue,
			tx.CommitteeID,
			tx.Recipient,
			tx.Msg,
			tx.PublicKey,
			tx.Signature,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

func (db *DB) DeleteTransactions(ctx context.Context, height uint64) error {
	stmt := fmt.Sprintf(`ALTER TABLE "%s"."txs" DELETE WHERE height = ?`, db.Name)
	return db.Db.Exec(ctx, stmt, height)
}

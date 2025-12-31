package chain

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initTransactions creates the transactions table and its staging table with ZSTD compression.
func (db *DB) initTransactions(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.TransactionColumns)

	queryTemplate := `
		CREATE TABLE IF NOT EXISTS "%s"."%s" %s (
			%s
		) ENGINE = %s
		ORDER BY (height, tx_hash)
	`

	// Create production table
	productionQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.TxsProductionTableName, db.OnCluster(), schemaSQL, db.Engine(indexermodels.TxsProductionTableName, "ReplacingMergeTree", "height"))
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.TxsProductionTableName, err)
	}

	// Create staging table
	stagingQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.TxsStagingTableName, db.OnCluster(), schemaSQL, db.Engine(indexermodels.TxsStagingTableName, "ReplacingMergeTree", "height"))
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.TxsStagingTableName, err)
	}

	return nil
}

// InsertTransactionsStaging persists transactions into the txs_staging table.
// This follows the two-phase commit pattern for data consistency.
// IMPORTANT: Must include all 33 columns to match the staging table schema.
func (db *DB) InsertTransactionsStaging(ctx context.Context, txs []*indexermodels.Transaction) error {
	if len(txs) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."txs_staging" (
		height, tx_hash, tx_index, time, height_time, created_height, network_id,
		message_type, signer, counterparty, amount, fee, memo,
		validator_address, commission, chain_id, sell_amount, buy_amount, liquidity_amount,
		order_id, price, param_key, param_value, committee_id, recipient, poll_hash,
		buyer_receive_address, buyer_send_address, buyer_chain_deadline,
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
			tx.TxIndex,
			tx.Time,
			tx.HeightTime,
			tx.CreatedHeight,
			tx.NetworkID,
			tx.MessageType,
			tx.Signer,
			tx.Counterparty,
			tx.Amount,
			tx.Fee,
			tx.Memo,
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
			tx.PollHash,
			tx.BuyerReceiveAddress,
			tx.BuyerSendAddress,
			tx.BuyerChainDeadline,
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
	stmt := fmt.Sprintf(`ALTER TABLE "%s"."txs" ON CLUSTER canopyx DELETE WHERE height = ?`, db.Name)
	return db.Db.Exec(ctx, stmt, height)
}

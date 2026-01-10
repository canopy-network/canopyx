package global

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initTransactions creates the transactions table with chain_id support.
// Uses ColumnsToGlobalCrossChainSchemaSQL because TransactionColumns has a semantic
// chain_id column that gets renamed to tx_chain_id to avoid conflict with the global chain_id.
func (db *DB) initTransactions(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToGlobalCrossChainSchemaSQL(indexermodels.TransactionColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, tx_hash, height)
	`, db.Name, indexermodels.TxsProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	return db.Exec(ctx, query)
}

// InsertTransactions persists transactions into the txs table.
func (db *DB) InsertTransactions(ctx context.Context, txs []*indexermodels.Transaction) error {
	if len(txs) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, height, tx_hash, tx_index, time, height_time, created_height, network_id,
		message_type, signer, amount, fee, memo,
		validator_address, commission, tx_chain_id, sell_amount, buy_amount, liquidity_amount, liquidity_percent,
		order_id, price, param_key, param_value, committee_id, recipient, poll_hash,
		buyer_receive_address, buyer_send_address, buyer_chain_deadline,
		msg, public_key, signature
	) VALUES`, db.Name, indexermodels.TxsProductionTableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	// Ensure the batch is closed, especially if not all data is sent immediately
	defer func() { _ = batch.Close() }()

	for _, tx := range txs {
		err = batch.Append(
			db.ChainID, // chain_id from DB context
			tx.Height,
			tx.TxHash,
			tx.TxIndex,
			tx.Time,
			tx.HeightTime,
			tx.CreatedHeight,
			tx.NetworkID,
			tx.MessageType,
			tx.Signer,
			tx.Amount,
			tx.Fee,
			tx.Memo,
			tx.ValidatorAddress,
			tx.Commission,
			tx.ChainID, // tx_chain_id (the original semantic chain_id in the transaction)
			tx.SellAmount,
			tx.BuyAmount,
			tx.LiquidityAmt,
			tx.LiquidityPercent,
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
			_ = batch.Abort()
			return err
		}
	}

	return batch.Send()
}

// DeleteTransactions removes transactions for the given height and chain.
func (db *DB) DeleteTransactions(ctx context.Context, height uint64) error {
	stmt := fmt.Sprintf(
		`DELETE FROM "%s"."%s" WHERE chain_id = ? AND height = ?`,
		db.Name, indexermodels.TxsProductionTableName,
	)
	return db.Db.Exec(ctx, stmt, db.ChainID, height)
}

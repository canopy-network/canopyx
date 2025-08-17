package rpc

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/go-jose/go-jose/v4/json"
)

// TODO: this require a bit more of investigation since properties like "recipient" are not always present because not
//   not all the transactions are sends.

// Transaction represents a transaction.
type Transaction struct {
	Sender      string `json:"sender"`
	Recipient   string `json:"recipient"`
	MessageType string `json:"messageType"`
	Height      int    `json:"height"`
	Transaction struct {
		Type string `json:"type"`
		Msg  struct {
			FromAddress string `json:"fromAddress"`
			ToAddress   string `json:"toAddress"`
			Amount      int    `json:"amount"`
		} `json:"msg"`
		Signature struct {
			PublicKey string `json:"publicKey"`
			Signature string `json:"signature"`
		} `json:"signature"`
		Time          int64 `json:"time"`
		CreatedHeight int   `json:"createdHeight"`
		Fee           int   `json:"fee"`
		NetworkID     int   `json:"networkID"`
		ChainID       int   `json:"chainID"`
	} `json:"transaction"`
	TxHash string `json:"txHash"`
}

// ToTransaction maps a rpc.Transaction into the lean Transaction row.
func (tx *Transaction) ToTransaction() *indexer.Transaction {
	// signer is always present per your note
	signer := tx.Sender

	// infer counterparty if present (send-like)
	var cp *string
	if to := tx.Transaction.Msg.ToAddress; to != "" {
		cp = &to
	} else if tx.Recipient != "" {
		r := tx.Recipient
		cp = &r
	}

	// optional numerics → pointers when > 0
	var amount *uint64
	if tx.Transaction.Msg.Amount > 0 {
		v := uint64(tx.Transaction.Msg.Amount)
		amount = &v
	}
	var fee uint64
	if tx.Transaction.Fee > 0 {
		v := uint64(tx.Transaction.Fee)
		fee = v
	}
	var createdHeight uint64
	if tx.Transaction.CreatedHeight > 0 {
		v := uint64(tx.Transaction.CreatedHeight)
		createdHeight = v
	}

	return &indexer.Transaction{
		Height:        uint64(tx.Height),
		TxHash:        tx.TxHash,
		Time:          time.UnixMicro(tx.Transaction.Time),
		MessageType:   tx.MessageType,
		Counterparty:  cp,
		Signer:        signer,
		Amount:        amount,
		Fee:           fee,
		CreatedHeight: createdHeight,
	}
}

// ToTransactionRaw maps a rpc.Transaction into the heavy sidecar row.
func (tx *Transaction) ToTransactionRaw() *indexer.TransactionRaw {
	// compact JSON for the message payload (so arbitrary fields for other msg types still fit)
	var msgRaw *string
	if b, err := json.Marshal(tx.Transaction.Msg); err == nil {
		s := string(b)
		msgRaw = &s
	}

	var pub *string
	if k := tx.Transaction.Signature.PublicKey; k != "" {
		pub = &k
	}
	var sig *string
	if s := tx.Transaction.Signature.Signature; s != "" {
		sig = &s
	}

	return &indexer.TransactionRaw{
		Height:    uint64(tx.Height),
		TxHash:    tx.TxHash,
		MsgRaw:    msgRaw,
		PublicKey: pub,
		Signature: sig,
	}
}

// ToTxModels converts a Transaction to Transaction and TransactionRaw models.
func (tx *Transaction) ToTxModels() (*indexer.Transaction, *indexer.TransactionRaw) {
	return tx.ToTransaction(), tx.ToTransactionRaw()
}

// TxsByHeight returns all transactions for a given height.
func (c *HTTPClient) TxsByHeight(ctx context.Context, h uint64) ([]*indexer.Transaction, []*indexer.TransactionRaw, error) {
	txs, txsErr := ListPaged[*Transaction](ctx, c, txsByHeightPath, map[string]any{"height": h})
	if txsErr != nil {
		return nil, nil, txsErr
	}

	// cast rpc tx to db tx
	dbTxs := make([]*indexer.Transaction, 0, len(txs))
	dbTxsRaw := make([]*indexer.TransactionRaw, 0, len(txs))

	for _, t := range txs {
		tx, txRaw := t.ToTxModels()
		dbTxs = append(dbTxs, tx)
		dbTxsRaw = append(dbTxsRaw, txRaw)
	}

	return dbTxs, dbTxsRaw, nil
}

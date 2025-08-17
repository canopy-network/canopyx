package indexer

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/go-clickhouse/ch"
)

// ---------------------------
// Models
// ---------------------------

// Transaction (core, small, query‑friendly)
type Transaction struct {
	ch.CHModel `ch:"table:txs,engine:MergeTree(),order_by:(height,tx_hash)"`

	// Position
	Height uint64 `ch:"height"`
	TxHash string `ch:"tx_hash"`

	// Time
	Time time.Time `ch:"time,type:DateTime64(6)"`

	// Classification
	MessageType string `ch:"message_type,lc"` // LowCardinality(String)

	// Counterparty It’s the “other address involved in the tx,” i.e., who the signer interacted with:
	// 1. send: the recipient (toAddress / recipient)
	// 2. delegate/undelegate/stake etc.: typically a validator address (if/when present)
	// 3. contract calls or system txs: may be unknown → nil
	Counterparty *string `ch:"counterparty"` // nullable

	// Actor
	Signer string `ch:"signer"`

	// Value / fees (nullable)
	Amount        *uint64 `ch:"amount"`
	Fee           uint64  `ch:"fee"`
	CreatedHeight uint64  `ch:"created_height"`
}

// TransactionRaw (heavy fields; TTL 30 days)
type TransactionRaw struct {
	ch.CHModel `ch:"table:txs_raw"`

	Height    uint64  `ch:"height"`
	TxHash    string  `ch:"tx_hash"`
	MsgRaw    *string `ch:"msg_raw"` // compact JSON for unknown/varied payloads
	PublicKey *string `ch:"public_key"`
	Signature *string `ch:"signature"`
}

// ---------------------------
// Initializers
// ---------------------------

// InitTransactions creates the core table via builder, and the raw table with TTL via DDL.
// dbName must be the ClickHouse database (since ExecContext needs a fully qualified name).
func InitTransactions(ctx context.Context, db *ch.DB, dbName string) error {
	// Core fact table (builder)
	if _, err := db.NewCreateTable().
		Model((*Transaction)(nil)).
		IfNotExists().
		Engine("MergeTree").
		// the tag already has order_by:(height,tx_hash); being explicit keeps it obvious:
		Order("height,tx_hash").
		Exec(ctx); err != nil {
		return err
	}

	// Raw sidecar with TTL (use DDL to express TTL cleanly)
	ddlRaw := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS "%[1]s"."txs_raw" (
  height    UInt64,
  tx_hash   String,
  msg_raw   Nullable(String),
  public_key Nullable(String),
  signature  Nullable(String),
  created_at DateTime DEFAULT now()
) ENGINE = MergeTree
ORDER BY (height, tx_hash)
TTL created_at + INTERVAL 30 DAY DELETE
`, dbName)

	if _, err := db.ExecContext(ctx, ddlRaw); err != nil {
		return err
	}

	return nil
}

// ---------------------------
// Insert helpers
// ---------------------------

// InsertTransactions inserts one or many Transaction rows efficiently.
// TODO: we may want to insert in parallel
func InsertTransactions(ctx context.Context, db *ch.DB, transactions []*Transaction, rawTransactions []*TransactionRaw) error {
	txErr := InsertTransactionsCore(ctx, db, transactions...)
	if txErr != nil {
		return txErr
	}

	rawTxErr := InsertTransactionsRaw(ctx, db, rawTransactions...)
	if rawTxErr != nil {
		return rawTxErr
	}

	return nil
}

// InsertTransactionsCore inserts one or many Transaction rows efficiently.
func InsertTransactionsCore(ctx context.Context, db *ch.DB, rows ...*Transaction) error {
	if len(rows) == 0 {
		return nil
	}
	_, err := db.NewInsert().Model(&rows).Exec(ctx)
	return err
}

// InsertTransactionsRaw inserts one or many TransactionRaw rows.
func InsertTransactionsRaw(ctx context.Context, db *ch.DB, rows ...*TransactionRaw) error {
	if len(rows) == 0 {
		return nil
	}
	_, err := db.NewInsert().Model(&rows).Exec(ctx)
	return err
}

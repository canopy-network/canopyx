package indexer

import (
	"context"
	"time"

	"github.com/uptrace/go-clickhouse/ch"
)

// Transaction stores ALL transaction data in a single table.
// Common queryable fields are typed columns.
// Type-specific fields are stored in the compressed 'msg' JSON field.
// ClickHouse's columnar storage ensures list queries only read the columns they need.
type Transaction struct {
	ch.CHModel `ch:"table:txs,engine:ReplacingMergeTree(height),order_by:(height,tx_hash)"`

	// Primary key
	Height uint64 `ch:"height,pk" json:"height"`
	TxHash string `ch:"tx_hash,pk" json:"tx_hash"`

	// Time fields for queries
	Time       time.Time `ch:"time,type:DateTime64(6)" json:"time"`             // Transaction timestamp
	HeightTime time.Time `ch:"height_time,type:DateTime64(6)" json:"height_time"` // Block timestamp for range queries

	// Common filterable fields
	MessageType  string  `ch:"message_type,lc" json:"message_type"`            // LowCardinality(String) for efficient filtering
	Signer       string  `ch:"signer" json:"signer"`                           // Transaction signer address
	Counterparty *string `ch:"counterparty" json:"counterparty,omitempty"`     // Recipient/validator/pool/contract address
	Amount       *uint64 `ch:"amount" json:"amount,omitempty"`                 // Amount transferred/staked/delegated (null for votes, etc.)
	Fee          uint64  `ch:"fee" json:"fee"`                                 // Transaction fee

	// Full message as compressed JSON (ALL type-specific fields)
	Msg string `ch:"msg,codec:ZSTD(3)" json:"msg"` // Complete message data with ZSTD compression

	// Signature fields (compressed, permanent audit trail)
	PublicKey *string `ch:"public_key,codec:ZSTD(3)" json:"public_key,omitempty"` // Compressed with ZSTD
	Signature *string `ch:"signature,codec:ZSTD(3)" json:"signature,omitempty"`   // Compressed with ZSTD

	// Deduplication field
	CreatedHeight uint64 `ch:"created_height" json:"created_height"`
}

// InitTransactions creates the single transactions table with ZSTD compression.
func InitTransactions(ctx context.Context, db *ch.DB, dbName string) error {
	// Use the model's CHModel tag for table creation
	// The codec specifications in the struct tags will be applied
	_, err := db.NewCreateTable().
		Model((*Transaction)(nil)).
		IfNotExists().
		Exec(ctx)
	return err
}
package indexer

import (
	"context"
	"time"

	"github.com/uptrace/go-clickhouse/ch"
)

// Event stores ALL event data emitted during block processing.
// Events can be emitted in three contexts:
//   - begin_block: Events occurring at the start of block processing
//   - tx_hash: Events occurring during transaction execution (referenced by transaction hash)
//   - end_block: Events occurring at the end of block processing
//
// Common queryable fields are typed columns.
// Type-specific fields are stored in the compressed 'msg' JSON field.
// ClickHouse's columnar storage ensures queries only read the columns they need.
type Event struct {
	ch.CHModel `ch:"table:events,engine:ReplacingMergeTree(height)"`

	// Primary key (composite)
	Height    uint64 `ch:"height,pk" json:"height"`
	Address   string `ch:"address,pk" json:"address"`
	Reference string `ch:"reference,pk" json:"reference"`   // "begin_block", tx hash, or "end_block"
	EventType string `ch:"event_type,pk,lc" json:"event_type"` // LowCardinality for efficient filtering

	// Extracted queryable fields (nullable - NULL when not applicable)
	Amount       *uint64 `ch:"amount" json:"amount,omitempty"`
	SoldAmount   *uint64 `ch:"sold_amount" json:"sold_amount,omitempty"`
	BoughtAmount *uint64 `ch:"bought_amount" json:"bought_amount,omitempty"`
	LocalAmount  *uint64 `ch:"local_amount" json:"local_amount,omitempty"`
	RemoteAmount *uint64 `ch:"remote_amount" json:"remote_amount,omitempty"`
	Success      *bool   `ch:"success" json:"success,omitempty"`
	LocalOrigin  *bool   `ch:"local_origin" json:"local_origin,omitempty"`
	OrderID      *string `ch:"order_id" json:"order_id,omitempty"`

	// Full message (compressed)
	Msg string `ch:"msg,codec:ZSTD(3)" json:"msg"`

	// Time-range query field
	HeightTime time.Time `ch:"height_time" json:"height_time"`
}

// InitEvents creates the events table with ZSTD compression.
// Uses ReplacingMergeTree with height as the deduplication key.
func InitEvents(ctx context.Context, db *ch.DB) error {
	// Use the model's CHModel tag for table creation
	// The codec specifications in the struct tags will be applied
	_, err := db.NewCreateTable().
		Model((*Event)(nil)).
		IfNotExists().
		Exec(ctx)
	return err
}

// InsertEventsStaging inserts events to the staging table.
// Staging tables are used for new data before promotion to production.
// This follows the two-phase commit pattern for data consistency.
func InsertEventsStaging(ctx context.Context, db *ch.DB, tableName string, events []*Event) error {
	if len(events) == 0 {
		return nil
	}
	_, err := db.NewInsert().
		Model(&events).
		Table(tableName).
		Exec(ctx)
	return err
}
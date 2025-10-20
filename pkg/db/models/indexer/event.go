package indexer

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
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
	// Primary key (composite)
	Height    uint64 `ch:"height" json:"height"`
	ChainID   uint64 `ch:"chain_id" json:"chain_id"` // Which nested chain (committee) this event is for
	Address   string `ch:"address" json:"address"`
	Reference string `ch:"reference" json:"reference"`   // "begin_block", tx hash, or "end_block"
	EventType string `ch:"event_type" json:"event_type"` // LowCardinality for efficient filtering

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
	Msg string `ch:"msg" json:"msg"`

	// Time-range query field
	HeightTime time.Time `ch:"height_time" json:"height_time"`
}

// InitEvents creates the events table with ZSTD compression.
// Uses ReplacingMergeTree with height as the deduplication key.
func InitEvents(ctx context.Context, db driver.Conn) error {
	query := `
		CREATE TABLE IF NOT EXISTS events (
			height UInt64,
			chain_id UInt64,
			address String,
			reference String,
			event_type LowCardinality(String),
			amount Nullable(UInt64),
			sold_amount Nullable(UInt64),
			bought_amount Nullable(UInt64),
			local_amount Nullable(UInt64),
			remote_amount Nullable(UInt64),
			success Nullable(Bool),
			local_origin Nullable(Bool),
			order_id Nullable(String),
			msg String CODEC(ZSTD(3)),
			height_time DateTime64(6)
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (height, chain_id, address, reference, event_type)
	`
	return db.Exec(ctx, query)
}

// InsertEventsStaging inserts events to the staging table.
// Staging tables are used for new data before promotion to production.
// This follows the two-phase commit pattern for data consistency.
func InsertEventsStaging(ctx context.Context, db driver.Conn, tableName string, events []*Event) error {
	if len(events) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO %s (height, chain_id, address, reference, event_type, amount, sold_amount, bought_amount, local_amount, remote_amount, success, local_origin, order_id, msg, height_time) VALUES`, tableName)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, event := range events {
		err = batch.Append(
			event.Height,
			event.ChainID,
			event.Address,
			event.Reference,
			event.EventType,
			event.Amount,
			event.SoldAmount,
			event.BoughtAmount,
			event.LocalAmount,
			event.RemoteAmount,
			event.Success,
			event.LocalOrigin,
			event.OrderID,
			event.Msg,
			event.HeightTime,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

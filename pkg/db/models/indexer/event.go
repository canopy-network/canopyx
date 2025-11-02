package indexer

import (
	"time"
)

const EventsProductionTableName = "events"
const EventsStagingTableName = "events_staging"

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

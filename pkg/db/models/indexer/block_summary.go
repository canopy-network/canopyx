package indexer

import (
	"context"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// BlockSummary stores aggregated entity counts for each indexed block.
// This table is separate from blocks to keep the blocks table immutable
// and avoid updating it with summary data after entity indexing completes.
type BlockSummary struct {
	Height         uint64            `ch:"height" json:"height"`
	HeightTime     time.Time         `ch:"height_time" json:"height_time"` // Block timestamp for time-range queries
	NumTxs         uint32            `ch:"num_txs" json:"num_txs"`
	TxCountsByType map[string]uint32 `ch:"tx_counts_by_type" json:"tx_counts_by_type"` // Transaction counts by message type
	// TODO: Add more entity counts here as we index more entities
	// NumEvents uint32 `ch:"num_events" json:"num_events"`
	// NumLogs   uint32 `ch:"num_logs" json:"num_logs"`
}

// InitBlockSummaries initializes the block_summaries table.
func InitBlockSummaries(ctx context.Context, db driver.Conn) error {
	query := `
		CREATE TABLE IF NOT EXISTS block_summaries (
			height UInt64,
			height_time DateTime64(6),
			num_txs UInt32 DEFAULT 0,
			tx_counts_by_type Map(String, UInt32)
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (height)
	`
	return db.Exec(ctx, query)
}

// GetBlockSummary returns the latest (deduped) summary for the given height.
func GetBlockSummary(ctx context.Context, db driver.Conn, height uint64) (*BlockSummary, error) {
	var bs BlockSummary

	query := `
		SELECT height, height_time, num_txs, tx_counts_by_type
		FROM block_summaries FINAL
		WHERE height = ?
		LIMIT 1
	`

	err := db.QueryRow(ctx, query, height).Scan(
		&bs.Height,
		&bs.HeightTime,
		&bs.NumTxs,
		&bs.TxCountsByType,
	)

	return &bs, err
}

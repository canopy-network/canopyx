package indexer

import (
	"context"
	"time"

	"github.com/uptrace/go-clickhouse/ch"
)

// BlockSummary stores aggregated entity counts for each indexed block.
// This table is separate from blocks to keep the blocks table immutable
// and avoid updating it with summary data after entity indexing completes.
type BlockSummary struct {
	ch.CHModel `ch:"table:block_summaries,engine:ReplacingMergeTree(height)"`

	Height         uint64            `ch:"height,pk" json:"height"`
	HeightTime     time.Time         `ch:"height_time,type:DateTime64(6)" json:"height_time"` // Block timestamp for time-range queries
	NumTxs         uint32            `ch:"num_txs,default:0" json:"num_txs"`
	TxCountsByType map[string]uint32 `ch:"tx_counts_by_type,type:Map(LowCardinality(String),UInt32)" json:"tx_counts_by_type"` // Transaction counts by message type
	// TODO: Add more entity counts here as we index more entities
	// NumEvents uint32 `ch:"num_events,default:0" json:"num_events"`
	// NumLogs   uint32 `ch:"num_logs,default:0" json:"num_logs"`
}

// InitBlockSummaries initializes the block_summaries table.
func InitBlockSummaries(ctx context.Context, db *ch.DB) error {
	_, err := db.NewCreateTable().
		Model((*BlockSummary)(nil)).
		IfNotExists().
		Exec(ctx)
	return err
}

// GetBlockSummary returns the latest (deduped) summary for the given height.
func GetBlockSummary(ctx context.Context, db *ch.DB, height uint64) (*BlockSummary, error) {
	var bs BlockSummary

	err := db.NewSelect().
		Model(&bs).
		Where("height = ?", height).
		Final().
		Limit(1).
		Scan(ctx)

	return &bs, err
}

package indexer

import (
	"context"

	"github.com/uptrace/go-clickhouse/ch"
)

// BlockSummary stores aggregated entity counts for each indexed block.
// This table is separate from blocks to keep the blocks table immutable
// and avoid updating it with summary data after entity indexing completes.
type BlockSummary struct {
	ch.CHModel `ch:"table:block_summaries"`

	Height uint64 `ch:"height,pk" json:"height"`
	NumTxs uint32 `ch:"num_txs,default:0" json:"num_txs"`
	// TODO: Add more entity counts here as we index more entities
	// NumEvents uint32 `ch:"num_events,default:0" json:"num_events"`
	// NumLogs   uint32 `ch:"num_logs,default:0" json:"num_logs"`
}

// InitBlockSummaries initializes the block_summaries table.
func InitBlockSummaries(ctx context.Context, db *ch.DB) error {
	_, err := db.NewCreateTable().
		Model((*BlockSummary)(nil)).
		IfNotExists().
		Engine("ReplacingMergeTree").
		Order("height").
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

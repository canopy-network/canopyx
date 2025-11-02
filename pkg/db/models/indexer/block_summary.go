package indexer

import (
	"time"
)

const BlockSummariesProductionTableName = "block_summaries"
const BlockSummariesStagingTableName = "block_summaries_staging"

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

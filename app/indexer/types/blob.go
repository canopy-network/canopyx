package types

import (
	"time"

	"github.com/canopy-network/canopy/fsm"
)

// ActivityFetchBlobOutput contains indexer blobs fetched from RPC and execution duration.
type ActivityFetchBlobOutput struct {
	Blobs      *fsm.IndexerBlobs `json:"blobs"`
	DurationMs float64           `json:"durationMs"`
}

// ActivityIndexBlockFromBlobInput provides the data needed to index from a blob.
type ActivityIndexBlockFromBlobInput struct {
	Height            uint64  `json:"height"`
	PrepareDurationMs float64 `json:"prepareDurationMs"`
}

type ActivityIndexBlockFromBlobOutput struct {
	Height         uint64             `json:"height"`
	IndexingTimeMs float64            `json:"indexingTimeMs"` // Total workflow execution time in milliseconds
	IndexingDetail map[string]float64 `json:"indexingDetail"` // JSON string with a breakdown of individual activity timings

	// Block info for RecordIndexed (avoid re-querying ClickHouse)
	BlockHash            string    `json:"blockHash"`
	BlockTime            time.Time `json:"blockTime"`
	BlockProposerAddress string    `json:"blockProposerAddress"`
}

package types

import (
	indexer "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

const (
	// LiveBlockThreshold defines the number of blocks from the chain head
	// that are considered "live" for queue routing purposes.
	// Blocks within this threshold are routed to the live queue for low-latency indexing.
	// Blocks beyond this threshold are routed to the historical queue for bulk processing.
	LiveBlockThreshold = 200 // Blocks within 200 of head are considered "live"
)

// IsLiveBlock determines if a block should be routed to the live queue.
// Blocks within LiveBlockThreshold of the chain head are considered live.
// This function is used to route blocks to either the live queue (for low-latency indexing)
// or the historical queue (for bulk processing).
func IsLiveBlock(latest, height uint64) bool {
	if height > latest {
		return true // Future blocks are always considered live
	}
	return latest-height <= LiveBlockThreshold
}

type ChainIdInput struct {
	ChainID string `json:"chainId"`
}

// BlockSummaries - Any summary of a block that is needed for indexing.
type BlockSummaries struct {
	NumTxs uint32 `json:"numTxs"`
}

type IndexBlockInput struct {
	ChainID        string          `json:"chainId"`
	Height         uint64          `json:"height"`
	BlockSummaries *BlockSummaries `json:"blockSummaries"`
	PriorityKey    int             `json:"priorityKey"`
	Reindex        bool            `json:"reindex"`
}

// SchedulerInput is the input for the SchedulerWorkflow
type SchedulerInput struct {
	ChainID        string   `json:"chainId"`
	StartHeight    uint64   `json:"startHeight"`
	EndHeight      uint64   `json:"endHeight"`
	LatestHeight   uint64   `json:"latestHeight"`
	ProcessedSoFar uint64   `json:"processedSoFar"` // For ContinueAsNew tracking
	PriorityRanges []uint64 `json:"priorityRanges"` // Priority bucket boundaries for continuation
}

// Activity output types with execution timing

// PrepareIndexBlockOutput contains the result of block preparation along with execution duration.
type PrepareIndexBlockOutput struct {
	Skip       bool    `json:"skip"`       // True if the block should be skipped
	DurationMs float64 `json:"durationMs"` // Execution time in milliseconds
}

// FetchBlockOutput contains the fetched block data along with execution duration.
// This is used by the FetchBlockFromRPC local activity.
type FetchBlockOutput struct {
	Block      *indexer.Block `json:"block"`      // The fetched block
	DurationMs float64        `json:"durationMs"` // Execution time in milliseconds
}

// SaveBlockInput contains the parameters for saving a block to the database.
type SaveBlockInput struct {
	ChainID string         `json:"chainId"`
	Height  uint64         `json:"height"`
	Block   *indexer.Block `json:"block"` // The block to save
}

// IndexBlockOutput contains the indexed block height along with execution duration.
type IndexBlockOutput struct {
	Height     uint64  `json:"height"`     // The indexed block height
	DurationMs float64 `json:"durationMs"` // Execution time in milliseconds
}

// IndexTransactionsOutput contains the number of indexed transactions along with execution duration.
type IndexTransactionsOutput struct {
	NumTxs     uint32  `json:"numTxs"`     // Number of transactions indexed
	DurationMs float64 `json:"durationMs"` // Execution time in milliseconds
}

// SaveBlockSummaryInput contains the parameters for saving block summaries.
type SaveBlockSummaryInput struct {
	ChainID   string         `json:"chainId"`
	Height    uint64         `json:"height"`
	Summaries BlockSummaries `json:"summaries"`
}

// SaveBlockSummaryOutput contains the result of saving block summaries along with execution duration.
type SaveBlockSummaryOutput struct {
	DurationMs float64 `json:"durationMs"` // Execution time in milliseconds
}

// RecordIndexedInput contains the parameters for recording indexing progress with timing data.
type RecordIndexedInput struct {
	ChainID        string  `json:"chainId"`
	Height         uint64  `json:"height"`
	IndexingTimeMs float64 `json:"indexingTimeMs"` // Total activity execution time in milliseconds
	IndexingDetail string  `json:"indexingDetail"` // JSON string with breakdown of individual activity timings
}

// BatchScheduleInput contains the parameters for batch scheduling workflows.
type BatchScheduleInput struct {
	ChainID     string `json:"chainId"`
	StartHeight uint64 `json:"startHeight"`
	EndHeight   uint64 `json:"endHeight"`
	PriorityKey int    `json:"priorityKey"`
}

// BatchScheduleOutput contains the result of batch scheduling.
type BatchScheduleOutput struct {
	Scheduled  int     `json:"scheduled"`  // Number of workflows successfully scheduled
	Failed     int     `json:"failed"`     // Number of workflows that failed to schedule
	DurationMs float64 `json:"durationMs"` // Total execution time in milliseconds
}

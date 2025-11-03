package types

import (
	"time"

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
	ChainID uint64 `json:"chainId"`
}

// BlockSummaries - Any summary of a block that is needed for indexing.
type BlockSummaries struct {
	NumTxs            uint32            `json:"numTxs"`
	TxCountsByType    map[string]uint32 `json:"txCountsByType"` // Count per type: {"send": 5, "delegate": 2}
	NumEvents         uint32            `json:"numEvents"`
	EventCountsByType map[string]uint32 `json:"eventCountsByType"` // Count per type: {"reward": 100, "dex-swap": 5}
	NumPools          uint32            `json:"numPools"`          // Number of pools indexed
	NumOrders         uint32            `json:"numOrders"`         // Number of orders indexed
	NumPrices         uint32            `json:"numPrices"`         // Number of DEX prices indexed
}

type IndexBlockInput struct {
	ChainID        uint64          `json:"chainId"`
	Height         uint64          `json:"height"`
	BlockSummaries *BlockSummaries `json:"blockSummaries"`
	PriorityKey    int             `json:"priorityKey"`
	Reindex        bool            `json:"reindex"`
}

// SchedulerInput is the input for the SchedulerWorkflow
type SchedulerInput struct {
	ChainID        uint64   `json:"chainId"`
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
	ChainID uint64         `json:"chainId"`
	Height  uint64         `json:"height"`
	Block   *indexer.Block `json:"block"` // The block to save
}

// IndexBlockOutput contains the indexed block height along with execution duration.
type IndexBlockOutput struct {
	Height     uint64  `json:"height"`     // The indexed block height
	DurationMs float64 `json:"durationMs"` // Execution time in milliseconds
}

// IndexTransactionsInput contains the parameters for indexing transactions.
type IndexTransactionsInput struct {
	ChainID   uint64    `json:"chainId"`
	Height    uint64    `json:"height"`
	BlockTime time.Time `json:"blockTime"` // Block timestamp for populating height_time
}

// IndexTransactionsOutput contains the number of indexed transactions along with execution duration.
type IndexTransactionsOutput struct {
	NumTxs         uint32            `json:"numTxs"`         // Number of transactions indexed
	TxCountsByType map[string]uint32 `json:"txCountsByType"` // Count per type: {"send": 5, "delegate": 2}
	DurationMs     float64           `json:"durationMs"`     // Execution time in milliseconds
}

// EnsureGenesisCachedInput contains the parameters for caching genesis state.
type EnsureGenesisCachedInput struct {
	ChainID uint64 `json:"chainId"`
}

// IndexAccountsInput contains the parameters for indexing accounts.
type IndexAccountsInput struct {
	ChainID   uint64    `json:"chainId"`
	Height    uint64    `json:"height"`
	BlockTime time.Time `json:"blockTime"` // Block timestamp for populating height_time
}

// IndexAccountsOutput contains the number of changed accounts (snapshots created) along with execution duration.
type IndexAccountsOutput struct {
	NumAccounts uint32  `json:"numAccounts"` // Number of changed accounts (snapshots created)
	DurationMs  float64 `json:"durationMs"`  // Execution time in milliseconds
}

// IndexEventsInput contains the parameters for indexing events.
type IndexEventsInput struct {
	ChainID   uint64    `json:"chainId"`
	Height    uint64    `json:"height"`
	BlockTime time.Time `json:"blockTime"` // Block timestamp for populating height_time
}

// IndexEventsOutput contains the number of indexed events along with execution duration.
type IndexEventsOutput struct {
	NumEvents         uint32            `json:"numEvents"`         // Number of events indexed
	EventCountsByType map[string]uint32 `json:"eventCountsByType"` // Count per type: {"reward": 100, "dex-swap": 5}
	DurationMs        float64           `json:"durationMs"`        // Execution time in milliseconds
}

// SaveBlockSummaryInput contains the parameters for saving block summaries.
type SaveBlockSummaryInput struct {
	ChainID   uint64         `json:"chainId"`
	Height    uint64         `json:"height"`
	BlockTime time.Time      `json:"blockTime"` // Block timestamp for time-range queries
	Summaries BlockSummaries `json:"summaries"`
}

// SaveBlockSummaryOutput contains the result of saving block summaries along with execution duration.
type SaveBlockSummaryOutput struct {
	DurationMs float64 `json:"durationMs"` // Execution time in milliseconds
}

// RecordIndexedInput contains the parameters for recording indexing progress with timing data.
type RecordIndexedInput struct {
	ChainID        uint64  `json:"chainId"`
	Height         uint64  `json:"height"`
	IndexingTimeMs float64 `json:"indexingTimeMs"` // Total activity execution time in milliseconds
	IndexingDetail string  `json:"indexingDetail"` // JSON string with breakdown of individual activity timings
}

// BatchScheduleInput contains the parameters for batch scheduling workflows.
type BatchScheduleInput struct {
	ChainID     uint64 `json:"chainId"`
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

// PromoteDataInput contains parameters for promoting entity data from staging to production
type PromoteDataInput struct {
	ChainID uint64 `json:"chainId"`
	Entity  string `json:"entity"` // Entity name: "blocks", "txs", "accounts", etc.
	Height  uint64 `json:"height"`
}

// PromoteDataOutput contains the result of data promotion
type PromoteDataOutput struct {
	Entity     string  `json:"entity"`
	Height     uint64  `json:"height"`
	DurationMs float64 `json:"durationMs"`
}

// CleanPromotedDataInput contains parameters for cleaning staging data
type CleanPromotedDataInput struct {
	ChainID uint64 `json:"chainId"`
	Entity  string `json:"entity"`
	Height  uint64 `json:"height"`
}

// CleanPromotedDataOutput contains the result of staging cleanup
type CleanPromotedDataOutput struct {
	Entity     string  `json:"entity"`
	Height     uint64  `json:"height"`
	DurationMs float64 `json:"durationMs"`
}

// IndexDexPricesInput contains the parameters for indexing DEX prices.
type IndexDexPricesInput struct {
	ChainID   uint64    `json:"chainId"`
	Height    uint64    `json:"height"`
	BlockTime time.Time `json:"blockTime"` // Block timestamp for populating height_time
}

// IndexDexPricesOutput contains the number of indexed DEX price records along with execution duration.
type IndexDexPricesOutput struct {
	NumPrices  uint32  `json:"numPrices"`  // Number of DEX price records indexed
	DurationMs float64 `json:"durationMs"` // Execution time in milliseconds
}

// IndexOrdersInput contains the parameters for indexing orders.
type IndexOrdersInput struct {
	ChainID   uint64    `json:"chainId"`
	Height    uint64    `json:"height"`
	BlockTime time.Time `json:"blockTime"` // Block timestamp for populating height_time
}

// IndexOrdersOutput contains the number of changed orders (snapshots created) along with execution duration.
type IndexOrdersOutput struct {
	NumOrders  uint32  `json:"numOrders"`  // Number of changed orders (snapshots created)
	DurationMs float64 `json:"durationMs"` // Execution time in milliseconds
}

// IndexPoolsInput contains the parameters for indexing pools.
type IndexPoolsInput struct {
	ChainID   uint64    `json:"chainId"`
	Height    uint64    `json:"height"`
	BlockTime time.Time `json:"blockTime"` // Block timestamp for populating height_time
}

// IndexPoolsOutput contains the number of indexed pools along with execution duration.
type IndexPoolsOutput struct {
	NumPools   uint32  `json:"numPools"`   // Number of pools indexed
	DurationMs float64 `json:"durationMs"` // Execution time in milliseconds
}

// IndexParamsInput contains the parameters for indexing chain parameters.
type IndexParamsInput struct {
	ChainID   uint64    `json:"chainId"`
	Height    uint64    `json:"height"`
	BlockTime time.Time `json:"blockTime"` // Block timestamp for populating height_time
}

// IndexParamsOutput contains the result of indexing params along with execution duration.
type IndexParamsOutput struct {
	ParamsChanged bool    `json:"paramsChanged"` // True if params changed from previous height
	DurationMs    float64 `json:"durationMs"`    // Execution time in milliseconds
}

// IndexCommitteesInput contains the parameters for indexing committee data.
type IndexCommitteesInput struct {
	ChainID   uint64    `json:"chainId"`
	Height    uint64    `json:"height"`
	BlockTime time.Time `json:"blockTime"` // Block timestamp for populating height_time
}

// IndexCommitteesOutput contains the result of indexing committees along with execution duration.
type IndexCommitteesOutput struct {
	NumCommittees uint32  `json:"numCommittees"` // Number of committees that changed
	DurationMs    float64 `json:"durationMs"`    // Execution time in milliseconds
}

// IndexPollInput contains the parameters for indexing governance poll data.
type IndexPollInput struct {
	ChainID   uint64    `json:"chainId"`
	Height    uint64    `json:"height"`
	BlockTime time.Time `json:"blockTime"` // Block timestamp for populating height_time
}

// IndexPollOutput contains the result of indexing poll data along with execution duration.
type IndexPollOutput struct {
	NumProposals uint32  `json:"numProposals"` // Number of proposals in the poll
	DurationMs   float64 `json:"durationMs"`   // Execution time in milliseconds
}

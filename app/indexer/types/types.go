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

// BlockSummaries - Comprehensive summary of all indexed entities for a block.
// This struct aggregates counts from all 16 indexed entity types (90+ fields total).
// Maps are used during workflow aggregation for dynamic type counting, then converted
// to individual fields when persisting to the database model.
type BlockSummaries struct {
	// ========== Transactions (24 fields) ==========
	NumTxs         uint32            `json:"numTxs"`
	TxCountsByType map[string]uint32 `json:"txCountsByType"` // Dynamic counts by message type

	// ========== Accounts (2 fields) ==========
	NumAccounts    uint32 `json:"numAccounts"`    // Number of accounts that changed
	NumAccountsNew uint32 `json:"numAccountsNew"` // Number of new accounts created

	// ========== Events (10 fields) ==========
	NumEvents         uint32            `json:"numEvents"`
	EventCountsByType map[string]uint32 `json:"eventCountsByType"` // Dynamic counts by event type

	// ========== Orders (6 fields) ==========
	NumOrders          uint32 `json:"numOrders"`          // Total number of orders
	NumOrdersNew       uint32 `json:"numOrdersNew"`       // Number of new orders
	NumOrdersOpen      uint32 `json:"numOrdersOpen"`      // Number of open orders
	NumOrdersFilled    uint32 `json:"numOrdersFilled"`    // Number of filled orders
	NumOrdersCancelled uint32 `json:"numOrdersCancelled"` // Number of cancelled orders
	NumOrdersExpired   uint32 `json:"numOrdersExpired"`   // Number of expired orders

	// ========== Pools (2 fields) ==========
	NumPools    uint32 `json:"numPools"`    // Total number of pools
	NumPoolsNew uint32 `json:"numPoolsNew"` // Number of new pools created

	// ========== DexPrices (1 field) ==========
	NumDexPrices uint32 `json:"numDexPrices"` // Number of DEX price records

	// ========== DexOrders (6 fields) ==========
	NumDexOrders         uint32 `json:"numDexOrders"`         // Total number of DEX orders
	NumDexOrdersFuture   uint32 `json:"numDexOrdersFuture"`   // Number of future DEX orders
	NumDexOrdersLocked   uint32 `json:"numDexOrdersLocked"`   // Number of locked DEX orders
	NumDexOrdersComplete uint32 `json:"numDexOrdersComplete"` // Number of complete DEX orders
	NumDexOrdersSuccess  uint32 `json:"numDexOrdersSuccess"`  // Number of successful DEX orders
	NumDexOrdersFailed   uint32 `json:"numDexOrdersFailed"`   // Number of failed DEX orders

	// ========== DexDeposits (3 fields) ==========
	NumDexDeposits         uint32 `json:"numDexDeposits"`         // Total number of DEX deposits
	NumDexDepositsPending  uint32 `json:"numDexDepositsPending"`  // Number of pending DEX deposits
	NumDexDepositsComplete uint32 `json:"numDexDepositsComplete"` // Number of complete DEX deposits

	// ========== DexWithdrawals (3 fields) ==========
	NumDexWithdrawals         uint32 `json:"numDexWithdrawals"`         // Total number of DEX withdrawals
	NumDexWithdrawalsPending  uint32 `json:"numDexWithdrawalsPending"`  // Number of pending DEX withdrawals
	NumDexWithdrawalsComplete uint32 `json:"numDexWithdrawalsComplete"` // Number of complete DEX withdrawals

	// ========== DexPoolPointsByHolder (2 fields) ==========
	NumDexPoolPointsHolders    uint32 `json:"numDexPoolPointsHolders"`    // Total number of pool point holders
	NumDexPoolPointsHoldersNew uint32 `json:"numDexPoolPointsHoldersNew"` // Number of new pool point holders

	// ========== Params (1 field) ==========
	ParamsChanged bool `json:"paramsChanged"` // Whether chain parameters changed at this height

	// ========== Validators (5 fields) ==========
	NumValidators          uint32 `json:"numValidators"`          // Total number of validators
	NumValidatorsNew       uint32 `json:"numValidatorsNew"`       // Number of new validators
	NumValidatorsActive    uint32 `json:"numValidatorsActive"`    // Number of active validators
	NumValidatorsPaused    uint32 `json:"numValidatorsPaused"`    // Number of paused validators
	NumValidatorsUnstaking uint32 `json:"numValidatorsUnstaking"` // Number of unstaking validators

	// ========== ValidatorSigningInfo (2 fields) ==========
	NumValidatorSigningInfo    uint32 `json:"numValidatorSigningInfo"`    // Total number of signing info records
	NumValidatorSigningInfoNew uint32 `json:"numValidatorSigningInfoNew"` // Number of new signing info records

	// ========== Committees (4 fields) ==========
	NumCommittees           uint32 `json:"numCommittees"`           // Total number of committees
	NumCommitteesNew        uint32 `json:"numCommitteesNew"`        // Number of new committees
	NumCommitteesSubsidized uint32 `json:"numCommitteesSubsidized"` // Number of subsidized committees
	NumCommitteesRetired    uint32 `json:"numCommitteesRetired"`    // Number of retired committees

	// ========== CommitteeValidators (1 field) ==========
	NumCommitteeValidators uint32 `json:"numCommitteeValidators"` // Number of committee-validator relationships

	// ========== PollSnapshots (1 field) ==========
	NumPollSnapshots uint32 `json:"numPollSnapshots"` // Number of poll snapshot records
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

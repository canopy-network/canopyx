package types

import "time"

// --- Workflow types

// WorkflowHeadScanInput extends ChainIdInput with continuation support for large ranges
type WorkflowHeadScanInput struct {
	ResumeFrom     uint64 `json:"resumeFrom"`     // If > 0, resume from this height (for ContinueAsNew)
	TargetLatest   uint64 `json:"targetLatest"`   // If > 0, use this as latest instead of querying (for ContinueAsNew)
	ProcessedSoFar uint64 `json:"processedSoFar"` // Track how many heights we've processed in this execution
}

// WorkflowSchedulerInput is the input for the SchedulerWorkflow
type WorkflowSchedulerInput struct {
	StartHeight    uint64 `json:"startHeight"`
	EndHeight      uint64 `json:"endHeight"`
	LatestHeight   uint64 `json:"latestHeight"`
	ProcessedSoFar uint64 `json:"processedSoFar"` // For ContinueAsNew tracking
}

// WorkflowReindexSchedulerInput is the input for the ReindexSchedulerWorkflow
type WorkflowReindexSchedulerInput struct {
	ChainID        uint64 `json:"chainID"`
	StartHeight    uint64 `json:"startHeight"`
	EndHeight      uint64 `json:"endHeight"`
	RequestedBy    string `json:"requestedBy"`
	RequestID      string `json:"requestID"`      // Unique ID for this reindex request (used for deterministic workflow IDs)
	ProcessedSoFar uint64 `json:"processedSoFar"` // For ContinueAsNew tracking
}

// WorkflowReindexSchedulerOutput is the output for the ReindexSchedulerWorkflow
type WorkflowReindexSchedulerOutput struct {
	TotalBlocks    uint64  `json:"totalBlocks"`
	TotalScheduled uint64  `json:"totalScheduled"`
	TotalFailed    uint64  `json:"totalFailed"`
	DurationMs     float64 `json:"durationMs"`
}

// --- Activity types

// ActivityPrepareIndexBlockOutput contains the result of block preparation along with execution duration.
type ActivityPrepareIndexBlockOutput struct {
	Skip       bool    `json:"skip"`       // True if the block should be skipped
	DurationMs float64 `json:"durationMs"` // Execution time in milliseconds
}

// ActivityIndexAtHeight represents the height of a block and its associated block time for indexing purposes.
type ActivityIndexAtHeight struct {
	Height    uint64    `json:"height"`
	BlockTime time.Time `json:"blockTime"` // Block timestamp for populating height_time
}

// ActivityRecordIndexedInput contains the parameters for recording indexing progress with timing data.
// Uses individual timing fields instead of JSON for ClickHouse column efficiency.
type ActivityRecordIndexedInput struct {
	Height         uint64  `json:"height"`
	IndexingTimeMs float64 `json:"indexingTimeMs"` // Total workflow execution time in milliseconds

	// Block info for Redis event (passed from IndexBlockFromBlob to avoid re-querying ClickHouse)
	BlockHash            string    `json:"blockHash"`
	BlockTime            time.Time `json:"blockTime"`
	BlockProposerAddress string    `json:"blockProposerAddress"`

	// Individual timing metrics (milliseconds) - columnar for ClickHouse efficiency
	TimingFetchBlockMs        float64 `json:"timing_fetch_block_ms"`
	TimingPrepareIndexMs      float64 `json:"timing_prepare_index_ms"`
	TimingIndexAccountsMs     float64 `json:"timing_index_accounts_ms"`
	TimingIndexCommitteesMs   float64 `json:"timing_index_committees_ms"`
	TimingIndexDexBatchMs     float64 `json:"timing_index_dex_batch_ms"`
	TimingIndexDexPricesMs    float64 `json:"timing_index_dex_prices_ms"`
	TimingIndexEventsMs       float64 `json:"timing_index_events_ms"`
	TimingIndexOrdersMs       float64 `json:"timing_index_orders_ms"`
	TimingIndexParamsMs       float64 `json:"timing_index_params_ms"`
	TimingIndexPoolsMs        float64 `json:"timing_index_pools_ms"`
	TimingIndexSupplyMs       float64 `json:"timing_index_supply_ms"`
	TimingIndexTransactionsMs float64 `json:"timing_index_transactions_ms"`
	TimingIndexValidatorsMs   float64 `json:"timing_index_validators_ms"`
	TimingSaveBlockMs         float64 `json:"timing_save_block_ms"`
	TimingSaveBlockSummaryMs  float64 `json:"timing_save_block_summary_ms"`
}

// ActivityBatchScheduleInput contains the parameters for batch scheduling workflows.
type ActivityBatchScheduleInput struct {
	StartHeight uint64 `json:"startHeight"`
	EndHeight   uint64 `json:"endHeight"`
	PriorityKey int    `json:"priorityKey"`
}

// ActivityBatchScheduleOutput contains the result of batch scheduling.
type ActivityBatchScheduleOutput struct {
	Scheduled  int     `json:"scheduled"`  // Number of workflows successfully scheduled
	Failed     int     `json:"failed"`     // Number of workflows that failed to schedule
	DurationMs float64 `json:"durationMs"` // Total execution time in milliseconds
}

// ActivityReindexBatchInput contains the parameters for reindex batch scheduling.
type ActivityReindexBatchInput struct {
	ChainID     uint64 `json:"chainID"`
	StartHeight uint64 `json:"startHeight"`
	EndHeight   uint64 `json:"endHeight"`
	Reindex     bool   `json:"reindex"`   // Mark as reindex operation
	RequestID   string `json:"requestID"` // Unique ID for this reindex request (for deterministic workflow IDs)
}

// ActivityReindexBatchOutput contains the result of reindex batch scheduling.
type ActivityReindexBatchOutput struct {
	Scheduled int `json:"scheduled"` // Number of workflows successfully scheduled
	Failed    int `json:"failed"`    // Number of workflows that failed to schedule
}

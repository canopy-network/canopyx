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
	StartHeight    uint64   `json:"startHeight"`
	EndHeight      uint64   `json:"endHeight"`
	LatestHeight   uint64   `json:"latestHeight"`
	ProcessedSoFar uint64   `json:"processedSoFar"` // For ContinueAsNew tracking
	PriorityRanges []uint64 `json:"priorityRanges"` // Priority bucket boundaries for continuation
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
type ActivityRecordIndexedInput struct {
	Height         uint64  `json:"height"`
	IndexingTimeMs float64 `json:"indexingTimeMs"` // Total activity execution time in milliseconds
	IndexingDetail string  `json:"indexingDetail"` // JSON string with breakdown of individual activity timings
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

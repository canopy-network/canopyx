package types

// --- Workflow types

// WorkflowCleanupStagingInput contains the parameters for cleanup staging workflow.
type WorkflowCleanupStagingInput struct {
	Height uint64 `json:"height"`
}

// --- Activity types

// ActivityPromoteDataInput contains parameters for promoting entity data from staging to production
type ActivityPromoteDataInput struct {
	Entity string `json:"entity"` // Entity name: "blocks", "txs", "accounts", etc.
	Height uint64 `json:"height"`
}

// ActivityPromoteDataOutput contains the result of data promotion
type ActivityPromoteDataOutput struct {
	Entity     string  `json:"entity"`
	Height     uint64  `json:"height"`
	DurationMs float64 `json:"durationMs"`
}

// ActivityCleanPromotedDataInput contains parameters for cleaning staging data
type ActivityCleanPromotedDataInput struct {
	Entity string `json:"entity"`
	Height uint64 `json:"height"`
}

// ActivityCleanPromotedDataOutput contains the result of staging cleanup
type ActivityCleanPromotedDataOutput struct {
	Entity     string  `json:"entity"`
	Height     uint64  `json:"height"`
	DurationMs float64 `json:"durationMs"`
}

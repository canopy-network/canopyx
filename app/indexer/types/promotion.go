package types

// --- Workflow types

// WorkflowCleanupStagingInput contains the parameters for the hourly cleanup staging workflow.
// This workflow runs periodically and cleans up staging data for heights that have been
// fully indexed (confirmed in index_progress).
type WorkflowCleanupStagingInput struct {
	// LookbackHours defines how far back to look for indexed heights to clean.
	// Default: 2 hours. Heights indexed within the last 2 hours are cleaned from staging.
	LookbackHours int `json:"lookback_hours"`
}

// WorkflowCleanupStagingOutput contains the result of the cleanup workflow.
type WorkflowCleanupStagingOutput struct {
	HeightsCleaned  int     `json:"heights_cleaned"`
	EntitiesCleaned int     `json:"entities_cleaned"`
	DurationMs      float64 `json:"duration_ms"`
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

// ActivityCleanAllPromotedDataInput contains parameters for cleaning all staging data in parallel
type ActivityCleanAllPromotedDataInput struct {
	Entities []string `json:"entities"` // List of entity names: ["blocks", "txs", "accounts", ...]
	Height   uint64   `json:"height"`
}

// ActivityCleanAllPromotedDataOutput contains the result of batch staging cleanup
type ActivityCleanAllPromotedDataOutput struct {
	EntityCount int     `json:"entity_count"`
	Height      uint64  `json:"height"`
	DurationMs  float64 `json:"durationMs"`
}

// --- Batch Cleanup Activity types (for hourly cleanup workflow)

// ActivityGetCleanableHeightsInput contains parameters for querying heights eligible for cleanup.
type ActivityGetCleanableHeightsInput struct {
	LookbackHours int `json:"lookback_hours"` // How far back to look (default: 2 hours)
}

// ActivityGetCleanableHeightsOutput contains heights that can be safely cleaned.
type ActivityGetCleanableHeightsOutput struct {
	Heights    []uint64 `json:"heights"`
	DurationMs float64  `json:"duration_ms"`
}

// ActivityCleanStagingBatchInput contains parameters for batch cleaning staging tables.
type ActivityCleanStagingBatchInput struct {
	Heights []uint64 `json:"heights"` // Heights to clean from all staging tables
}

// ActivityCleanStagingBatchOutput contains the result of batch staging cleanup.
type ActivityCleanStagingBatchOutput struct {
	HeightsCleaned  int     `json:"heights_cleaned"`
	EntitiesCleaned int     `json:"entities_cleaned"`
	DurationMs      float64 `json:"duration_ms"`
}

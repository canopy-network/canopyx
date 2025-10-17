package types

import "time"

// HealthInfo represents health status for a specific subsystem
type HealthInfo struct {
	Status    string    `json:"status"`     // unknown, healthy, degraded, warning, critical, unreachable, failed
	Message   string    `json:"message"`    // human-readable details
	UpdatedAt time.Time `json:"updated_at"` // when this health info was last updated
}

type ChainStatus struct {
	ChainID        string         `json:"chain_id"`
	ChainName      string         `json:"chain_name"`
	Image          string         `json:"image"`
	Notes          string         `json:"notes,omitempty"`
	Paused         bool           `json:"paused"`
	Deleted        bool           `json:"deleted"`
	MinReplicas    uint16         `json:"min_replicas"`
	MaxReplicas    uint16         `json:"max_replicas"`
	LastIndexed    uint64         `json:"last_indexed"`
	Head           uint64         `json:"head"`
	Queue          QueueStatus    `json:"queue"`         // Deprecated: use OpsQueue/IndexerQueue instead. Currently returns IndexerQueue for backward compatibility.
	OpsQueue       QueueStatus    `json:"ops_queue"`     // Ops queue metrics (headscan, gapscan, scheduler)
	IndexerQueue   QueueStatus    `json:"indexer_queue"` // Indexer queue metrics (block indexing)
	ReindexHistory []ReindexEntry `json:"reindex_history,omitempty"`

	// Gap/Missing Block Information - for historical indexing progress display
	MissingBlocksCount uint64 `json:"missing_blocks_count"` // Total number of missing blocks across all gaps
	GapRangesCount     int    `json:"gap_ranges_count"`     // Number of gap ranges (contiguous missing block segments)
	LargestGapStart    uint64 `json:"largest_gap_start"`    // Start height of the largest gap (0 if no gaps)
	LargestGapEnd      uint64 `json:"largest_gap_end"`      // End height of the largest gap (0 if no gaps)
	IsLiveSync         bool   `json:"is_live_sync"`         // True if last indexed is within 2 blocks of head

	// Dual-Queue Fields - Individual queue metrics for live and historical indexing
	LiveQueueDepth          int64   `json:"live_queue_depth"`           // Live queue pending tasks (workflows + activities)
	LiveQueueBacklogAge     float64 `json:"live_queue_backlog_age"`     // Live queue oldest task age in seconds
	HistoricalQueueDepth    int64   `json:"historical_queue_depth"`     // Historical queue pending tasks (workflows + activities)
	HistoricalQueueBacklogAge float64 `json:"historical_queue_backlog_age"` // Historical queue oldest task age in seconds

	// Health Status - populated from stored chain health fields
	Health           HealthInfo `json:"health"`            // overall health status
	RPCHealth        HealthInfo `json:"rpc_health"`        // RPC endpoint health (from headscan)
	QueueHealth      HealthInfo `json:"queue_health"`      // queue backlog health (from queue monitor)
	DeploymentHealth HealthInfo `json:"deployment_health"` // k8s deployment health (from controller)
}

type QueueStatus struct {
	PendingWorkflow int64   `json:"pending_workflow"`
	PendingActivity int64   `json:"pending_activity"`
	BacklogAgeSecs  float64 `json:"backlog_age_secs"`
	Pollers         int     `json:"pollers"`
}

type ReindexEntry struct {
	Height      uint64    `json:"height"`
	Status      string    `json:"status"`
	RequestedBy string    `json:"requested_by"`
	RequestedAt time.Time `json:"requested_at"`
	WorkflowID  string    `json:"workflow_id"`
	RunID       string    `json:"run_id"`
}

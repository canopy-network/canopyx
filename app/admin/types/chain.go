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
}

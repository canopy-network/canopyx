package types

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
	Queue          QueueStatus    `json:"queue"`
	ReindexHistory []ReindexEntry `json:"reindex_history,omitempty"`
}

type QueueStatus struct {
	PendingWorkflow int64   `json:"pending_workflow"`
	PendingActivity int64   `json:"pending_activity"`
	BacklogAgeSecs  float64 `json:"backlog_age_secs"`
	Pollers         int     `json:"pollers"`
}

type ReindexEntry struct {
	Height      uint64 `json:"height"`
	Status      string `json:"status"`
	RequestedBy string `json:"requested_by"`
	RequestedAt string `json:"requested_at"`
}

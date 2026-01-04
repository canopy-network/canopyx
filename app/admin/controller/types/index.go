package types

type ReindexRequest struct {
	Heights []uint64 `json:"heights"`
	From    *uint64  `json:"from"`
	To      *uint64  `json:"to"`
}

// BulkUpdateChainsRequest represents a request to update all chains with common fields.
// Only non-zero/non-empty values will be applied.
type BulkUpdateChainsRequest struct {
	Image                 string `json:"image"`                   // Docker image for all chains
	ReindexMinReplicas    uint16 `json:"reindex_min_replicas"`    // Minimum replicas for reindex workers
	ReindexMaxReplicas    uint16 `json:"reindex_max_replicas"`    // Maximum replicas for reindex workers
	ReindexScaleThreshold uint32 `json:"reindex_scale_threshold"` // Scale threshold for reindex workers
	MinReplicas           uint16 `json:"min_replicas"`            // Minimum replicas for indexer workers
	MaxReplicas           uint16 `json:"max_replicas"`            // Maximum replicas for indexer workers
}

// BulkUpdateChainsResponse represents the result of a bulk update operation.
type BulkUpdateChainsResponse struct {
	UpdatedCount int      `json:"updated_count"`
	ChainIDs     []uint64 `json:"chain_ids"`
}

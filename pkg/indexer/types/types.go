package types

type ChainIdInput struct {
	ChainID string `json:"chainId"`
}

// BlockSummaries - Any summary of a block that is needed for indexing.
type BlockSummaries struct {
	NumTxs uint32 `json:"numTxs"`
}

type IndexBlockInput struct {
	ChainID        string          `json:"chainId"`
	Height         uint64          `json:"height"`
	BlockSummaries *BlockSummaries `json:"blockSummaries"`
	PriorityKey    int             `json:"priorityKey"`
	Reindex        bool            `json:"reindex"`
}

// SchedulerInput is the input for the SchedulerWorkflow
type SchedulerInput struct {
	ChainID        string   `json:"chainId"`
	StartHeight    uint64   `json:"startHeight"`
	EndHeight      uint64   `json:"endHeight"`
	LatestHeight   uint64   `json:"latestHeight"`
	ProcessedSoFar uint64   `json:"processedSoFar"` // For ContinueAsNew tracking
	PriorityRanges []uint64 `json:"priorityRanges"` // Priority bucket boundaries for continuation
}

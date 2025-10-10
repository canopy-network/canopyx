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

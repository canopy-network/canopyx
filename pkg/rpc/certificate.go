package rpc

// QuorumCertificateResponse represents the response from /v1/query/cert-by-height endpoint.
// This endpoint returns the quorum certificate for a given height, which includes the chain ID.
type QuorumCertificateResponse struct {
	Header struct {
		Height          uint64 `json:"height"`
		CommitteeHeight uint64 `json:"committeeHeight"`
		Round           uint64 `json:"round"`
		Phase           string `json:"phase"`
		NetworkID       uint64 `json:"networkID"`
		ChainID         uint64 `json:"chainID"` // Note: JSON tag uses camelCase
	} `json:"header"`
	BlockHash   string `json:"blockHash"`
	ResultsHash string `json:"resultsHash"`
	Results     struct {
		RewardRecipients interface{} `json:"rewardRecipients"`
		SlashRecipients  interface{} `json:"slashRecipients"`
		Orders           interface{} `json:"orders"`
	} `json:"results"`
	ProposerKey string `json:"proposerKey"`
	Signature   struct {
		Signature string `json:"signature"`
		Bitmap    string `json:"bitmap"`
	} `json:"signature"`
}

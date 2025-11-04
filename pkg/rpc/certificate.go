package rpc

import (
	"context"
	"fmt"
	"net/http"
)

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

// CertByHeight queries the /v1/query/cert-by-height endpoint to retrieve the quorum certificate at a specific height.
// The quorum certificate contains consensus information including chain ID, network ID, proposer, and signatures.
//
// This is the base method for certificate operations. Other methods like FetchChainID use this internally.
//
// Parameters:
//   - height: The block height to query the certificate for (0 = genesis)
//
// Returns:
//   - *QuorumCertificateResponse: The quorum certificate data
//   - error: If the endpoint is unreachable or returns invalid data
//
// Example usage:
//
//	cert, err := client.CertByHeight(ctx, 0)  // Get genesis certificate
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Chain ID: %d\n", cert.Header.ChainID)
func (c *HTTPClient) CertByHeight(ctx context.Context, height uint64) (*QuorumCertificateResponse, error) {
	var cert QuorumCertificateResponse
	req := QueryByHeightRequest{Height: height}
	if err := c.doJSON(ctx, http.MethodPost, certByHeightPath, req, &cert); err != nil {
		return nil, fmt.Errorf("fetch certificate at height %d: %w", height, err)
	}
	return &cert, nil
}

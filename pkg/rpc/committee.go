package rpc

import (
	"context"
	"fmt"
	"net/http"
)

// CommitteesDataByHeight queries the /v1/query/committees-data endpoint to retrieve all committees data at a specific height.
// This returns data for all active committees in the network.
//
// Parameters:
//   - height: The block height to query committees data for (0 = latest)
//
// Returns:
//   - []*RpcCommitteeData: List of all committees data at the specified height
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) CommitteesDataByHeight(ctx context.Context, height uint64) ([]*RpcCommitteeData, error) {
	var committeesData RpcCommitteesData
	req := QueryByHeightRequest{Height: height}
	if err := c.doJSON(ctx, http.MethodPost, committeesDataPath, req, &committeesData); err != nil {
		return nil, fmt.Errorf("fetch committees data at height %d: %w", height, err)
	}
	if committeesData.List == nil {
		return []*RpcCommitteeData{}, nil
	}
	return committeesData.List, nil
}

// SubsidizedCommitteesByHeight queries the /v1/query/subsidized-committees endpoint to retrieve subsidized committee IDs at a specific height.
// Subsidized committees meet the stake percentage requirement and receive subsidies from the protocol.
//
// Parameters:
//   - height: The block height to query subsidized committees for (0 = latest)
//
// Returns:
//   - []uint64: List of subsidized committee IDs at the specified height
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) SubsidizedCommitteesByHeight(ctx context.Context, height uint64) ([]uint64, error) {
	var committees []uint64
	req := QueryByHeightRequest{Height: height}
	if err := c.doJSON(ctx, http.MethodPost, subsidizedCommitteesPath, req, &committees); err != nil {
		return nil, fmt.Errorf("fetch subsidized committees at height %d: %w", height, err)
	}
	if committees == nil {
		return []uint64{}, nil
	}
	return committees, nil
}

// RetiredCommitteesByHeight queries the /v1/query/retired-committees endpoint to retrieve retired committee IDs at a specific height.
// Retired committees have shut down and are marked as 'forever unsubsidized' on the root chain.
//
// Parameters:
//   - height: The block height to query retired committees for (0 = latest)
//
// Returns:
//   - []uint64: List of retired committee IDs at the specified height
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) RetiredCommitteesByHeight(ctx context.Context, height uint64) ([]uint64, error) {
	var committees []uint64
	req := QueryByHeightRequest{Height: height}
	if err := c.doJSON(ctx, http.MethodPost, retiredCommitteesPath, req, &committees); err != nil {
		return nil, fmt.Errorf("fetch retired committees at height %d: %w", height, err)
	}
	if committees == nil {
		return []uint64{}, nil
	}
	return committees, nil
}

// RpcPaymentPercent represents a payment recipient and their reward percentage.
// This matches the PaymentPercents proto structure.
type RpcPaymentPercent struct {
	Address string `json:"address"` // Hex-encoded recipient address
	Percent uint64 `json:"percent"` // Percentage of rewards (0-100)
}

// RpcCommitteeData represents committee data returned from the RPC.
// This matches the CommitteeData proto structure returned by /v1/query/committee-data and /v1/query/committees-data.
// A committee is a group of validators responsible for a specific chain.
type RpcCommitteeData struct {
	ChainID                uint64               `json:"chainID"`                // Unique identifier of the chain/committee
	LastRootHeightUpdated  uint64               `json:"lastRootHeightUpdated"`  // Canopy height of most recent Certificate Results tx
	LastChainHeightUpdated uint64               `json:"lastChainHeightUpdated"` // 3rd party chain height of most recent Certificate Results
	PaymentPercents        []*RpcPaymentPercent `json:"paymentPercents"`        // List of reward recipients and percentages
	NumberOfSamples        uint64               `json:"numberOfSamples"`        // Count of processed Certificate Result Transactions
}

// RpcCommitteesData represents a list of committee data returned from /v1/query/committees-data.
type RpcCommitteesData struct {
	List []*RpcCommitteeData `json:"list"` // List of all committees
}

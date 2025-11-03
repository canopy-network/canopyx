package rpc

import (
	"context"
	"fmt"
	"net/http"
)

const (
	committeeDataPath     = "/v1/query/committee-data"
	committeesDataPath    = "/v1/query/committees-data"
	subsidizedCommittees  = "/v1/query/subsidized-committees"
	retiredCommitteesPath = "/v1/query/retired-committees"
)

// CommitteeData queries the /v1/query/committee-data endpoint to retrieve committee data for a specific chain at a specific height.
// A committee is a group of validators responsible for a specific chain.
//
// Parameters:
//   - chainID: The unique identifier of the chain/committee
//   - height: The block height to query committee data for (0 = latest)
//
// Returns:
//   - *RpcCommitteeData: Committee data at the specified height
//   - error: If the endpoint is unreachable, returns invalid data, or committee doesn't exist
func (c *HTTPClient) CommitteeData(ctx context.Context, chainID uint64, height uint64) (*RpcCommitteeData, error) {
	var committeeData RpcCommitteeData
	req := map[string]any{
		"height":  height,
		"chainId": chainID,
	}
	if err := c.doJSON(ctx, http.MethodPost, committeeDataPath, req, &committeeData); err != nil {
		return nil, fmt.Errorf("fetch committee data for chain %d at height %d: %w", chainID, height, err)
	}
	return &committeeData, nil
}

// CommitteesData queries the /v1/query/committees-data endpoint to retrieve all committees data at a specific height.
// This returns data for all active committees in the network.
//
// Parameters:
//   - height: The block height to query committees data for (0 = latest)
//
// Returns:
//   - []*RpcCommitteeData: List of all committees data at the specified height
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) CommitteesData(ctx context.Context, height uint64) ([]*RpcCommitteeData, error) {
	var committeesData RpcCommitteesData
	req := map[string]any{"height": height}
	if err := c.doJSON(ctx, http.MethodPost, committeesDataPath, req, &committeesData); err != nil {
		return nil, fmt.Errorf("fetch committees data at height %d: %w", height, err)
	}
	if committeesData.List == nil {
		return []*RpcCommitteeData{}, nil
	}
	return committeesData.List, nil
}

// SubsidizedCommittees queries the /v1/query/subsidized-committees endpoint to retrieve subsidized committee IDs at a specific height.
// Subsidized committees meet the stake percentage requirement and receive subsidies from the protocol.
//
// Parameters:
//   - height: The block height to query subsidized committees for (0 = latest)
//
// Returns:
//   - []uint64: List of subsidized committee IDs at the specified height
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) SubsidizedCommittees(ctx context.Context, height uint64) ([]uint64, error) {
	var committees []uint64
	req := map[string]any{"height": height}
	if err := c.doJSON(ctx, http.MethodPost, subsidizedCommittees, req, &committees); err != nil {
		return nil, fmt.Errorf("fetch subsidized committees at height %d: %w", height, err)
	}
	if committees == nil {
		return []uint64{}, nil
	}
	return committees, nil
}

// RetiredCommittees queries the /v1/query/retired-committees endpoint to retrieve retired committee IDs at a specific height.
// Retired committees have shut down and are marked as 'forever unsubsidized' on the root chain.
//
// Parameters:
//   - height: The block height to query retired committees for (0 = latest)
//
// Returns:
//   - []uint64: List of retired committee IDs at the specified height
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) RetiredCommittees(ctx context.Context, height uint64) ([]uint64, error) {
	var committees []uint64
	req := map[string]any{"height": height}
	if err := c.doJSON(ctx, http.MethodPost, retiredCommitteesPath, req, &committees); err != nil {
		return nil, fmt.Errorf("fetch retired committees at height %d: %w", height, err)
	}
	if committees == nil {
		return []uint64{}, nil
	}
	return committees, nil
}

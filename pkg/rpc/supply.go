package rpc

import (
	"context"
	"fmt"
	"net/http"
)

// RpcSupply represents token supply information returned from /v1/query/supply endpoint.
// This matches the Supply proto structure returned by Canopy.
type RpcSupply struct {
	Total         uint64 `json:"total"`         // Total tokens existing in the system (minted - burned)
	Staked        uint64 `json:"staked"`        // Total locked tokens in the protocol (includes delegated)
	DelegatedOnly uint64 `json:"delegatedOnly"` // Total locked tokens that are delegated only
	// Note: committeeStaked and committeeDelegatedOnly arrays are omitted
	// as this data is derivable from validator/committee tables
}

// SupplyByHeight queries the /v1/query/supply endpoint to retrieve token supply metrics at a specific height.
//
// Parameters:
//   - height: The block height to query supply for (0 = latest)
//
// Returns:
//   - *RpcSupply: Supply metrics at the specified height
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) SupplyByHeight(ctx context.Context, height uint64) (*RpcSupply, error) {
	var supply RpcSupply
	req := QueryByHeightRequest{Height: height}
	if err := c.doJSON(ctx, http.MethodPost, supplyPath, req, &supply); err != nil {
		return nil, fmt.Errorf("fetch supply at height %d: %w", height, err)
	}
	return &supply, nil
}

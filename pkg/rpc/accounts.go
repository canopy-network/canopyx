package rpc

import (
	"context"

	"github.com/canopy-network/canopy/fsm"
)

// AccountsByHeight fetches ALL accounts at a specific height using pagination.
// This method handles pagination automatically and returns the complete set of accounts.
// JSON from RPC is unmarshaled directly into fsm.Account (protobuf types support JSON).
//
// The Canopy RPC endpoint /v1/query/accounts supports:
//   - height: uint64 - The block height to query (required)
//   - pageNumber: int - Page number (1-indexed, default: 1)
//   - perPage: int - Results per page (default: 1000, max: 1000)
//
// This function fetches all pages in parallel for performance.
func (c *HTTPClient) AccountsByHeight(ctx context.Context, height uint64) ([]*fsm.Account, error) {
	// Build request payload for the first page
	accounts, rpcErr := ListPaged[*fsm.Account](ctx, c, accountsByHeightPath, NewQueryByHeightRequest(height))

	if rpcErr != nil {
		return nil, rpcErr
	}

	return accounts, nil
}

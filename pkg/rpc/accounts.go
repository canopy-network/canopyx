package rpc

import (
	"context"
)

// Account represents an account returned from Canopy's RPC endpoint.
// This maps to the account structure returned by /v1/query/accounts.
type Account struct {
	Address string `json:"address"` // Hex bytes from RPC
	Amount  uint64 `json:"amount"`  // Account balance in uCNPY
}

// AccountsByHeight fetches ALL accounts at a specific height using pagination.
// This method handles pagination automatically and returns the complete set of accounts.
//
// The Canopy RPC endpoint /v1/query/accounts supports:
//   - height: uint64 - The block height to query (required)
//   - pageNumber: int - Page number (1-indexed, default: 1)
//   - perPage: int - Results per page (default: 1000, max: 1000)
//
// This function fetches all pages in parallel for performance.
func (c *HTTPClient) AccountsByHeight(ctx context.Context, height uint64) ([]*Account, error) {
	// Build request payload for the first page
	accounts, rpcErr := ListPaged[*Account](ctx, c, accountsByHeightPath, NewQueryByHeightRequest(height))

	if rpcErr != nil {
		return nil, rpcErr
	}

	return accounts, nil
}

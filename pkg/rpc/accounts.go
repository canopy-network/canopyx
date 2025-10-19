package rpc

import (
	"context"
	"fmt"
	"net/http"
)

const (
	accountsByHeightPath = "/v1/query/accounts"
)

// RpcAccount represents an account returned from Canopy's RPC endpoint.
// This maps to the account structure returned by /v1/query/accounts.
type RpcAccount struct {
	Address string `json:"address"` // Hex bytes from RPC
	Amount  uint64 `json:"amount"`  // Account balance in uCNPY
}

// accountsResponse wraps the paginated response from the accounts endpoint.
type accountsResponse struct {
	Results    []*RpcAccount `json:"results"`
	PageNumber int           `json:"pageNumber"`
	PerPage    int           `json:"perPage"`
	TotalPages int           `json:"totalPages"`
	TotalCount int           `json:"totalCount"`
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
func (c *HTTPClient) AccountsByHeight(ctx context.Context, height uint64) ([]*RpcAccount, error) {
	// Build request payload for first page
	args := map[string]any{
		"height":     height,
		"pageNumber": 1,
		"perPage":    1000, // Max page size
	}

	// Fetch first page to determine total pages
	var firstPage accountsResponse
	if err := c.doJSON(ctx, http.MethodPost, accountsByHeightPath, args, &firstPage); err != nil {
		return nil, fmt.Errorf("fetch accounts page 1 at height %d: %w", height, err)
	}

	// If only one page, return immediately
	if firstPage.TotalPages <= 1 {
		return firstPage.Results, nil
	}

	// Pre-allocate slice for all accounts
	allAccounts := make([]*RpcAccount, 0, firstPage.TotalCount)
	allAccounts = append(allAccounts, firstPage.Results...)

	// Fetch remaining pages in parallel
	type pageResult struct {
		accounts []*RpcAccount
		err      error
	}
	resultsCh := make(chan pageResult, firstPage.TotalPages-1)

	for page := 2; page <= firstPage.TotalPages; page++ {
		go func(pageNum int) {
			pageArgs := map[string]any{
				"height":     height,
				"pageNumber": pageNum,
				"perPage":    1000,
			}

			var pageResp accountsResponse
			if err := c.doJSON(ctx, http.MethodPost, accountsByHeightPath, pageArgs, &pageResp); err != nil {
				resultsCh <- pageResult{nil, fmt.Errorf("fetch accounts page %d at height %d: %w", pageNum, height, err)}
				return
			}

			resultsCh <- pageResult{pageResp.Results, nil}
		}(page)
	}

	// Collect results from parallel fetches
	for i := 0; i < firstPage.TotalPages-1; i++ {
		result := <-resultsCh
		if result.err != nil {
			return nil, result.err
		}
		allAccounts = append(allAccounts, result.accounts...)
	}

	return allAccounts, nil
}

// GenesisState represents the full genesis state returned by the RPC endpoint.
// This contains all initial blockchain state including accounts, validators, pools, and parameters.
type GenesisState struct {
	Time     uint64        `json:"time"`     // Genesis timestamp
	Accounts []*RpcAccount `json:"accounts"` // Initial account balances
	// Future fields for complete genesis state
	Validators []interface{} `json:"validators"` // Initial validators (not used yet)
	Pools      []interface{} `json:"pools"`      // Initial pools (not used yet)
	Params     interface{}   `json:"params"`     // Chain parameters (not used yet)
}

// GetGenesisState fetches the full genesis state at height 0.
// This is used by the accounts indexer to cache genesis for height 1 comparisons.
//
// The Canopy RPC endpoint /v1/query/state returns:
//   - time: genesis timestamp
//   - accounts: array of account objects with address and amount
//   - validators: array of initial validators
//   - pools: array of initial pools
//   - params: chain parameters
//
// Height parameter should always be 0 for genesis state.
func (c *HTTPClient) GetGenesisState(ctx context.Context, height uint64) (*GenesisState, error) {
	args := map[string]any{
		"height": height,
	}

	var genesis GenesisState
	if err := c.doJSON(ctx, http.MethodPost, "/v1/query/state", args, &genesis); err != nil {
		return nil, fmt.Errorf("fetch genesis state at height %d: %w", height, err)
	}

	return &genesis, nil
}
package rpc

import (
	"context"
	"fmt"
)

const (
	validatorsPath = "/v1/query/validators"
	nonSignersPath = "/v1/query/non-signers"
)

// Validators returns all validators at a specific block height using pagination.
// The RPC endpoint returns validators in pages, this method fetches all pages.
//
// The Canopy RPC endpoint /v1/query/validators supports:
//   - height: uint64 - Block height to query (0 = latest)
//   - pageNumber: int - Page number (1-indexed, default: 1)
//   - perPage: int - Results per page (default: 1000, max: 1000)
//
// Parameters:
//   - height: The block height to query validators for (0 = latest)
//
// Returns:
//   - []*RpcValidator: All validators at the specified height
//   - error: If the endpoint is unreachable or returns invalid data
//
// This function fetches all pages in parallel for performance.
func (c *HTTPClient) Validators(ctx context.Context, height uint64) ([]*RpcValidator, error) {
	req := NewQueryByHeightRequest(height)
	validators, err := ListPaged[*RpcValidator](ctx, c, validatorsPath, req)
	if err != nil {
		return nil, fmt.Errorf("fetch validators at height %d: %w", height, err)
	}
	return validators, nil
}

// NonSigners returns all non-signers (validators with missed blocks) at a specific block height.
// The RPC endpoint returns non-signer information for validators that have missed blocks
// within the non-sign tracking window.
//
// The Canopy RPC endpoint /v1/query/non-signers supports:
//   - height: uint64 - Block height to query (0 = latest)
//   - pageNumber: int - Page number (1-indexed, default: 1)
//   - perPage: int - Results per page (default: 1000, max: 1000)
//
// Parameters:
//   - height: The block height to query non-signers for (0 = latest)
//
// Returns:
//   - []*RpcNonSigner: All non-signers at the specified height
//   - error: Empty list if the endpoint doesn't exist or returns an error
//
// This function fetches all pages in parallel for performance.
// Note: Returns an empty list (not an error) if the endpoint is not available,
// for backward compatibility with older Canopy versions.
func (c *HTTPClient) NonSigners(ctx context.Context, height uint64) ([]*RpcNonSigner, error) {
	req := NewQueryByHeightRequest(height)
	nonSigners, err := ListPaged[*RpcNonSigner](ctx, c, nonSignersPath, req)
	if err != nil {
		// If the endpoint doesn't exist or returns an error, return empty list
		// This handles cases where the RPC doesn't support this endpoint yet
		return []*RpcNonSigner{}, nil
	}
	return nonSigners, nil
}

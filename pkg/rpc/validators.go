package rpc

import (
	"context"
	"fmt"
	"net/http"

	"github.com/canopy-network/canopy/fsm"
	"github.com/canopy-network/canopy/lib"
)

// Path constants moved to paths.go for centralized management

// ValidatorsByHeight returns all validators at a specific block height using pagination.
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
//   - []*fsm.Validator: All validators at the specified height (from canopy/fsm)
//   - error: If the endpoint is unreachable or returns invalid data
//
// This function fetches all pages in parallel for performance.
func (c *HTTPClient) ValidatorsByHeight(ctx context.Context, height uint64) ([]*fsm.Validator, error) {
	req := NewQueryByHeightRequest(height)
	validators, err := ListPaged[*fsm.Validator](ctx, c, validatorsPath, req)
	if err != nil {
		return nil, fmt.Errorf("fetch validators at height %d: %w", height, err)
	}
	return validators, nil
}

// NonSignersByHeight returns all non-signers (validators with missed blocks) at a specific block height.
// The RPC endpoint returns non-signer information for validators that have missed blocks
// within the non-sign tracking window.
//
// The Canopy RPC endpoint /v1/query/non-signers:
//   - height: uint64 - Block height to query (0 = latest)
//   - Returns: Plain array of NonSigner objects (NOT paginated)
//
// Parameters:
//   - height: The block height to query non-signers for (0 = latest)
//
// Returns:
//   - []*fsm.NonSigner: All non-signers at the specified height (from canopy/fsm)
//   - error: If the endpoint is unreachable or returns invalid data
//
// IMPORTANT: This method fails loudly on any error to prevent silent data loss
// and expensive reindexing. Ensure RPC endpoints are properly configured.
func (c *HTTPClient) NonSignersByHeight(ctx context.Context, height uint64) ([]*fsm.NonSigner, error) {
	req := NewQueryByHeightRequest(height)
	var nonSigners []*fsm.NonSigner
	if err := c.doJSON(ctx, http.MethodPost, nonSignersPath, req, &nonSigners); err != nil {
		return nil, fmt.Errorf("fetch non-signers at height %d: %w", height, err)
	}
	return nonSigners, nil
}

// NOTE: RpcValidator has been removed.
// Use fsm.Validator from github.com/canopy-network/canopy/fsm instead.
// This type is defined in the Canopy protocol and has proper JSON marshaling support.

// NOTE: RpcNonSigner has been removed.
// Use fsm.NonSigner from github.com/canopy-network/canopy/fsm instead.
// This type is defined in the Canopy protocol and has proper JSON marshaling support.

// DoubleSignersByHeight returns all double-signers (validators with Byzantine behavior) at a specific block height.
// The RPC endpoint returns double-signer information for validators that have signed multiple conflicting blocks
// at the same height (Byzantine fault).
//
// The Canopy RPC endpoint /v1/query/double-signers:
//   - height: uint64 - Block height to query (0 = latest)
//   - Returns: Plain array of DoubleSigner objects (NOT paginated)
//
// Parameters:
//   - height: The block height to query double-signers for (0 = latest)
//
// Returns:
//   - []*lib.DoubleSigner: All double-signers at the specified height (from canopy/lib)
//   - error: If the endpoint is unreachable or returns invalid data
//
// IMPORTANT: This method fails loudly on any error to prevent silent data loss
// and expensive reindexing. Ensure RPC endpoints are properly configured.
func (c *HTTPClient) DoubleSignersByHeight(ctx context.Context, height uint64) ([]*lib.DoubleSigner, error) {
	req := NewQueryByHeightRequest(height)
	var doubleSigners []*lib.DoubleSigner
	if err := c.doJSON(ctx, http.MethodPost, doubleSignersPath, req, &doubleSigners); err != nil {
		return nil, fmt.Errorf("fetch double-signers at height %d: %w", height, err)
	}
	return doubleSigners, nil
}

// NOTE: RpcDoubleSigner has been removed.
// Use lib.DoubleSigner from github.com/canopy-network/canopy/lib instead.
// This type is defined in the Canopy protocol and has proper JSON marshaling support.

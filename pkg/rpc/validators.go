package rpc

import (
	"context"
	"fmt"
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
//   - []*RpcValidator: All validators at the specified height
//   - error: If the endpoint is unreachable or returns invalid data
//
// This function fetches all pages in parallel for performance.
func (c *HTTPClient) ValidatorsByHeight(ctx context.Context, height uint64) ([]*RpcValidator, error) {
	req := NewQueryByHeightRequest(height)
	validators, err := ListPaged[*RpcValidator](ctx, c, validatorsPath, req)
	if err != nil {
		return nil, fmt.Errorf("fetch validators at height %d: %w", height, err)
	}
	return validators, nil
}

// NonSignersByHeight returns all non-signers (validators with missed blocks) at a specific block height.
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
func (c *HTTPClient) NonSignersByHeight(ctx context.Context, height uint64) ([]*RpcNonSigner, error) {
	req := NewQueryByHeightRequest(height)
	nonSigners, err := ListPaged[*RpcNonSigner](ctx, c, nonSignersPath, req)
	if err != nil {
		// If the endpoint doesn't exist or returns an error, return empty list
		// This handles cases where the RPC doesn't support this endpoint yet
		return []*RpcNonSigner{}, nil
	}
	return nonSigners, nil
}

// RpcValidator represents a validator returned from the RPC.
// This matches the Validator protobuf structure from Canopy (fsm/validator.pb.go:45-76).
// The structure contains exactly 10 fields that map directly to Canopy's validator state.
type RpcValidator struct {
	Address         string   `json:"address"`              // Hex-encoded validator address
	PublicKey       string   `json:"publicKey"`            // Hex-encoded Ed25519 public key
	NetAddress      string   `json:"netAddress"`           // P2P network address (host:port)
	StakedAmount    uint64   `json:"stakedAmount"`         // Amount staked by validator in uCNPY
	Committees      []uint64 `json:"committees,omitempty"` // Committee IDs validator is assigned to
	MaxPausedHeight uint64   `json:"maxPausedHeight"`      // Block height when pause expires (0 = not paused)
	UnstakingHeight uint64   `json:"unstakingHeight"`      // Block height when unstaking completes (0 = not unstaking)
	Output          string   `json:"output,omitempty"`     // Reward destination address (hex-encoded, empty = self)
	Delegate        bool     `json:"delegate,omitempty"`   // Whether validator accepts delegations
	Compound        bool     `json:"compound,omitempty"`   // Whether rewards auto-compound into stake
}

// RpcNonSigner represents non-signing info returned from the RPC.
// This matches the NonSigner protobuf structure from Canopy (fsm/validator.pb.go:236-244).
// Tracks validators that have failed to sign blocks within the non-sign tracking window.
type RpcNonSigner struct {
	Address string `json:"address"` // Hex-encoded validator address
	Counter uint64 `json:"counter"` // Number of blocks missed within the tracking window
}

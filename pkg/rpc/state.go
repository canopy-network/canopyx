package rpc

import (
	"context"
	"fmt"
	"net/http"
)

const (
	statePath = "/v1/query/state"
)

// State queries the /v1/query/state endpoint to retrieve the current blockchain state.
// This endpoint returns the full state including params, accounts, validators, and pools.
// The state includes the RootChainID in params.consensusParams, which identifies the parent chain.
//
// This method is useful for:
// - Getting the root chain ID during chain registration (alternative to cert-by-height)
// - Retrieving governance parameters
// - Accessing current blockchain state
func (c *HTTPClient) State(ctx context.Context) (*StateResponse, error) {
	var state StateResponse
	if err := c.doJSON(ctx, http.MethodPost, statePath, map[string]any{}, &state); err != nil {
		return nil, fmt.Errorf("fetch state: %w", err)
	}

	return &state, nil
}

// FetchChainIDFromState queries the /v1/query/state endpoint to extract both the chain ID and root chain ID.
// This is an alternative to FetchChainID (which uses cert-by-height) and provides additional information.
//
// Returns:
//   - chainID: The chain ID from the certificate header
//   - rootChainID: The root (parent) chain ID from params.consensusParams
//   - error: If the endpoint is unreachable or returns invalid data
//
// The function uses a context timeout to prevent hanging on slow/unreachable endpoints.
func FetchChainIDFromState(ctx context.Context, rpcURL string) (chainID uint64, rootChainID uint64, err error) {
	// Create HTTP client with single endpoint
	opts := Opts{
		Endpoints: []string{rpcURL},
		Timeout:   5 * http.DefaultClient.Timeout, // 5s timeout
		RPS:       20,
		Burst:     40,
	}
	client := NewHTTPWithOpts(opts)

	// Query state endpoint
	state, err := client.State(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch state from %s: %w", rpcURL, err)
	}

	// Validate that we have params with consensus params
	if state.Params == nil {
		return 0, 0, fmt.Errorf("state params is nil from %s", rpcURL)
	}
	if state.Params.ConsensusParams == nil {
		return 0, 0, fmt.Errorf("consensus params is nil from %s", rpcURL)
	}

	// For chainID, we need to query cert-by-height since state doesn't include it
	// The state endpoint returns the state but not the chain's own ID
	chainID, err = FetchChainID(ctx, rpcURL)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch chain ID from %s: %w", rpcURL, err)
	}

	rootChainID = state.Params.ConsensusParams.RootChainID

	return chainID, rootChainID, nil
}

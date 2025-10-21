package rpc

import (
	"context"
	"fmt"
	"net/http"
)

// GetGenesisState fetches the full genesis state at height 0.
// This is used by the account indexer to cache genesis for height 1 comparisons.
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
	if err := c.doJSON(ctx, http.MethodGet, "/v1/query/state", args, &genesis); err != nil {
		return nil, fmt.Errorf("fetch genesis state at height %d: %w", height, err)
	}

	return &genesis, nil
}

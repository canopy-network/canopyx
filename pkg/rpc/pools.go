package rpc

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopy/fsm"
)

// PoolsByHeight returns all pools at the current chain head using pagination.
// The RPC endpoint returns pools in pages, this method fetches all pages.
//
// Returns fsm.Pool (protobuf type from Canopy).
// The Canopy RPC endpoint /v1/query/pools returns fsm.Pool structures.
func (c *HTTPClient) PoolsByHeight(ctx context.Context, height uint64) ([]*fsm.Pool, error) {
	pools, err := ListPaged[*fsm.Pool](ctx, c, poolsPath, NewQueryByHeightRequest(height))
	if err != nil {
		return nil, fmt.Errorf("fetch pools: %w", err)
	}
	return pools, nil
}

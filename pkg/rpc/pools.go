package rpc

import (
	"context"
	"fmt"
)

// PointsEntry represents a single address and points allocation in a pool.
type PointsEntry struct {
	Address string `json:"address"`
	Points  uint64 `json:"points"`
}

// RpcPool represents a pool response from the RPC.
// This matches the JSON structure returned by /v1/query/pool?id={id}.
type RpcPool struct {
	ID              uint64        `json:"id"`
	ChainID         uint64        `json:"chainId,omitempty"` // Blockchain's internal chain/committee ID (if provided by RPC)
	Amount          uint64        `json:"amount"`
	Points          []PointsEntry `json:"poolPoints"`
	TotalPoolPoints uint64        `json:"totalPoolPoints"`
}

// PoolsByHeight returns all pools at the current chain head using pagination.
// The RPC endpoint returns pools in pages, this method fetches all pages.
func (c *HTTPClient) PoolsByHeight(ctx context.Context, height uint64) ([]*RpcPool, error) {
	pools, err := ListPaged[*RpcPool](ctx, c, poolsPath, NewQueryByHeightRequest(height))
	if err != nil {
		return nil, fmt.Errorf("fetch pools: %w", err)
	}
	return pools, nil
}

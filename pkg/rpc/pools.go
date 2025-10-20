package rpc

import (
	"context"
	"fmt"
	"net/http"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

const (
	poolByIDPath = "/v1/query/pool"
	poolsPath    = "/v1/query/pools"
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
	ChainID         uint64        `json:"chain_id"` // Blockchain's internal chain/committee ID
	Amount          uint64        `json:"amount"`
	Points          []PointsEntry `json:"points"`
	TotalPoolPoints uint64        `json:"total_pool_points"`
}

// ToPool converts an RpcPool to the database Pool model.
// The points array is used only to calculate lp_count.
// Individual points allocations are not stored in the pool table.
// The chain_id comes from the RPC data (blockchain's internal chain ID).
func (rp *RpcPool) ToPool(height uint64) *indexer.Pool {
	return &indexer.Pool{
		PoolID:      rp.ID,
		Height:      height,
		ChainID:     rp.ChainID, // Use chain_id from RPC
		Amount:      rp.Amount,
		TotalPoints: rp.TotalPoolPoints,
		LPCount:     uint32(len(rp.Points)),
	}
}

// PoolByID returns a single pool by ID at the current chain head.
// This method fetches pool state from the RPC endpoint.
func (c *HTTPClient) PoolByID(ctx context.Context, id uint64) (*RpcPool, error) {
	args := map[string]any{"id": id}

	var pool RpcPool
	if err := c.doJSON(ctx, http.MethodPost, poolByIDPath, args, &pool); err != nil {
		return nil, fmt.Errorf("fetch pool %d: %w", id, err)
	}

	return &pool, nil
}

// Pools returns all pools at the current chain head using pagination.
// The RPC endpoint returns pools in pages, this method fetches all pages.
func (c *HTTPClient) Pools(ctx context.Context) ([]*RpcPool, error) {
	pools, err := ListPaged[*RpcPool](ctx, c, poolsPath, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch pools: %w", err)
	}
	return pools, nil
}

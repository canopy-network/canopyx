package rpc

import (
	"context"
	"fmt"
	"net/http"
)

const (
	dexBatchByHeightPath     = "/v1/query/dex-batch"
	nextDexBatchByHeightPath = "/v1/query/next-dex-batch"
)

// DexBatchByHeight fetches the current DEX batch at a specific height for a given committee.
// The current batch contains orders that are "locked" for execution at this height.
//
// The Canopy RPC endpoint /v1/query/dex-batch supports:
//   - height: uint64 - The block height to query (required)
//   - committee: uint64 - The committee (counter-asset chain) ID (required)
//
// Returns nil if no batch exists at this height for the committee.
func (c *HTTPClient) DexBatchByHeight(ctx context.Context, height uint64, committee uint64) (*RpcDexBatch, error) {
	args := map[string]any{
		"height":    height,
		"committee": committee,
	}

	var batch RpcDexBatch
	if err := c.doJSON(ctx, http.MethodPost, dexBatchByHeightPath, args, &batch); err != nil {
		return nil, fmt.Errorf("fetch dex batch at height %d, committee %d: %w", height, committee, err)
	}

	return &batch, nil
}

// NextDexBatchByHeight fetches the next DEX batch at a specific height for a given committee.
// The next batch contains orders in "future" state that will be locked in the next batch rotation.
//
// The Canopy RPC endpoint /v1/query/next-dex-batch supports:
//   - height: uint64 - The block height to query (required)
//   - committee: uint64 - The committee (counter-asset chain) ID (required)
//
// Returns nil if no next batch exists at this height for the committee.
func (c *HTTPClient) NextDexBatchByHeight(ctx context.Context, height uint64, committee uint64) (*RpcDexBatch, error) {
	args := map[string]any{
		"height":    height,
		"committee": committee,
	}

	var batch RpcDexBatch
	if err := c.doJSON(ctx, http.MethodPost, nextDexBatchByHeightPath, args, &batch); err != nil {
		return nil, fmt.Errorf("fetch next dex batch at height %d, committee %d: %w", height, committee, err)
	}

	return &batch, nil
}

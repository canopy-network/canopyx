package rpc

import (
	"context"
	"fmt"
	"net/http"

	"github.com/canopy-network/canopy/lib"
)

// AllDexBatchesByHeight fetches ALL DEX batches at a specific height across all committees.
// Uses the special id: 0 parameter to return batches for all committees in a single call.
//
// This is more efficient than querying committees and fetching each batch individually.
// The RPC returns all active batches at the specified height.
//
// Returns lib.DexBatch (protobuf type from Canopy).
// The Canopy RPC endpoint /v1/query/dex-batch returns lib.DexBatch structures.
func (c *HTTPClient) AllDexBatchesByHeight(ctx context.Context, height uint64) ([]*lib.DexBatch, error) {
	req := DexBatchRequest{
		Height:    height,
		Committee: 0, // 0 means "all committees"
	}

	// Try to unmarshal as array first (multiple batches)
	var batches []*lib.DexBatch
	if err := c.doJSON(ctx, http.MethodPost, dexBatchByHeightPath, req, &batches); err != nil {
		// If that fails, try as single object
		var singleBatch lib.DexBatch
		if err := c.doJSON(ctx, http.MethodPost, dexBatchByHeightPath, req, &singleBatch); err != nil {
			return nil, fmt.Errorf("fetch all dex batches at height %d: %w", height, err)
		}
		batches = []*lib.DexBatch{&singleBatch}
	}

	return batches, nil
}

// AllNextDexBatchesByHeight fetches ALL next DEX batches at a specific height across all committees.
// Uses the special id: 0 parameter to return next batches for all committees in a single call.
//
// This is more efficient than querying committees and fetching each batch individually.
// The RPC returns all active next batches at the specified height.
//
// Returns lib.DexBatch (protobuf type from Canopy).
// The Canopy RPC endpoint /v1/query/next-dex-batch returns lib.DexBatch structures.
func (c *HTTPClient) AllNextDexBatchesByHeight(ctx context.Context, height uint64) ([]*lib.DexBatch, error) {
	req := DexBatchRequest{
		Height:    height,
		Committee: 0, // 0 means "all committees"
	}

	// Try to unmarshal as array first (multiple batches)
	var batches []*lib.DexBatch
	if err := c.doJSON(ctx, http.MethodPost, nextDexBatchByHeightPath, req, &batches); err != nil {
		// If that fails, try as single object
		var singleBatch lib.DexBatch
		if err := c.doJSON(ctx, http.MethodPost, nextDexBatchByHeightPath, req, &singleBatch); err != nil {
			return nil, fmt.Errorf("fetch all next dex batches at height %d: %w", height, err)
		}
		batches = []*lib.DexBatch{&singleBatch}
	}

	return batches, nil
}

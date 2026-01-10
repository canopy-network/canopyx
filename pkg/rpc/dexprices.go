package rpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/canopy-network/canopy/lib"
)

// DexPricesByHeight fetches all DEX prices for all chain pairs.
// This endpoint returns the current state of all DEX pools.
//
// Returns lib.DexPrice (type from Canopy lib/dex.go).
// The Canopy RPC endpoint /v1/query/dex-price returns lib.DexPrice structures.
// Callers should convert to indexer models using transform.DexPrice() if needed.
func (c *HTTPClient) DexPricesByHeight(ctx context.Context, height uint64) ([]*lib.DexPrice, error) {
	// The RPC might return either:
	// 1. A single object (when there's only one price)
	// 2. An array of objects (when there are multiple prices)
	// We need to handle both cases
	req := DexPriceRequest{Height: height, ID: 0}

	var raw json.RawMessage
	// Send an empty object instead of nil to satisfy server requirements
	if err := c.doJSON(ctx, "POST", dexPricePath, req, &raw); err != nil {
		return nil, fmt.Errorf("fetch all dex prices: %w", err)
	}

	// Try to unmarshal as an array first
	var rpcPrices []*lib.DexPrice
	if err := json.Unmarshal(raw, &rpcPrices); err != nil {
		// If that fails, try as a single object
		var singlePrice lib.DexPrice
		if err := json.Unmarshal(raw, &singlePrice); err != nil {
			return nil, fmt.Errorf("unmarshal dex prices: %w", err)
		}
		rpcPrices = []*lib.DexPrice{&singlePrice}
	}

	// Return lib types - callers will transform to DB models
	result := make([]*lib.DexPrice, 0, len(rpcPrices))
	result = append(result, rpcPrices...)

	return result, nil
}

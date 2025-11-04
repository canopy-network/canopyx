package rpc

import (
	"context"
	"encoding/json"
	"fmt"
)

// RpcDexPrice represents the DEX price information returned from the RPC endpoint.
// This matches the JSON structure returned by /v1/query/dex-price?id={chain}.
// JSON tags match Canopy's actual response format (camelCase).
type RpcDexPrice struct {
	LocalChainID  uint64 `json:"chainId"`
	RemoteChainID uint64 `json:"remoteChainId"`
	LocalPool     uint64 `json:"localPool"`
	RemotePool    uint64 `json:"remotePool"`
	E6ScaledPrice uint64 `json:"e6ScaledPrice"`
}

// DexPricesByHeight fetches all DEX prices for all chain pairs.
// This endpoint returns the current state of all DEX pools.
// Returns a slice of RpcDexPrice or an error if the request fails.
// Callers should convert to indexer models using transform.DexPrice() if needed.
func (c *HTTPClient) DexPricesByHeight(ctx context.Context, height uint64) ([]*RpcDexPrice, error) {
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
	var rpcPrices []RpcDexPrice
	if err := json.Unmarshal(raw, &rpcPrices); err != nil {
		// If that fails, try as a single object
		var singlePrice RpcDexPrice
		if err := json.Unmarshal(raw, &singlePrice); err != nil {
			return nil, fmt.Errorf("unmarshal dex prices: %w", err)
		}
		rpcPrices = []RpcDexPrice{singlePrice}
	}

	// Return RPC types - callers will transform to DB models
	result := make([]*RpcDexPrice, 0, len(rpcPrices))
	for i := range rpcPrices {
		result = append(result, &rpcPrices[i])
	}

	return result, nil
}

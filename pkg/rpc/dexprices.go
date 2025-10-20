package rpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// RpcDexPrice represents the DEX price information returned from the RPC endpoint.
// This matches the JSON structure returned by /v1/query/dex-price?id={chain}.
type RpcDexPrice struct {
	LocalChainID  uint64 `json:"LocalChainId"`
	RemoteChainID uint64 `json:"RemoteChainId"`
	LocalPool     uint64 `json:"LocalPool"`
	RemotePool    uint64 `json:"RemotePool"`
	E6ScaledPrice uint64 `json:"E6ScaledPrice"`
}

// ToDexPrice converts an RPC dex price response to the database model.
// The height and height_time fields must be populated by the caller (activity layer).
func (r *RpcDexPrice) ToDexPrice() *indexer.DexPrice {
	return &indexer.DexPrice{
		LocalChainID:  r.LocalChainID,
		RemoteChainID: r.RemoteChainID,
		LocalPool:     r.LocalPool,
		RemotePool:    r.RemotePool,
		PriceE6:       r.E6ScaledPrice,
		// Height and HeightTime will be set by the activity layer
	}
}

// DexPrice fetches the DEX price for a specific chain pair.
// The chainId parameter identifies the remote chain in the pair.
// Returns a single DexPrice record or an error if the request fails.
func (c *HTTPClient) DexPrice(ctx context.Context, chainID uint64) (*indexer.DexPrice, error) {
	var rpcPrice RpcDexPrice
	if err := c.doJSON(ctx, "POST", dexPricePath, map[string]any{"id": chainID}, &rpcPrice); err != nil {
		return nil, fmt.Errorf("fetch dex price for chain %d: %w", chainID, err)
	}

	return rpcPrice.ToDexPrice(), nil
}

// DexPrices fetches all DEX prices for all chain pairs.
// This endpoint returns the current state of all DEX pools.
// Returns a slice of DexPrice records or an error if the request fails.
func (c *HTTPClient) DexPrices(ctx context.Context) ([]*indexer.DexPrice, error) {
	// The RPC might return either:
	// 1. A single object (when there's only one price)
	// 2. An array of objects (when there are multiple prices)
	// We need to handle both cases

	var raw json.RawMessage
	// Send empty object instead of nil to satisfy server requirements
	if err := c.doJSON(ctx, "POST", dexPricePath, map[string]any{}, &raw); err != nil {
		return nil, fmt.Errorf("fetch all dex prices: %w", err)
	}

	// Try to unmarshal as array first
	var rpcPrices []RpcDexPrice
	if err := json.Unmarshal(raw, &rpcPrices); err != nil {
		// If that fails, try as single object
		var singlePrice RpcDexPrice
		if err := json.Unmarshal(raw, &singlePrice); err != nil {
			return nil, fmt.Errorf("unmarshal dex prices: %w", err)
		}
		rpcPrices = []RpcDexPrice{singlePrice}
	}

	// Convert to database models
	dbPrices := make([]*indexer.DexPrice, 0, len(rpcPrices))
	for i := range rpcPrices {
		dbPrices = append(dbPrices, rpcPrices[i].ToDexPrice())
	}

	return dbPrices, nil
}

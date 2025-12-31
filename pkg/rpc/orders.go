package rpc

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopy/lib"
)

// OrdersByHeight fetches ALL orders at a specific height.
// When chainID is 0, it returns all order books for all chains.
// When chainID is specified, it returns orders for that specific chain.
//
// Returns lib.SellOrder (protobuf type from Canopy).
// The Canopy RPC endpoint /v1/query/orders returns lib.OrderBook structures.
func (c *HTTPClient) OrdersByHeight(ctx context.Context, height uint64, chainID uint64) ([]*lib.SellOrder, error) {
	req := QueryByHeightRequest{Height: height}
	if chainID > 0 {
		// When chainID is specified, use the request that includes it
		req := struct {
			Height  uint64 `json:"height"`
			ChainID uint64 `json:"chainId"`
		}{Height: height, ChainID: chainID}

		var orderBooks []*lib.OrderBook
		if err := c.doJSON(ctx, "POST", ordersByHeightPath, req, &orderBooks); err != nil {
			return nil, fmt.Errorf("fetch orders at height %d, chainId %d: %w", height, chainID, err)
		}

		// Extract orders from the response
		var allOrders []*lib.SellOrder
		for _, book := range orderBooks {
			allOrders = append(allOrders, book.Orders...)
		}
		return allOrders, nil
	}

	// chainID == 0: fetch all order books
	var orderBooks []*lib.OrderBook
	if err := c.doJSON(ctx, "POST", ordersByHeightPath, req, &orderBooks); err != nil {
		return nil, fmt.Errorf("fetch all orders at height %d: %w", height, err)
	}

	// Extract all orders from all books
	var allOrders []*lib.SellOrder
	for _, book := range orderBooks {
		allOrders = append(allOrders, book.Orders...)
	}
	return allOrders, nil
}

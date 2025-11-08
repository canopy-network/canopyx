package rpc

import (
	"context"
	"fmt"
)

// RpcOrder represents an order returned from Canopy's RPC endpoint.
// This maps to the SellOrder proto structure returned by /v1/query/order and /v1/query/orders.
// See: /home/overlordyorch/Development/canopy/lib/.proto/swap.proto
type RpcOrder struct {
	ID                   string `json:"id"`                   // Unique order identifier (20 bytes hex)
	Committee            uint64 `json:"committee"`            // Committee ID for the order
	Data                 string `json:"data"`                 // Generic data field for committee-specific functionality
	AmountForSale        uint64 `json:"amountForSale"`        // Amount of CNPY for sale
	RequestedAmount      uint64 `json:"requestedAmount"`      // Amount of counter-asset to receive
	SellerReceiveAddress string `json:"sellerReceiveAddress"` // External chain address to receive counter-asset
	BuyerSendAddress     string `json:"buyerSendAddress"`     // Address buyer transfers funds from (empty if not locked)
	BuyerReceiveAddress  string `json:"buyerReceiveAddress"`  // Buyer Canopy address to receive CNPY (empty if not locked)
	BuyerChainDeadline   uint64 `json:"buyerChainDeadline"`   // External chain height deadline (0 if not locked)
	SellersSendAddress   string `json:"sellersSendAddress"`   // Signing address of seller selling CNPY
}

// RpcOrderBook represents an order book for a specific chain.
type RpcOrderBook struct {
	ChainID uint64      `json:"chainID"`
	Orders  []*RpcOrder `json:"orders"`
}

// RpcOrderBooksResponse wraps the response from the orders endpoint.
type RpcOrderBooksResponse struct {
	OrderBooks []*RpcOrderBook `json:"orderBooks"`
}

// OrdersByHeight fetches ALL orders at a specific height.
// When chainID is 0, it returns all order books for all chains.
// When chainID is specified, it returns orders for that specific chain.
//
// The Canopy RPC endpoint /v1/query/orders returns an array of OrderBooks directly:
//
//	[
//	  {
//	    "chainID": 2,
//	    "orders": [...]
//	  }
//	]
func (c *HTTPClient) OrdersByHeight(ctx context.Context, height uint64, chainID uint64) ([]*RpcOrder, error) {
	req := QueryByHeightRequest{Height: height}
	if chainID > 0 {
		// When chainID is specified, use the request that includes it
		req := struct {
			Height  uint64 `json:"height"`
			ChainID uint64 `json:"chainId"`
		}{Height: height, ChainID: chainID}

		var orderBooks []*RpcOrderBook
		if err := c.doJSON(ctx, "POST", ordersByHeightPath, req, &orderBooks); err != nil {
			return nil, fmt.Errorf("fetch orders at height %d, chainId %d: %w", height, chainID, err)
		}

		// Extract orders from the response
		var allOrders []*RpcOrder
		for _, book := range orderBooks {
			allOrders = append(allOrders, book.Orders...)
		}
		return allOrders, nil
	}

	// chainID == 0: fetch all order books
	var orderBooks []*RpcOrderBook
	if err := c.doJSON(ctx, "POST", ordersByHeightPath, req, &orderBooks); err != nil {
		return nil, fmt.Errorf("fetch all orders at height %d: %w", height, err)
	}

	// Extract all orders from all books
	var allOrders []*RpcOrder
	for _, book := range orderBooks {
		allOrders = append(allOrders, book.Orders...)
	}
	return allOrders, nil
}

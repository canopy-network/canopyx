package rpc

import (
	"context"
	"fmt"
)

// RpcOrder represents an order returned from Canopy's RPC endpoint.
// This maps to the SellOrder proto structure returned by /v1/query/order and /v1/query/orders.
type RpcOrder struct {
	OrderID         string  `json:"orderId"`         // Unique order identifier
	Committee       uint64  `json:"committee"`       // Committee ID for the order
	AmountForSale   uint64  `json:"amountForSale"`   // Amount being sold
	RequestedAmount uint64  `json:"requestedAmount"` // Amount requested in return
	SellerAddress   string  `json:"sellerAddress"`   // (SellerReceiveAddress) Address of the seller
	BuyerAddress    *string `json:"buyerAddress"`    // Address of the buyer (null if not filled) - SPLIT into recieve/send (BuyerSendAddress|BuyerReceiveAddress)
	Deadline        *uint64 `json:"deadline"`        // Order deadline height (null if no deadline)
	Status          string  `json:"status"`          // Order status: "open", "complete", "canceled"

	// TODO: Add missing fields
	//  // seller_receive_address: the external chain address to receive the 'counter-asset'
	//  bytes SellerReceiveAddress = 6; // @gotags: json:"sellerReceiveAddress"
	//  // buyer_send_address: the address the buyer will be transferring the funds from
	//  bytes BuyerSendAddress = 7; // @gotags: json:"buyerSendAddress"
	//  // buyer_receive_address: the buyer Canopy address to receive the CNPY
	//  bytes BuyerReceiveAddress = 8; // @gotags: json:"buyerReceiveAddress"
	//  // buyer_chain_deadline: the external chain height deadline to send the 'tokens' to SellerReceiveAddress
	//  uint64 BuyerChainDeadline = 9; // @gotags: json:"buyerChainDeadline"
	//  // sellers_send_address: the signing address of seller who is selling the CNPY
	//  bytes SellersSendAddress = 10; // @gotags: json:"sellersSendAddress"
	//  // "data" same of the event - needs eric oracle pr (probably save as is)
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

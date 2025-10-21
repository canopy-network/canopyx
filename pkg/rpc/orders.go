package rpc

import (
	"context"
	"fmt"
	"net/http"
)

const (
	ordersByHeightPath = "/v1/query/orders"
	orderByIDPath      = "/v1/query/order"
)

// RpcOrder represents an order returned from Canopy's RPC endpoint.
// This maps to the SellOrder proto structure returned by /v1/query/order and /v1/query/orders.
type RpcOrder struct {
	OrderID         string  `json:"orderId"`         // Unique order identifier
	Committee       uint64  `json:"committee"`       // Committee ID for the order
	AmountForSale   uint64  `json:"amountForSale"`   // Amount being sold
	RequestedAmount uint64  `json:"requestedAmount"` // Amount requested in return
	SellerAddress   string  `json:"sellerAddress"`   // Address of the seller
	BuyerAddress    *string `json:"buyerAddress"`    // Address of the buyer (null if not filled)
	Deadline        *uint64 `json:"deadline"`        // Order deadline height (null if no deadline)
	Status          string  `json:"status"`          // Order status: "open", "filled", "cancelled", "expired"
}

// OrdersResponse wraps the paginated response from the orders endpoint.
type OrdersResponse struct {
	Results    []*RpcOrder `json:"results"`
	PageNumber int         `json:"pageNumber"`
	PerPage    int         `json:"perPage"`
	TotalPages int         `json:"totalPages"`
	TotalCount int         `json:"totalCount"`
}

// OrdersByHeight fetches ALL orders at a specific height using pagination.
// This method handles pagination automatically and returns the complete set of orders.
//
// The Canopy RPC endpoint /v1/query/orders supports:
//   - height: uint64 - The block height to query (required)
//   - chainId: uint64 - The chain ID to query (required)
//   - pageNumber: int - Page number (1-indexed, default: 1)
//   - perPage: int - Results per page (default: 1000, max: 1000)
//
// This function fetches all pages in parallel for performance.
func (c *HTTPClient) OrdersByHeight(ctx context.Context, height uint64, chainID uint64) ([]*RpcOrder, error) {
	// Build request payload for first page
	args := map[string]any{
		"height":     height,
		"chainId":    chainID,
		"pageNumber": 1,
		"perPage":    1000, // Max page size
	}

	// Fetch first page to determine total pages
	var firstPage OrdersResponse
	if err := c.doJSON(ctx, http.MethodPost, ordersByHeightPath, args, &firstPage); err != nil {
		return nil, fmt.Errorf("fetch orders page 1 at height %d, chainId %d: %w", height, chainID, err)
	}

	// If only one page, return immediately
	if firstPage.TotalPages <= 1 {
		return firstPage.Results, nil
	}

	// Pre-allocate slice for all orders
	allOrders := make([]*RpcOrder, 0, firstPage.TotalCount)
	allOrders = append(allOrders, firstPage.Results...)

	// Fetch remaining pages in parallel
	type pageResult struct {
		orders []*RpcOrder
		err    error
	}
	resultsCh := make(chan pageResult, firstPage.TotalPages-1)

	for page := 2; page <= firstPage.TotalPages; page++ {
		go func(pageNum int) {
			pageArgs := map[string]any{
				"height":     height,
				"chainId":    chainID,
				"pageNumber": pageNum,
				"perPage":    1000,
			}

			var pageResp OrdersResponse
			if err := c.doJSON(ctx, http.MethodPost, ordersByHeightPath, pageArgs, &pageResp); err != nil {
				resultsCh <- pageResult{nil, fmt.Errorf("fetch orders page %d at height %d, chainId %d: %w", pageNum, height, chainID, err)}
				return
			}

			resultsCh <- pageResult{pageResp.Results, nil}
		}(page)
	}

	// Collect results from parallel fetches
	for i := 0; i < firstPage.TotalPages-1; i++ {
		result := <-resultsCh
		if result.err != nil {
			return nil, result.err
		}
		allOrders = append(allOrders, result.orders...)
	}

	return allOrders, nil
}

// OrderByID fetches a single order by its ID at a specific chain and height.
//
// The Canopy RPC endpoint /v1/query/order supports:
//   - orderId: string - The order ID to query (required)
//   - chainId: uint64 - The chain ID to query (required)
//   - height: uint64 - The block height to query (optional, defaults to latest)
//
// Returns the order or an error if not found.
func (c *HTTPClient) OrderByID(ctx context.Context, orderID string, chainID uint64, height *uint64) (*RpcOrder, error) {
	args := map[string]any{
		"orderId": orderID,
		"chainId": chainID,
	}

	// Add optional height parameter
	if height != nil {
		args["height"] = *height
	}

	var order RpcOrder
	if err := c.doJSON(ctx, http.MethodPost, orderByIDPath, args, &order); err != nil {
		return nil, fmt.Errorf("fetch order %s at chainId %d: %w", orderID, chainID, err)
	}

	return &order, nil
}

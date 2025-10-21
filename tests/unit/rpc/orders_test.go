package rpc_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/canopy-network/canopyx/pkg/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHTTPClient_OrderByID tests fetching a single order by ID.
func TestHTTPClient_OrderByID(t *testing.T) {
	buyerAddr := "buyer1"
	deadline := uint64(10000)
	response := rpc.RpcOrder{
		OrderID:         "order456",
		Committee:       2,
		AmountForSale:   5000,
		RequestedAmount: 10000,
		SellerAddress:   "seller1",
		BuyerAddress:    &buyerAddr,
		Deadline:        &deadline,
		Status:          "open",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/v1/query/order")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	})

	client := newTestRPCClient(handler)
	height := uint64(1000)
	order, err := client.OrderByID(context.Background(), "order456", 2, &height)

	require.NoError(t, err)
	require.NotNil(t, order)
	assert.Equal(t, response.OrderID, order.OrderID)
	assert.Equal(t, response.Committee, order.Committee)
	assert.Equal(t, response.Status, order.Status)
}

// TestHTTPClient_OrdersByHeight tests fetching orders at a specific height.
func TestHTTPClient_OrdersByHeight(t *testing.T) {
	response := rpc.OrdersResponse{
		Results: []*rpc.RpcOrder{
			{OrderID: "order1", Committee: 1, AmountForSale: 1000, Status: "open"},
			{OrderID: "order2", Committee: 2, AmountForSale: 2000, Status: "filled"},
		},
		PageNumber: 1,
		PerPage:    1000,
		TotalPages: 1,
		TotalCount: 2,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/query/orders", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	})

	client := newTestRPCClient(handler)
	orders, err := client.OrdersByHeight(context.Background(), 1000, 1)

	require.NoError(t, err)
	require.Len(t, orders, 2)
	assert.Equal(t, "order1", orders[0].OrderID)
	assert.Equal(t, "order2", orders[1].OrderID)
}

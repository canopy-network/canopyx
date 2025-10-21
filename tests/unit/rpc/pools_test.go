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

// TestRpcPool_ToPool tests the conversion from RPC format to DB model.
func TestRpcPool_ToPool(t *testing.T) {
	rpcPool := rpc.RpcPool{
		ID:              1,
		ChainID:         5,
		Amount:          1000000,
		Points:          []rpc.PointsEntry{{Address: "addr1", Points: 500}, {Address: "addr2", Points: 500}},
		TotalPoolPoints: 1000,
	}

	dbPool := rpcPool.ToPool(100)

	assert.NotNil(t, dbPool)
	assert.Equal(t, rpcPool.ID, dbPool.PoolID)
	assert.Equal(t, rpcPool.ChainID, dbPool.ChainID)
	assert.Equal(t, uint64(100), dbPool.Height)
	assert.Equal(t, rpcPool.Amount, dbPool.Amount)
	assert.Equal(t, rpcPool.TotalPoolPoints, dbPool.TotalPoints)
	assert.Equal(t, uint32(2), dbPool.LPCount)
}

// TestHTTPClient_PoolByID tests fetching a single pool by ID.
func TestHTTPClient_PoolByID(t *testing.T) {
	response := rpc.RpcPool{
		ID:              1,
		Amount:          5000000,
		Points:          []rpc.PointsEntry{{Address: "addr1", Points: 3000}},
		TotalPoolPoints: 5000,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/v1/query/pool")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	})

	client := newTestRPCClient(handler)
	pool, err := client.PoolByID(context.Background(), 1)

	require.NoError(t, err)
	require.NotNil(t, pool)
	assert.Equal(t, response.ID, pool.ID)
	assert.Equal(t, response.Amount, pool.Amount)
}

// TestHTTPClient_Pools tests fetching all pools.
func TestHTTPClient_Pools(t *testing.T) {
	response := struct {
		Results []*rpc.RpcPool `json:"results"`
	}{
		Results: []*rpc.RpcPool{
			{ID: 1, Amount: 1000000, TotalPoolPoints: 1000},
			{ID: 2, Amount: 2000000, TotalPoolPoints: 2000},
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/query/pools", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	})

	client := newTestRPCClient(handler)
	pools, err := client.Pools(context.Background())

	require.NoError(t, err)
	require.Len(t, pools, 2)
	assert.Equal(t, uint64(1), pools[0].ID)
	assert.Equal(t, uint64(2), pools[1].ID)
}

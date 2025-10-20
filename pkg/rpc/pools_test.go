package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRpcPool_ToPool tests the conversion from RPC format to DB model.
func TestRpcPool_ToPool(t *testing.T) {
	rpcPool := RpcPool{
		ID:              1,
		Amount:          1000000,
		Points:          []PointsEntry{{Address: "addr1", Points: 500}, {Address: "addr2", Points: 500}},
		TotalPoolPoints: 1000,
	}

	dbPool := rpcPool.ToPool(5, 100)

	assert.NotNil(t, dbPool)
	assert.Equal(t, rpcPool.ID, dbPool.PoolID)
	assert.Equal(t, uint64(5), dbPool.ChainID)
	assert.Equal(t, uint64(100), dbPool.Height)
	assert.Equal(t, rpcPool.Amount, dbPool.Amount)
	assert.Equal(t, rpcPool.TotalPoolPoints, dbPool.TotalPoints)
	assert.Equal(t, uint32(2), dbPool.LPCount)
}

// TestHTTPClient_PoolByID tests fetching a single pool by ID.
func TestHTTPClient_PoolByID(t *testing.T) {
	response := RpcPool{
		ID:              1,
		Amount:          5000000,
		Points:          []PointsEntry{{Address: "addr1", Points: 3000}},
		TotalPoolPoints: 5000,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/v1/query/pool")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewHTTPWithOpts(Opts{Endpoints: []string{server.URL}})
	pool, err := client.PoolByID(context.Background(), 1)

	require.NoError(t, err)
	require.NotNil(t, pool)
	assert.Equal(t, response.ID, pool.ID)
	assert.Equal(t, response.Amount, pool.Amount)
}

// TestHTTPClient_Pools tests fetching all pools.
func TestHTTPClient_Pools(t *testing.T) {
	response := struct {
		Results []*RpcPool `json:"results"`
	}{
		Results: []*RpcPool{
			{ID: 1, Amount: 1000000, TotalPoolPoints: 1000},
			{ID: 2, Amount: 2000000, TotalPoolPoints: 2000},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/query/pools", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewHTTPWithOpts(Opts{Endpoints: []string{server.URL}})
	pools, err := client.Pools(context.Background())

	require.NoError(t, err)
	require.Len(t, pools, 2)
	assert.Equal(t, uint64(1), pools[0].ID)
	assert.Equal(t, uint64(2), pools[1].ID)
}

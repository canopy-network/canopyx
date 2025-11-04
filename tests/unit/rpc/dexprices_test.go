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

// TestHTTPClient_DexPrice tests fetching a single DEX price.
func TestHTTPClient_DexPrice(t *testing.T) {
	tests := []struct {
		name           string
		chainID        uint64
		serverResponse interface{}
		statusCode     int
		wantErr        bool
		validateResult func(*testing.T, rpc.RpcDexPrice)
	}{
		{
			name:    "successful fetch",
			chainID: 2,
			serverResponse: map[string]interface{}{
				"LocalChainId":  float64(1),
				"RemoteChainId": float64(2),
				"LocalPool":     float64(1000000),
				"RemotePool":    float64(2000000),
				"E6ScaledPrice": float64(500000),
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			validateResult: func(t *testing.T, price rpc.RpcDexPrice) {
				assert.Equal(t, uint64(1), price.LocalChainID)
				assert.Equal(t, uint64(2), price.RemoteChainID)
				assert.Equal(t, uint64(1000000), price.LocalPool)
				assert.Equal(t, uint64(2000000), price.RemotePool)
				assert.Equal(t, uint64(500000), price.E6ScaledPrice)
			},
		},
		{
			name:           "server error",
			chainID:        2,
			serverResponse: nil,
			statusCode:     http.StatusInternalServerError,
			wantErr:        true,
		},
		{
			name:    "zero values",
			chainID: 5,
			serverResponse: map[string]interface{}{
				"LocalChainId":  float64(1),
				"RemoteChainId": float64(5),
				"LocalPool":     float64(0),
				"RemotePool":    float64(0),
				"E6ScaledPrice": float64(0),
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			validateResult: func(t *testing.T, price rpc.RpcDexPrice) {
				assert.Equal(t, uint64(0), price.LocalPool)
				assert.Equal(t, uint64(0), price.RemotePool)
				assert.Equal(t, uint64(0), price.E6ScaledPrice)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/v1/query/dex-price", r.URL.Path)

				var reqBody map[string]interface{}
				err := json.NewDecoder(r.Body).Decode(&reqBody)
				require.NoError(t, err)
				assert.Equal(t, float64(tt.chainID), reqBody["id"])

				w.WriteHeader(tt.statusCode)
				if tt.serverResponse != nil {
					json.NewEncoder(w).Encode(tt.serverResponse)
				}
			})

			client := newTestRPCClient(handler)

			// Execute
			result, err := client.DexPrice(context.Background(), tt.chainID)

			// Validate
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				if tt.validateResult != nil {
					tt.validateResult(t, *result)
				}
			}
		})
	}
}

// TestHTTPClient_DexPrices tests fetching all DEX prices.
func TestHTTPClient_DexPrices(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse interface{}
		statusCode     int
		wantErr        bool
		wantCount      int
		validateResult func(*testing.T, []*rpc.RpcDexPrice)
	}{
		{
			name: "multiple prices",
			serverResponse: []map[string]interface{}{
				{
					"LocalChainId":  float64(1),
					"RemoteChainId": float64(2),
					"LocalPool":     float64(1000000),
					"RemotePool":    float64(2000000),
					"E6ScaledPrice": float64(500000),
				},
				{
					"LocalChainId":  float64(1),
					"RemoteChainId": float64(3),
					"LocalPool":     float64(3000000),
					"RemotePool":    float64(1500000),
					"E6ScaledPrice": float64(2000000),
				},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantCount:  2,
			validateResult: func(t *testing.T, prices []*rpc.RpcDexPrice) {
				assert.Len(t, prices, 2)
				assert.Equal(t, uint64(1), prices[0].LocalChainID)
				assert.Equal(t, uint64(2), prices[0].RemoteChainID)
				assert.Equal(t, uint64(1), prices[1].LocalChainID)
				assert.Equal(t, uint64(3), prices[1].RemoteChainID)
			},
		},
		{
			name: "single price as object",
			serverResponse: map[string]interface{}{
				"LocalChainId":  float64(1),
				"RemoteChainId": float64(2),
				"LocalPool":     float64(1000000),
				"RemotePool":    float64(2000000),
				"E6ScaledPrice": float64(500000),
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantCount:  1,
			validateResult: func(t *testing.T, prices []*rpc.RpcDexPrice) {
				assert.Len(t, prices, 1)
				assert.Equal(t, uint64(1), prices[0].LocalChainID)
				assert.Equal(t, uint64(2), prices[0].RemoteChainID)
			},
		},
		{
			name:           "empty array",
			serverResponse: []map[string]interface{}{},
			statusCode:     http.StatusOK,
			wantErr:        false,
			wantCount:      0,
		},
		{
			name:           "server error",
			serverResponse: nil,
			statusCode:     http.StatusInternalServerError,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/v1/query/dex-price", r.URL.Path)

				w.WriteHeader(tt.statusCode)
				if tt.serverResponse != nil {
					json.NewEncoder(w).Encode(tt.serverResponse)
				}
			})

			client := newTestRPCClient(handler)

			// Execute
			results, err := client.DexPricesByHeight(context.Background())

			// Validate
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, results)
			} else {
				require.NoError(t, err)
				require.NotNil(t, results)
				assert.Len(t, results, tt.wantCount)

				if tt.validateResult != nil {
					tt.validateResult(t, results)
				}
			}
		})
	}
}

// TestHTTPClient_DexPrice_Retry tests the retry logic with circuit breaker.
func TestHTTPClient_DexPrice_Retry(t *testing.T) {
	requestCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount <= 2 {
			// Fail first 2 requests
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Succeed on 3rd request
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"LocalChainId":  float64(1),
			"RemoteChainId": float64(2),
			"LocalPool":     float64(1000000),
			"RemotePool":    float64(2000000),
			"E6ScaledPrice": float64(500000),
		})
	})

	client := newTestRPCClientWithOpts(handler, rpc.Opts{
		Endpoints:       []string{"http://mock1", "http://mock2", "http://mock3"},
		BreakerFailures: 5,
	})

	result, err := client.DexPrice(context.Background(), 2)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, uint64(1), result.LocalChainID)
	assert.Equal(t, uint64(2), result.RemoteChainID)
	assert.GreaterOrEqual(t, requestCount, 3, "Should have retried at least twice")
}

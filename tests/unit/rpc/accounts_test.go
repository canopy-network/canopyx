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

// TestAccountsByHeight_Success tests successful account fetching with pagination
func TestAccountsByHeight_Success(t *testing.T) {
	// Create mock accounts response
	mockAccounts := []*rpc.Account{
		{Address: "0x123", Amount: 1000},
		{Address: "0x456", Amount: 2000},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request path
		assert.Equal(t, "/v1/query/accounts", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		// Parse request body
		var req map[string]any
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Verify height parameter
		assert.Equal(t, float64(100), req["height"])

		// Return mock response
		resp := struct {
			Results    []*rpc.Account `json:"results"`
			PageNumber int            `json:"pageNumber"`
			PerPage    int            `json:"perPage"`
			TotalPages int            `json:"totalPages"`
			TotalCount int            `json:"totalCount"`
		}{
			Results:    mockAccounts,
			PageNumber: 1,
			PerPage:    1000,
			TotalPages: 1,
			TotalCount: len(mockAccounts),
		}
		json.NewEncoder(w).Encode(resp)
	})

	client := newTestRPCClient(handler)

	// Test
	ctx := context.Background()
	accounts, err := client.AccountsByHeight(ctx, 100)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, accounts, 2)
	assert.Equal(t, "0x123", accounts[0].Address)
	assert.Equal(t, uint64(1000), accounts[0].Amount)
	assert.Equal(t, "0x456", accounts[1].Address)
	assert.Equal(t, uint64(2000), accounts[1].Amount)
}

// TestAccountsByHeight_MultiplePagesSuccess tests pagination handling
func TestAccountsByHeight_MultiplePagesSuccess(t *testing.T) {
	// Mock data for different pages
	page1Accounts := []*rpc.Account{
		{Address: "0x001", Amount: 100},
		{Address: "0x002", Amount: 200},
	}
	page2Accounts := []*rpc.Account{
		{Address: "0x003", Amount: 300},
		{Address: "0x004", Amount: 400},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request
		var req map[string]any
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		pageNum := 1
		if raw, ok := req["pageNumber"]; ok {
			if v, ok := raw.(float64); ok {
				pageNum = int(v)
			}
		}

		var resp struct {
			Results    []*rpc.Account `json:"results"`
			PageNumber int            `json:"pageNumber"`
			PerPage    int            `json:"perPage"`
			TotalPages int            `json:"totalPages"`
			TotalCount int            `json:"totalCount"`
		}
		if pageNum == 1 {
			resp = struct {
				Results    []*rpc.Account `json:"results"`
				PageNumber int            `json:"pageNumber"`
				PerPage    int            `json:"perPage"`
				TotalPages int            `json:"totalPages"`
				TotalCount int            `json:"totalCount"`
			}{
				Results:    page1Accounts,
				PageNumber: 1,
				PerPage:    2,
				TotalPages: 2,
				TotalCount: 4,
			}
		} else {
			resp = struct {
				Results    []*rpc.Account `json:"results"`
				PageNumber int            `json:"pageNumber"`
				PerPage    int            `json:"perPage"`
				TotalPages int            `json:"totalPages"`
				TotalCount int            `json:"totalCount"`
			}{
				Results:    page2Accounts,
				PageNumber: 2,
				PerPage:    2,
				TotalPages: 2,
				TotalCount: 4,
			}
		}
		json.NewEncoder(w).Encode(resp)
	})

	client := newTestRPCClient(handler)
	ctx := context.Background()
	accounts, err := client.AccountsByHeight(ctx, 100)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, accounts, 4)
	// Verify all accounts are collected (order may vary due to parallel fetch)
	addressMap := make(map[string]uint64)
	for _, acc := range accounts {
		addressMap[acc.Address] = acc.Amount
	}
	assert.Equal(t, uint64(100), addressMap["0x001"])
	assert.Equal(t, uint64(200), addressMap["0x002"])
	assert.Equal(t, uint64(300), addressMap["0x003"])
	assert.Equal(t, uint64(400), addressMap["0x004"])
}

// TestAccountsByHeight_EmptyResponse tests when no accounts exist at height
func TestAccountsByHeight_EmptyResponse(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			Results    []*rpc.Account `json:"results"`
			PageNumber int            `json:"pageNumber"`
			PerPage    int            `json:"perPage"`
			TotalPages int            `json:"totalPages"`
			TotalCount int            `json:"totalCount"`
		}{
			Results:    []*rpc.Account{},
			PageNumber: 1,
			PerPage:    1000,
			TotalPages: 1,
			TotalCount: 0,
		}
		json.NewEncoder(w).Encode(resp)
	})

	client := newTestRPCClient(handler)
	ctx := context.Background()
	accounts, err := client.AccountsByHeight(ctx, 100)

	// Assert
	assert.NoError(t, err)
	assert.Empty(t, accounts)
}

// TestAccountsByHeight_NetworkError tests handling of network failures
func TestAccountsByHeight_NetworkError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("RPC error"))
	})

	client := newTestRPCClient(handler)
	ctx := context.Background()
	accounts, err := client.AccountsByHeight(ctx, 100)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "server 500")
	assert.Nil(t, accounts)
}

// TestAccountsByHeight_PaginationError tests error in fetching subsequent pages
func TestAccountsByHeight_PaginationError(t *testing.T) {
	requestCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		pageNum := 1
		if raw, ok := req["pageNumber"]; ok {
			if v, ok := raw.(float64); ok {
				pageNum = int(v)
			}
		}

		if pageNum == 1 {
			// First page succeeds
			resp := struct {
				Results    []*rpc.Account `json:"results"`
				PageNumber int            `json:"pageNumber"`
				PerPage    int            `json:"perPage"`
				TotalPages int            `json:"totalPages"`
				TotalCount int            `json:"totalCount"`
			}{
				Results:    []*rpc.Account{{Address: "0x001", Amount: 100}},
				PageNumber: 1,
				PerPage:    1,
				TotalPages: 2,
				TotalCount: 2,
			}
			json.NewEncoder(w).Encode(resp)
		} else {
			// Second page fails
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	client := newTestRPCClient(handler)
	ctx := context.Background()
	accounts, err := client.AccountsByHeight(ctx, 100)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "server 500")
	assert.Nil(t, accounts)
}

// TestGetGenesisState_Success tests successful genesis state fetching
func TestGetGenesisState_Success(t *testing.T) {
	// Mock genesis data
	mockGenesis := &rpc.GenesisState{
		Time: 1234567890,
		Accounts: []*rpc.Account{
			{Address: "0xgenesis1", Amount: 1000000},
			{Address: "0xgenesis2", Amount: 2000000},
		},
		Validators: []interface{}{},
		Pools:      []interface{}{},
		Params:     nil,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "/v1/query/state", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		// Parse request body
		var req map[string]any
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Verify height is 0 for genesis
		assert.Equal(t, float64(0), req["height"])

		// Return mock genesis
		json.NewEncoder(w).Encode(mockGenesis)
	})

	client := newTestRPCClient(handler)
	ctx := context.Background()
	genesis, err := client.GetGenesisState(ctx, 0)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, genesis)
	assert.Equal(t, uint64(1234567890), genesis.Time)
	assert.Len(t, genesis.Accounts, 2)
	assert.Equal(t, "0xgenesis1", genesis.Accounts[0].Address)
	assert.Equal(t, uint64(1000000), genesis.Accounts[0].Amount)
}

// TestGetGenesisState_ParseError tests handling of invalid response format
func TestGetGenesisState_ParseError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("invalid json{"))
	})

	client := newTestRPCClient(handler)
	ctx := context.Background()
	genesis, err := client.GetGenesisState(ctx, 0)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fetch genesis state at height 0")
	assert.Nil(t, genesis)
}

// TestGetGenesisState_NetworkError tests handling of network failures
func TestGetGenesisState_NetworkError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("Service temporarily unavailable"))
	})

	client := newTestRPCClient(handler)
	ctx := context.Background()
	genesis, err := client.GetGenesisState(ctx, 0)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fetch genesis state")
	assert.Nil(t, genesis)
}

// Benchmark test for AccountsByHeight
func BenchmarkAccountsByHeight(b *testing.B) {
	// Create mock server with large response
	accounts := make([]*rpc.Account, 1000)
	for i := 0; i < 1000; i++ {
		accounts[i] = &rpc.Account{
			Address: "0x" + string(rune(i)),
			Amount:  uint64(i * 100),
		}
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			Results    []*rpc.Account `json:"results"`
			PageNumber int            `json:"pageNumber"`
			PerPage    int            `json:"perPage"`
			TotalPages int            `json:"totalPages"`
			TotalCount int            `json:"totalCount"`
		}{
			Results:    accounts,
			PageNumber: 1,
			PerPage:    1000,
			TotalPages: 1,
			TotalCount: 1000,
		}
		json.NewEncoder(w).Encode(resp)
	})

	client := newTestRPCClient(handler)
	ctx := context.Background()

	// Run benchmark
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.AccountsByHeight(ctx, 100)
		if err != nil {
			b.Fatal(err)
		}
	}
}

package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	admintypes "github.com/canopy-network/canopyx/app/admin/controller/types"
	"github.com/canopy-network/canopyx/app/admin/types"
	"github.com/canopy-network/canopyx/pkg/db/chain"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/gorilla/mux"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockChainStore is a mock implementation of chain.Store for testing
type mockChainStore struct {
	chain.Store
	dbName     string
	selectFunc func(ctx context.Context, dest interface{}, query string, args ...any) error
}

func (m *mockChainStore) DatabaseName() string {
	return m.dbName
}

func (m *mockChainStore) Select(ctx context.Context, dest interface{}, query string, args ...any) error {
	if m.selectFunc != nil {
		return m.selectFunc(ctx, dest, query, args...)
	}
	return nil
}

func (m *mockChainStore) ChainKey() string {
	return "1"
}

func (m *mockChainStore) Close() error {
	return nil
}

// setupTestController creates a test controller with mock dependencies
func setupTestController(t *testing.T) *Controller {
	logger, _ := zap.NewDevelopment()
	app := &types.App{
		Logger:   logger,
		ChainsDB: xsync.NewMap[string, chain.Store](),
	}

	return &Controller{
		App:       app,
		JWTSecret: []byte("test-secret"),
	}
}

func TestParseEntityQueryRequest(t *testing.T) {
	c := setupTestController(t)

	tests := []struct {
		name        string
		queryParams map[string]string
		want        admintypes.EntityQueryRequest
		wantErr     bool
		errContains string
	}{
		{
			name:        "default values",
			queryParams: map[string]string{},
			want: admintypes.EntityQueryRequest{
				Limit:      defaultQueryLimit,
				Cursor:     0,
				SortDesc:   true,
				UseStaging: false,
			},
			wantErr: false,
		},
		{
			name: "valid custom values",
			queryParams: map[string]string{
				"limit":       "100",
				"cursor":      "500",
				"sort":        "asc",
				"use_staging": "true",
			},
			want: admintypes.EntityQueryRequest{
				Limit:      100,
				Cursor:     500,
				SortDesc:   false,
				UseStaging: true,
			},
			wantErr: false,
		},
		{
			name: "limit exceeds max",
			queryParams: map[string]string{
				"limit": "2000",
			},
			want: admintypes.EntityQueryRequest{
				Limit:      maxQueryLimit,
				Cursor:     0,
				SortDesc:   true,
				UseStaging: false,
			},
			wantErr: false,
		},
		{
			name: "invalid limit",
			queryParams: map[string]string{
				"limit": "abc",
			},
			wantErr:     true,
			errContains: "invalid limit",
		},
		{
			name: "negative limit",
			queryParams: map[string]string{
				"limit": "-10",
			},
			wantErr:     true,
			errContains: "must be positive",
		},
		{
			name: "invalid cursor",
			queryParams: map[string]string{
				"cursor": "abc",
			},
			wantErr:     true,
			errContains: "invalid cursor",
		},
		{
			name: "invalid sort order",
			queryParams: map[string]string{
				"sort": "invalid",
			},
			wantErr:     true,
			errContains: "invalid sort order",
		},
		{
			name: "sort desc",
			queryParams: map[string]string{
				"sort": "desc",
			},
			want: admintypes.EntityQueryRequest{
				Limit:      defaultQueryLimit,
				Cursor:     0,
				SortDesc:   true,
				UseStaging: false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request with query parameters
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			q := req.URL.Query()
			for k, v := range tt.queryParams {
				q.Add(k, v)
			}
			req.URL.RawQuery = q.Encode()

			got, err := c.parseEntityQueryRequest(req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseEntityGetRequest(t *testing.T) {
	t.Skip("TODO: Fix test - add property/value query parameters to all test cases")
	c := setupTestController(t)

	tests := []struct {
		name        string
		queryParams map[string]string
		want        admintypes.EntityGetRequest
		wantErr     bool
		errContains string
	}{
		{
			name:        "default values",
			queryParams: map[string]string{},
			want: admintypes.EntityGetRequest{
				Height:     nil,
				UseStaging: false,
			},
			wantErr: false,
		},
		{
			name: "with height",
			queryParams: map[string]string{
				"height": "123",
			},
			want: admintypes.EntityGetRequest{
				Height:     uintPtr(123),
				UseStaging: false,
			},
			wantErr: false,
		},
		{
			name: "with staging",
			queryParams: map[string]string{
				"use_staging": "true",
			},
			want: admintypes.EntityGetRequest{
				Height:     nil,
				UseStaging: true,
			},
			wantErr: false,
		},
		{
			name: "invalid height",
			queryParams: map[string]string{
				"height": "abc",
			},
			wantErr:     true,
			errContains: "invalid height",
		},
		{
			name: "invalid use_staging",
			queryParams: map[string]string{
				"use_staging": "invalid",
			},
			wantErr:     true,
			errContains: "invalid use_staging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request with query parameters
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			q := req.URL.Query()
			for k, v := range tt.queryParams {
				q.Add(k, v)
			}
			req.URL.RawQuery = q.Encode()

			got, err := c.parseEntityGetRequest(req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestHandleEntityQuery(t *testing.T) {
	t.Skip("TODO: Fix test - update selectFunc to use type switches with typed models")
	tests := []struct {
		name           string
		chainID        string
		entity         string
		queryParams    map[string]string
		setupStore     func() *mockChainStore
		wantStatusCode int
		checkResponse  func(t *testing.T, resp *httptest.ResponseRecorder)
	}{
		{
			name:    "successful query",
			chainID: "1",
			entity:  "blocks",
			queryParams: map[string]string{
				"limit": "10",
			},
			setupStore: func() *mockChainStore {
				return &mockChainStore{
					dbName: "chain_1",
					selectFunc: func(ctx context.Context, dest interface{}, query string, args ...any) error {
						// Simulate returning 5 results
						results := dest.(*[]map[string]interface{})
						*results = []map[string]interface{}{
							{"height": uint64(100)},
							{"height": uint64(99)},
							{"height": uint64(98)},
							{"height": uint64(97)},
							{"height": uint64(96)},
						}
						return nil
					},
				}
			},
			wantStatusCode: http.StatusOK,
			checkResponse: func(t *testing.T, resp *httptest.ResponseRecorder) {
				var response admintypes.EntityQueryResponse
				err := json.NewDecoder(resp.Body).Decode(&response)
				require.NoError(t, err)
				assert.Equal(t, 10, response.Limit)
				assert.Len(t, response.Data, 5)
				assert.Nil(t, response.NextCursor)
			},
		},
		{
			name:    "query with cursor and more results",
			chainID: "1",
			entity:  "blocks",
			queryParams: map[string]string{
				"limit":  "5",
				"cursor": "100",
			},
			setupStore: func() *mockChainStore {
				return &mockChainStore{
					dbName: "chain_1",
					selectFunc: func(ctx context.Context, dest interface{}, query string, args ...any) error {
						// Simulate returning limit+1 results to indicate more data
						results := dest.(*[]map[string]interface{})
						*results = []map[string]interface{}{
							{"height": uint64(99)},
							{"height": uint64(98)},
							{"height": uint64(97)},
							{"height": uint64(96)},
							{"height": uint64(95)},
							{"height": uint64(94)}, // Extra result
						}
						return nil
					},
				}
			},
			wantStatusCode: http.StatusOK,
			checkResponse: func(t *testing.T, resp *httptest.ResponseRecorder) {
				var response admintypes.EntityQueryResponse
				err := json.NewDecoder(resp.Body).Decode(&response)
				require.NoError(t, err)
				assert.Equal(t, 5, response.Limit)
				assert.Len(t, response.Data, 5) // Should trim the extra result
				assert.NotNil(t, response.NextCursor)
				assert.Equal(t, uint64(95), *response.NextCursor)
			},
		},
		{
			name:    "invalid entity",
			chainID: "1",
			entity:  "invalid_entity",
			queryParams: map[string]string{
				"limit": "10",
			},
			setupStore:     func() *mockChainStore { return nil },
			wantStatusCode: http.StatusBadRequest,
			checkResponse: func(t *testing.T, resp *httptest.ResponseRecorder) {
				var response map[string]string
				err := json.NewDecoder(resp.Body).Decode(&response)
				require.NoError(t, err)
				assert.Contains(t, response["error"], "invalid entity")
			},
		},
		{
			name:    "invalid limit parameter",
			chainID: "1",
			entity:  "blocks",
			queryParams: map[string]string{
				"limit": "abc",
			},
			setupStore:     func() *mockChainStore { return nil },
			wantStatusCode: http.StatusBadRequest,
			checkResponse: func(t *testing.T, resp *httptest.ResponseRecorder) {
				var response map[string]string
				err := json.NewDecoder(resp.Body).Decode(&response)
				require.NoError(t, err)
				assert.Contains(t, response["error"], "invalid request parameters")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup controller
			c := setupTestController(t)

			// Setup mock store if needed
			if tt.setupStore != nil {
				store := tt.setupStore()
				if store != nil {
					c.App.ChainsDB.Store(tt.chainID, store)
				}
			}

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			q := req.URL.Query()
			for k, v := range tt.queryParams {
				q.Add(k, v)
			}
			req.URL.RawQuery = q.Encode()

			// Add path variables
			req = mux.SetURLVars(req, map[string]string{
				"id":     tt.chainID,
				"entity": tt.entity,
			})

			// Create response recorder
			resp := httptest.NewRecorder()

			// Call handler
			c.HandleEntityQuery(resp, req)

			// Check status code
			assert.Equal(t, tt.wantStatusCode, resp.Code)

			// Check response body
			if tt.checkResponse != nil {
				tt.checkResponse(t, resp)
			}
		})
	}
}

func TestHandleEntityGet(t *testing.T) {
	tests := []struct {
		name           string
		chainID        string
		entity         string
		idValue        string
		queryParams    map[string]string
		setupStore     func() *mockChainStore
		wantStatusCode int
		checkResponse  func(t *testing.T, resp *httptest.ResponseRecorder)
	}{
		{
			name:    "get by height",
			chainID: "1",
			entity:  "blocks",
			idValue: "100",
			queryParams: map[string]string{
				"property": "height",
				"value":    "100",
			},
			setupStore: func() *mockChainStore {
				return &mockChainStore{
					dbName: "chain_1",
					selectFunc: func(ctx context.Context, dest interface{}, query string, args ...any) error {
						switch v := dest.(type) {
						case *[]indexermodels.Block:
							*v = []indexermodels.Block{
								{Height: 100, Hash: "abc123"},
							}
						}
						return nil
					},
				}
			},
			wantStatusCode: http.StatusOK,
			checkResponse: func(t *testing.T, resp *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&response)
				require.NoError(t, err)
				assert.Equal(t, float64(100), response["height"]) // JSON numbers are float64
				assert.Equal(t, "abc123", response["hash"])
			},
		},
		{
			name:    "get by address",
			chainID: "1",
			entity:  "accounts",
			idValue: "canopy1abc123",
			queryParams: map[string]string{
				"property": "address",
				"value":    "canopy1abc123",
			},
			setupStore: func() *mockChainStore {
				return &mockChainStore{
					dbName: "chain_1",
					selectFunc: func(ctx context.Context, dest interface{}, query string, args ...any) error {
						switch v := dest.(type) {
						case *[]indexermodels.Account:
							*v = []indexermodels.Account{
								{Address: "canopy1abc123", Amount: 1000},
							}
						}
						return nil
					},
				}
			},
			wantStatusCode: http.StatusOK,
			checkResponse: func(t *testing.T, resp *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&response)
				require.NoError(t, err)
				assert.Equal(t, "canopy1abc123", response["address"])
			},
		},
		{
			name:    "entity not found",
			chainID: "1",
			entity:  "blocks",
			idValue: "999999",
			queryParams: map[string]string{
				"property": "height",
				"value":    "999999",
			},
			setupStore: func() *mockChainStore {
				return &mockChainStore{
					dbName: "chain_1",
					selectFunc: func(ctx context.Context, dest interface{}, query string, args ...any) error {
						switch v := dest.(type) {
						case *[]indexermodels.Block:
							*v = []indexermodels.Block{} // Empty result
						}
						return nil
					},
				}
			},
			wantStatusCode: http.StatusNotFound,
			checkResponse: func(t *testing.T, resp *httptest.ResponseRecorder) {
				var response map[string]string
				err := json.NewDecoder(resp.Body).Decode(&response)
				require.NoError(t, err)
				assert.Contains(t, response["error"], "entity not found")
			},
		},
		{
			name:    "invalid entity",
			chainID: "1",
			entity:  "invalid_entity",
			idValue: "100",
			queryParams: map[string]string{
				"property": "height",
				"value":    "100",
			},
			setupStore: func() *mockChainStore {
				return &mockChainStore{dbName: "chain_1"}
			},
			wantStatusCode: http.StatusBadRequest,
			checkResponse: func(t *testing.T, resp *httptest.ResponseRecorder) {
				var response map[string]string
				err := json.NewDecoder(resp.Body).Decode(&response)
				require.NoError(t, err)
				assert.Contains(t, response["error"], "invalid entity")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup controller
			c := setupTestController(t)

			// Setup mock store if needed
			if tt.setupStore != nil {
				store := tt.setupStore()
				if store != nil {
					c.App.ChainsDB.Store(tt.chainID, store)
				}
			}

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			q := req.URL.Query()
			for k, v := range tt.queryParams {
				q.Add(k, v)
			}
			req.URL.RawQuery = q.Encode()

			// Add path variables
			req = mux.SetURLVars(req, map[string]string{
				"id":       tt.chainID,
				"entity":   tt.entity,
				"id_value": tt.idValue,
			})

			// Execute request
			resp := httptest.NewRecorder()
			c.HandleEntityGet(resp, req)

			// Check status code
			assert.Equal(t, tt.wantStatusCode, resp.Code)

			// Additional response checks
			if tt.checkResponse != nil {
				tt.checkResponse(t, resp)
			}
		})
	}
}

// uintPtr is a helper function to create a pointer to a uint64
func uintPtr(v uint64) *uint64 {
	return &v
}

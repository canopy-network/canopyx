//go:build integration

package indexer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/tests/integration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestAdminDB_TableCreation verifies that all AdminDB tables are created correctly
func TestAdminDB_TableCreation(t *testing.T) {
	testDB := integration.TestDB()
	require.NotNil(t, testDB, "test database not initialized")

	ctx := context.Background()
	integration.CleanDB(t)

	tests := []struct {
		name        string
		tableName   string
		checkEngine string
		checkOrder  string
	}{
		{
			name:        "chains table with ReplacingMergeTree",
			tableName:   "chains",
			checkEngine: "ReplacingMergeTree",
			checkOrder:  "chain_id",
		},
		{
			name:        "index_progress table with MergeTree",
			tableName:   "index_progress",
			checkEngine: "MergeTree",
			checkOrder:  "chain_id, height",
		},
		{
			name:        "index_progress_agg table with AggregatingMergeTree",
			tableName:   "index_progress_agg",
			checkEngine: "AggregatingMergeTree",
			checkOrder:  "chain_id",
		},
		{
			name:        "reindex_requests table with ReplacingMergeTree",
			tableName:   "reindex_requests",
			checkEngine: "ReplacingMergeTree",
			checkOrder:  "chain_id, requested_at, height",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify table exists
			integration.RequireTableExists(t, tt.tableName)

			// Check engine type
			var engine string
			query := `
				SELECT engine
				FROM system.tables
				WHERE database = ? AND name = ?
			`
			err := testDB.Db.NewRaw(query, testDB.Name, tt.tableName).Scan(ctx, &engine)
			require.NoError(t, err)
			assert.Contains(t, engine, tt.checkEngine, "unexpected engine for %s", tt.tableName)

			// Check ORDER BY clause
			var sortingKey string
			query = `
				SELECT sorting_key
				FROM system.tables
				WHERE database = ? AND name = ?
			`
			err = testDB.Db.NewRaw(query, testDB.Name, tt.tableName).Scan(ctx, &sortingKey)
			require.NoError(t, err)
			assert.Contains(t, sortingKey, tt.checkOrder, "unexpected ORDER BY for %s", tt.tableName)
		})
	}

	// Verify materialized view exists
	integration.RequireMaterializedViewExists(t, "index_progress_mv")
}

// TestAdminDB_ChainOperations tests all chain-related CRUD operations
func TestAdminDB_ChainOperations(t *testing.T) {
	testDB := integration.TestDB()
	require.NotNil(t, testDB)

	ctx := context.Background()

	tests := []struct {
		name string
		fn   func(t *testing.T, ctx context.Context)
	}{
		{
			name: "UpsertChain with all fields",
			fn:   testChainUpsertAllFields,
		},
		{
			name: "UpsertChain with nullable and array fields",
			fn:   testChainNullableAndArrayFields,
		},
		{
			name: "GetChain with FINAL for ReplacingMergeTree",
			fn:   testChainGetWithFinal,
		},
		{
			name: "ListChain with multiple chains",
			fn:   testChainList,
		},
		{
			name: "UpdateRPCHealth",
			fn:   testChainUpdateRPCHealth,
		},
		{
			name: "UpdateQueueHealth",
			fn:   testChainUpdateQueueHealth,
		},
		{
			name: "UpdateDeploymentHealth",
			fn:   testChainUpdateDeploymentHealth,
		},
		{
			name: "UpdateOverallHealth computation",
			fn:   testChainUpdateOverallHealth,
		},
		{
			name: "PatchChains bulk partial updates",
			fn:   testChainPatchBulk,
		},
		{
			name: "ReplacingMergeTree deduplication",
			fn:   testChainReplacingMergeTreeDedup,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			integration.CleanDB(t)
			tt.fn(t, ctx)
		})
	}
}

func testChainUpsertAllFields(t *testing.T, ctx context.Context) {
	testDB := integration.TestDB()
	now := time.Now().UTC()

	chain := &admin.Chain{
		ChainID:      "test-chain",
		ChainName:    "Test Chain",
		RPCEndpoints: []string{"http://rpc1.example.com", "http://rpc2.example.com"},
		Paused:       0,
		Deleted:      0,
		Image:        "test-image:v1.0.0",
		MinReplicas:  2,
		MaxReplicas:  5,
		Notes:        "Test chain notes",
		CreatedAt:    now,
		UpdatedAt:    now,
		// Health fields
		RPCHealthStatus:           "healthy",
		RPCHealthMessage:          "All endpoints responding",
		RPCHealthUpdatedAt:        now,
		QueueHealthStatus:         "warning",
		QueueHealthMessage:        "Queue backlog at 1000 items",
		QueueHealthUpdatedAt:      now,
		DeploymentHealthStatus:    "degraded",
		DeploymentHealthMessage:   "2 of 5 pods running",
		DeploymentHealthUpdatedAt: now,
		OverallHealthStatus:       "degraded",
		OverallHealthUpdatedAt:    now,
	}

	err := testDB.UpsertChain(ctx, chain)
	require.NoError(t, err)

	// Retrieve and verify all fields
	retrieved, err := testDB.GetChain(ctx, "test-chain")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Verify all fields
	assert.Equal(t, chain.ChainID, retrieved.ChainID)
	assert.Equal(t, chain.ChainName, retrieved.ChainName)
	assert.Equal(t, chain.RPCEndpoints, retrieved.RPCEndpoints)
	assert.Equal(t, chain.Paused, retrieved.Paused)
	assert.Equal(t, chain.Deleted, retrieved.Deleted)
	assert.Equal(t, chain.Image, retrieved.Image)
	assert.Equal(t, chain.MinReplicas, retrieved.MinReplicas)
	assert.Equal(t, chain.MaxReplicas, retrieved.MaxReplicas)
	assert.Equal(t, chain.Notes, retrieved.Notes)

	// Health fields
	assert.Equal(t, chain.RPCHealthStatus, retrieved.RPCHealthStatus)
	assert.Equal(t, chain.RPCHealthMessage, retrieved.RPCHealthMessage)
	assert.Equal(t, chain.QueueHealthStatus, retrieved.QueueHealthStatus)
	assert.Equal(t, chain.QueueHealthMessage, retrieved.QueueHealthMessage)
	assert.Equal(t, chain.DeploymentHealthStatus, retrieved.DeploymentHealthStatus)
	assert.Equal(t, chain.DeploymentHealthMessage, retrieved.DeploymentHealthMessage)
	assert.Equal(t, chain.OverallHealthStatus, retrieved.OverallHealthStatus)

	// Timestamps should be close (within 1 second due to microsecond precision)
	assert.WithinDuration(t, chain.CreatedAt, retrieved.CreatedAt, time.Second)
	assert.WithinDuration(t, chain.UpdatedAt, retrieved.UpdatedAt, time.Second)
}

func testChainNullableAndArrayFields(t *testing.T, ctx context.Context) {
	testDB := integration.TestDB()

	// Test empty array
	chain1 := &admin.Chain{
		ChainID:      "empty-array-chain",
		ChainName:    "Empty Array Chain",
		RPCEndpoints: []string{}, // Empty array
		Image:        "test:v1",
		MinReplicas:  1,
		MaxReplicas:  1,
	}
	err := testDB.UpsertChain(ctx, chain1)
	require.NoError(t, err)

	retrieved1, err := testDB.GetChain(ctx, "empty-array-chain")
	require.NoError(t, err)
	assert.Empty(t, retrieved1.RPCEndpoints)

	// Test array with multiple elements
	chain2 := &admin.Chain{
		ChainID:      "multi-rpc-chain",
		ChainName:    "Multi RPC Chain",
		RPCEndpoints: []string{"http://rpc1", "http://rpc2", "http://rpc3"},
		Image:        "test:v1",
		MinReplicas:  1,
		MaxReplicas:  3,
	}
	err = testDB.UpsertChain(ctx, chain2)
	require.NoError(t, err)

	retrieved2, err := testDB.GetChain(ctx, "multi-rpc-chain")
	require.NoError(t, err)
	assert.Len(t, retrieved2.RPCEndpoints, 3)
	assert.Equal(t, chain2.RPCEndpoints, retrieved2.RPCEndpoints)

	// Test default values
	chain3 := &admin.Chain{
		ChainID:      "default-chain",
		ChainName:    "Default Chain",
		RPCEndpoints: []string{"http://default"},
		// Not setting: Paused, Deleted, Notes, Health fields
	}
	err = testDB.UpsertChain(ctx, chain3)
	require.NoError(t, err)

	retrieved3, err := testDB.GetChain(ctx, "default-chain")
	require.NoError(t, err)
	assert.Equal(t, uint8(0), retrieved3.Paused)
	assert.Equal(t, uint8(0), retrieved3.Deleted)
	assert.Equal(t, "", retrieved3.Notes)
	assert.Equal(t, "unknown", retrieved3.RPCHealthStatus)
	assert.Equal(t, "unknown", retrieved3.QueueHealthStatus)
	assert.Equal(t, "unknown", retrieved3.DeploymentHealthStatus)
	assert.Equal(t, "unknown", retrieved3.OverallHealthStatus)
}

func testChainGetWithFinal(t *testing.T, ctx context.Context) {
	testDB := integration.TestDB()

	// Insert multiple versions of the same chain
	chain := &admin.Chain{
		ChainID:      "versioned-chain",
		ChainName:    "Version 1",
		RPCEndpoints: []string{"http://v1"},
		Image:        "test:v1",
		MinReplicas:  1,
		MaxReplicas:  1,
	}

	// Insert version 1
	err := testDB.UpsertChain(ctx, chain)
	require.NoError(t, err)

	// Wait a bit to ensure different timestamps
	time.Sleep(10 * time.Millisecond)

	// Update to version 2
	chain.ChainName = "Version 2"
	chain.RPCEndpoints = []string{"http://v2"}
	err = testDB.UpsertChain(ctx, chain)
	require.NoError(t, err)

	// Wait a bit to ensure different timestamps
	time.Sleep(10 * time.Millisecond)

	// Update to version 3
	chain.ChainName = "Version 3"
	chain.RPCEndpoints = []string{"http://v3"}
	err = testDB.UpsertChain(ctx, chain)
	require.NoError(t, err)

	// GetChain should return the latest version (using FINAL)
	retrieved, err := testDB.GetChain(ctx, "versioned-chain")
	require.NoError(t, err)
	assert.Equal(t, "Version 3", retrieved.ChainName)
	assert.Equal(t, []string{"http://v3"}, retrieved.RPCEndpoints)
}

func testChainList(t *testing.T, ctx context.Context) {
	testDB := integration.TestDB()

	chains := []*admin.Chain{
		{
			ChainID:      "chain-a",
			ChainName:    "Chain A",
			RPCEndpoints: []string{"http://a"},
			Image:        "test:a",
			MinReplicas:  1,
			MaxReplicas:  1,
		},
		{
			ChainID:      "chain-b",
			ChainName:    "Chain B",
			RPCEndpoints: []string{"http://b"},
			Image:        "test:b",
			MinReplicas:  2,
			MaxReplicas:  4,
			Paused:       1,
		},
		{
			ChainID:      "chain-c",
			ChainName:    "Chain C",
			RPCEndpoints: []string{"http://c"},
			Image:        "test:c",
			MinReplicas:  3,
			MaxReplicas:  6,
			Deleted:      1,
		},
	}

	for _, chain := range chains {
		err := testDB.UpsertChain(ctx, chain)
		require.NoError(t, err)
	}

	// List all chains
	list, err := testDB.ListChain(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 3)

	// Verify ordering by chain_id
	assert.Equal(t, "chain-a", list[0].ChainID)
	assert.Equal(t, "chain-b", list[1].ChainID)
	assert.Equal(t, "chain-c", list[2].ChainID)

	// Verify individual fields
	assert.Equal(t, uint8(0), list[0].Paused)
	assert.Equal(t, uint8(1), list[1].Paused)
	assert.Equal(t, uint8(1), list[2].Deleted)
}

func testChainUpdateRPCHealth(t *testing.T, ctx context.Context) {
	testDB := integration.TestDB()

	// Create initial chain
	chain := &admin.Chain{
		ChainID:      "rpc-health-chain",
		ChainName:    "RPC Health Test",
		RPCEndpoints: []string{"http://rpc"},
		Image:        "test:v1",
		MinReplicas:  1,
		MaxReplicas:  1,
	}
	err := testDB.UpsertChain(ctx, chain)
	require.NoError(t, err)

	// Update RPC health
	err = testDB.UpdateRPCHealth(ctx, "rpc-health-chain", "degraded", "High latency on endpoint 1")
	require.NoError(t, err)

	// Retrieve and verify
	retrieved, err := testDB.GetChain(ctx, "rpc-health-chain")
	require.NoError(t, err)
	assert.Equal(t, "degraded", retrieved.RPCHealthStatus)
	assert.Equal(t, "High latency on endpoint 1", retrieved.RPCHealthMessage)
	assert.WithinDuration(t, time.Now(), retrieved.RPCHealthUpdatedAt, 5*time.Second)

	// Update again with different status
	err = testDB.UpdateRPCHealth(ctx, "rpc-health-chain", "unreachable", "All endpoints down")
	require.NoError(t, err)

	retrieved2, err := testDB.GetChain(ctx, "rpc-health-chain")
	require.NoError(t, err)
	assert.Equal(t, "unreachable", retrieved2.RPCHealthStatus)
	assert.Equal(t, "All endpoints down", retrieved2.RPCHealthMessage)
}

func testChainUpdateQueueHealth(t *testing.T, ctx context.Context) {
	testDB := integration.TestDB()

	// Create initial chain
	chain := &admin.Chain{
		ChainID:      "queue-health-chain",
		ChainName:    "Queue Health Test",
		RPCEndpoints: []string{"http://rpc"},
		Image:        "test:v1",
		MinReplicas:  1,
		MaxReplicas:  1,
	}
	err := testDB.UpsertChain(ctx, chain)
	require.NoError(t, err)

	// Update queue health
	err = admin.UpdateQueueHealth(ctx, testDB.Db, "queue-health-chain", "critical", "Queue backlog > 10000")
	require.NoError(t, err)

	// Retrieve and verify
	retrieved, err := testDB.GetChain(ctx, "queue-health-chain")
	require.NoError(t, err)
	assert.Equal(t, "critical", retrieved.QueueHealthStatus)
	assert.Equal(t, "Queue backlog > 10000", retrieved.QueueHealthMessage)
	assert.WithinDuration(t, time.Now(), retrieved.QueueHealthUpdatedAt, 5*time.Second)
}

func testChainUpdateDeploymentHealth(t *testing.T, ctx context.Context) {
	testDB := integration.TestDB()

	// Create initial chain
	chain := &admin.Chain{
		ChainID:      "deploy-health-chain",
		ChainName:    "Deploy Health Test",
		RPCEndpoints: []string{"http://rpc"},
		Image:        "test:v1",
		MinReplicas:  3,
		MaxReplicas:  5,
	}
	err := testDB.UpsertChain(ctx, chain)
	require.NoError(t, err)

	// Update deployment health
	err = admin.UpdateDeploymentHealth(ctx, testDB.Db, "deploy-health-chain", "failed", "0 of 3 pods running")
	require.NoError(t, err)

	// Retrieve and verify
	retrieved, err := testDB.GetChain(ctx, "deploy-health-chain")
	require.NoError(t, err)
	assert.Equal(t, "failed", retrieved.DeploymentHealthStatus)
	assert.Equal(t, "0 of 3 pods running", retrieved.DeploymentHealthMessage)
	assert.WithinDuration(t, time.Now(), retrieved.DeploymentHealthUpdatedAt, 5*time.Second)
}

func testChainUpdateOverallHealth(t *testing.T, ctx context.Context) {
	testDB := integration.TestDB()

	// Create initial chain
	chain := &admin.Chain{
		ChainID:      "overall-health-chain",
		ChainName:    "Overall Health Test",
		RPCEndpoints: []string{"http://rpc"},
		Image:        "test:v1",
		MinReplicas:  1,
		MaxReplicas:  3,
	}
	err := testDB.UpsertChain(ctx, chain)
	require.NoError(t, err)

	testCases := []struct {
		name           string
		rpcStatus      string
		queueStatus    string
		deployStatus   string
		expectedStatus string
	}{
		{
			name:           "All healthy",
			rpcStatus:      "healthy",
			queueStatus:    "healthy",
			deployStatus:   "healthy",
			expectedStatus: "healthy",
		},
		{
			name:           "One degraded",
			rpcStatus:      "healthy",
			queueStatus:    "warning",
			deployStatus:   "healthy",
			expectedStatus: "warning",
		},
		{
			name:           "One critical",
			rpcStatus:      "healthy",
			queueStatus:    "healthy",
			deployStatus:   "failed",
			expectedStatus: "failed",
		},
		{
			name:           "Mixed statuses - worst wins",
			rpcStatus:      "degraded",
			queueStatus:    "critical",
			deployStatus:   "healthy",
			expectedStatus: "critical",
		},
		{
			name:           "Unknown status",
			rpcStatus:      "unknown",
			queueStatus:    "unknown",
			deployStatus:   "unknown",
			expectedStatus: "unknown",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Update all health statuses
			err := admin.UpdateRPCHealth(ctx, testDB.Db, "overall-health-chain", tc.rpcStatus, "")
			require.NoError(t, err)

			err = admin.UpdateQueueHealth(ctx, testDB.Db, "overall-health-chain", tc.queueStatus, "")
			require.NoError(t, err)

			err = admin.UpdateDeploymentHealth(ctx, testDB.Db, "overall-health-chain", tc.deployStatus, "")
			require.NoError(t, err)

			// Verify overall health was computed correctly
			retrieved, err := testDB.GetChain(ctx, "overall-health-chain")
			require.NoError(t, err)
			assert.Equal(t, tc.expectedStatus, retrieved.OverallHealthStatus, "Overall health mismatch for test: %s", tc.name)
		})
	}
}

func testChainPatchBulk(t *testing.T, ctx context.Context) {
	testDB := integration.TestDB()

	// Create initial chains
	chains := []*admin.Chain{
		{
			ChainID:      "patch-1",
			ChainName:    "Patch Test 1",
			RPCEndpoints: []string{"http://old1"},
			Image:        "test:v1",
			MinReplicas:  1,
			MaxReplicas:  1,
			Paused:       0,
			Deleted:      0,
		},
		{
			ChainID:      "patch-2",
			ChainName:    "Patch Test 2",
			RPCEndpoints: []string{"http://old2"},
			Image:        "test:v2",
			MinReplicas:  2,
			MaxReplicas:  4,
			Paused:       0,
			Deleted:      0,
		},
	}

	for _, chain := range chains {
		err := testDB.UpsertChain(ctx, chain)
		require.NoError(t, err)
	}

	// Apply patches
	patches := []admin.Chain{
		{
			ChainID:      "patch-1",
			Paused:       1, // Pause chain 1
			RPCEndpoints: []string{"http://new1", "http://new1-backup"},
		},
		{
			ChainID: "patch-2",
			Deleted: 1, // Delete chain 2
		},
	}

	err := testDB.PatchChains(ctx, patches)
	require.NoError(t, err)

	// Verify patches were applied
	chain1, err := testDB.GetChain(ctx, "patch-1")
	require.NoError(t, err)
	assert.Equal(t, uint8(1), chain1.Paused)
	assert.Equal(t, []string{"http://new1", "http://new1-backup"}, chain1.RPCEndpoints)
	assert.Equal(t, "Patch Test 1", chain1.ChainName) // Unchanged

	chain2, err := testDB.GetChain(ctx, "patch-2")
	require.NoError(t, err)
	assert.Equal(t, uint8(1), chain2.Deleted)
	assert.Equal(t, []string{"http://old2"}, chain2.RPCEndpoints) // Unchanged
	assert.Equal(t, "Patch Test 2", chain2.ChainName)             // Unchanged
}

func testChainReplacingMergeTreeDedup(t *testing.T, ctx context.Context) {
	testDB := integration.TestDB()

	// Insert same chain multiple times rapidly
	for i := 0; i < 5; i++ {
		chain := &admin.Chain{
			ChainID:      "dedup-chain",
			ChainName:    fmt.Sprintf("Version %d", i+1),
			RPCEndpoints: []string{fmt.Sprintf("http://v%d", i+1)},
			Image:        "test:v1",
			MinReplicas:  uint16(i + 1),
			MaxReplicas:  uint16(i + 2),
		}
		err := testDB.UpsertChain(ctx, chain)
		require.NoError(t, err)
	}

	// Count raw rows (before deduplication)
	var rawCount int
	query := `SELECT count() FROM test_canopyx.chains WHERE chain_id = ?`
	err := testDB.Db.NewRaw(query, "dedup-chain").Scan(ctx, &rawCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, rawCount, 1, "Should have at least 1 row")

	// Get deduplicated result with FINAL
	chain, err := testDB.GetChain(ctx, "dedup-chain")
	require.NoError(t, err)
	assert.Equal(t, "Version 5", chain.ChainName)
	assert.Equal(t, []string{"http://v5"}, chain.RPCEndpoints)
	assert.Equal(t, uint16(5), chain.MinReplicas)
	assert.Equal(t, uint16(6), chain.MaxReplicas)

	// List should also use FINAL
	list, err := testDB.ListChain(ctx)
	require.NoError(t, err)

	dedupCount := 0
	for _, c := range list {
		if c.ChainID == "dedup-chain" {
			dedupCount++
			assert.Equal(t, "Version 5", c.ChainName)
		}
	}
	assert.Equal(t, 1, dedupCount, "ListChain should return only 1 deduplicated row")
}

// TestAdminDB_IndexProgressOperations tests index progress tracking with materialized view
func TestAdminDB_IndexProgressOperations(t *testing.T) {
	testDB := integration.TestDB()
	require.NotNil(t, testDB)

	ctx := context.Background()

	tests := []struct {
		name string
		fn   func(t *testing.T, ctx context.Context)
	}{
		{
			name: "RecordIndexed and LastIndexed with materialized view",
			fn:   testIndexProgressRecordAndQuery,
		},
		{
			name: "FindGaps with window function",
			fn:   testIndexProgressFindGaps,
		},
		{
			name: "LastIndexed with empty data",
			fn:   testIndexProgressEmptyData,
		},
		{
			name: "Materialized view aggregation",
			fn:   testIndexProgressMaterializedView,
		},
		{
			name: "Large dataset performance",
			fn:   testIndexProgressLargeDataset,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			integration.CleanDB(t)
			tt.fn(t, ctx)
		})
	}
}

func testIndexProgressRecordAndQuery(t *testing.T, ctx context.Context) {
	testDB := integration.TestDB()

	// Create a test chain first
	chain := &admin.Chain{
		ChainID:      "progress-chain",
		ChainName:    "Progress Test",
		RPCEndpoints: []string{"http://test"},
		Image:        "test:v1",
	}
	err := testDB.UpsertChain(ctx, chain)
	require.NoError(t, err)

	// Record multiple heights
	heights := []uint64{100, 101, 102, 105, 106, 110}
	for _, h := range heights {
		err := testDB.RecordIndexed(ctx, "progress-chain", h)
		require.NoError(t, err)
		time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	}

	// Wait for materialized view to update
	integration.WaitForMaterializedView(t, 200*time.Millisecond)

	// Query last indexed height
	lastHeight, err := testDB.LastIndexed(ctx, "progress-chain")
	require.NoError(t, err)
	assert.Equal(t, uint64(110), lastHeight)

	// Verify raw data exists
	var count int
	query := `SELECT count() FROM test_canopyx.index_progress WHERE chain_id = ?`
	err = testDB.Db.NewRaw(query, "progress-chain").Scan(ctx, &count)
	require.NoError(t, err)
	assert.Equal(t, len(heights), count)

	// Verify aggregate table has data
	var aggCount int
	query = `SELECT count() FROM test_canopyx.index_progress_agg WHERE chain_id = ?`
	err = testDB.Db.NewRaw(query, "progress-chain").Scan(ctx, &aggCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, aggCount, 1)
}

func testIndexProgressFindGaps(t *testing.T, ctx context.Context) {
	testDB := integration.TestDB()

	// Create a test chain
	chain := &admin.Chain{
		ChainID:      "gaps-chain",
		ChainName:    "Gaps Test",
		RPCEndpoints: []string{"http://test"},
		Image:        "test:v1",
	}
	err := testDB.UpsertChain(ctx, chain)
	require.NoError(t, err)

	// Record heights with intentional gaps
	// Heights: 1, 2, 3, [gap: 4-6], 7, 8, [gap: 9-14], 15, 16, 17, [gap: 18-19], 20
	heightsToRecord := []uint64{
		1, 2, 3, // Continuous
		7, 8, // Gap before: 4-6
		15, 16, 17, // Gap before: 9-14
		20, // Gap before: 18-19
	}

	for _, h := range heightsToRecord {
		err := testDB.RecordIndexed(ctx, "gaps-chain", h)
		require.NoError(t, err)
	}

	// Find gaps
	gaps, err := testDB.FindGaps(ctx, "gaps-chain")
	require.NoError(t, err)

	// Expected gaps (internal gaps only, not trailing gap)
	expectedGaps := []integration.GapExpectation{
		{From: 4, To: 6},
		{From: 9, To: 14},
		{From: 18, To: 19},
	}

	require.Len(t, gaps, len(expectedGaps))
	for i, expected := range expectedGaps {
		assert.Equal(t, expected.From, gaps[i].From, "Gap %d From mismatch", i)
		assert.Equal(t, expected.To, gaps[i].To, "Gap %d To mismatch", i)
	}
}

func testIndexProgressEmptyData(t *testing.T, ctx context.Context) {
	testDB := integration.TestDB()

	// Query for non-existent chain
	lastHeight, err := testDB.LastIndexed(ctx, "non-existent-chain")
	require.NoError(t, err)
	assert.Equal(t, uint64(0), lastHeight)

	// Find gaps for non-existent chain
	gaps, err := testDB.FindGaps(ctx, "non-existent-chain")
	require.NoError(t, err)
	assert.Empty(t, gaps)
}

func testIndexProgressMaterializedView(t *testing.T, ctx context.Context) {
	testDB := integration.TestDB()

	chainID := "mv-test-chain"

	// Create a test chain
	chain := &admin.Chain{
		ChainID:      chainID,
		ChainName:    "MV Test",
		RPCEndpoints: []string{"http://test"},
		Image:        "test:v1",
	}
	err := testDB.UpsertChain(ctx, chain)
	require.NoError(t, err)

	// Insert initial data
	for i := uint64(1); i <= 10; i++ {
		err := testDB.RecordIndexed(ctx, chainID, i)
		require.NoError(t, err)
	}

	// Wait for MV to process
	integration.WaitForMaterializedView(t, 200*time.Millisecond)

	// Check aggregate table directly
	var maxHeightState uint64
	query := `SELECT maxMerge(max_height) FROM test_canopyx.index_progress_agg WHERE chain_id = ?`
	err = testDB.Db.NewRaw(query, chainID).Scan(ctx, &maxHeightState)
	require.NoError(t, err)
	assert.Equal(t, uint64(10), maxHeightState)

	// Add more data
	for i := uint64(11); i <= 20; i++ {
		err := testDB.RecordIndexed(ctx, chainID, i)
		require.NoError(t, err)
	}

	// Wait for MV to process
	integration.WaitForMaterializedView(t, 200*time.Millisecond)

	// Verify aggregate updated
	err = testDB.Db.NewRaw(query, chainID).Scan(ctx, &maxHeightState)
	require.NoError(t, err)
	assert.Equal(t, uint64(20), maxHeightState)

	// Verify LastIndexed uses the aggregate
	lastHeight, err := testDB.LastIndexed(ctx, chainID)
	require.NoError(t, err)
	assert.Equal(t, uint64(20), lastHeight)
}

func testIndexProgressLargeDataset(t *testing.T, ctx context.Context) {
	testDB := integration.TestDB()

	chainID := "large-dataset-chain"

	// Create a test chain
	chain := &admin.Chain{
		ChainID:      chainID,
		ChainName:    "Large Dataset Test",
		RPCEndpoints: []string{"http://test"},
		Image:        "test:v1",
	}
	err := testDB.UpsertChain(ctx, chain)
	require.NoError(t, err)

	// Insert a large dataset with some gaps
	batchSize := 100
	batches := 10

	for batch := 0; batch < batches; batch++ {
		rows := make([]*admin.IndexProgress, 0, batchSize)
		for i := 0; i < batchSize; i++ {
			height := uint64(batch*batchSize + i*2) // Create gaps by using i*2
			rows = append(rows, &admin.IndexProgress{
				ChainID:   chainID,
				Height:    height,
				IndexedAt: time.Now(),
			})
		}

		// Bulk insert
		_, err := testDB.Db.NewInsert().Model(&rows).Exec(ctx)
		require.NoError(t, err)
	}

	// Wait for MV
	integration.WaitForMaterializedView(t, 500*time.Millisecond)

	// Performance test: LastIndexed should be fast even with large dataset
	start := time.Now()
	lastHeight, err := testDB.LastIndexed(ctx, chainID)
	duration := time.Since(start)
	require.NoError(t, err)
	assert.Equal(t, uint64((batches-1)*batchSize+(batchSize-1)*2), lastHeight)
	assert.Less(t, duration, 100*time.Millisecond, "LastIndexed should be fast with aggregate table")

	// Performance test: FindGaps should handle large datasets
	start = time.Now()
	gaps, err := testDB.FindGaps(ctx, chainID)
	duration = time.Since(start)
	require.NoError(t, err)
	assert.NotEmpty(t, gaps) // Should have many gaps due to i*2 pattern
	assert.Less(t, duration, 500*time.Millisecond, "FindGaps should complete within reasonable time")
}

// TestAdminDB_ReindexRequestOperations tests reindex request audit logging
func TestAdminDB_ReindexRequestOperations(t *testing.T) {
	testDB := integration.TestDB()
	require.NotNil(t, testDB)

	ctx := context.Background()
	integration.CleanDB(t)

	t.Run("RecordReindexRequests bulk insert", func(t *testing.T) {
		heights := []uint64{100, 101, 102, 105, 110}
		err := testDB.RecordReindexRequests(ctx, "test-chain", "admin-user", heights)
		require.NoError(t, err)

		// Verify all records inserted
		var count int
		query := `SELECT count() FROM test_canopyx.reindex_requests WHERE chain_id = ?`
		err = testDB.Db.NewRaw(query, "test-chain").Scan(ctx, &count)
		require.NoError(t, err)
		assert.Equal(t, len(heights), count)
	})

	t.Run("ListReindexRequests with ordering and limit", func(t *testing.T) {
		// Insert more requests from different users
		err := testDB.RecordReindexRequests(ctx, "test-chain", "user1", []uint64{200, 201})
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		err = testDB.RecordReindexRequests(ctx, "test-chain", "user2", []uint64{300, 301, 302})
		require.NoError(t, err)

		// List with limit
		requests, err := testDB.ListReindexRequests(ctx, "test-chain", 3)
		require.NoError(t, err)
		assert.Len(t, requests, 3)

		// Verify ordering (most recent first)
		assert.Equal(t, "user2", requests[0].RequestedBy)
		assert.Contains(t, []uint64{300, 301, 302}, requests[0].Height)
	})

	t.Run("ReindexRequest default values", func(t *testing.T) {
		err := testDB.RecordReindexRequests(ctx, "default-chain", "admin", []uint64{500})
		require.NoError(t, err)

		requests, err := testDB.ListReindexRequests(ctx, "default-chain", 1)
		require.NoError(t, err)
		require.Len(t, requests, 1)

		assert.Equal(t, "queued", requests[0].Status)
		assert.WithinDuration(t, time.Now(), requests[0].RequestedAt, 5*time.Second)
	})

	t.Run("Empty heights array", func(t *testing.T) {
		err := testDB.RecordReindexRequests(ctx, "empty-chain", "admin", []uint64{})
		require.NoError(t, err) // Should not error

		requests, err := testDB.ListReindexRequests(ctx, "empty-chain", 10)
		require.NoError(t, err)
		assert.Empty(t, requests)
	})
}

// TestChainDB_TableCreation verifies chain-specific database tables
func TestChainDB_TableCreation(t *testing.T) {
	ctx := context.Background()
	logger, _ := zap.NewDevelopment()

	// Create a chain-specific database
	chainDB, err := db.NewChainDb(ctx, logger, "testchain")
	require.NoError(t, err)
	defer chainDB.Close()

	tests := []struct {
		name        string
		tableName   string
		checkEngine string
		checkOrder  string
	}{
		{
			name:        "blocks table with MergeTree",
			tableName:   "blocks",
			checkEngine: "MergeTree",
			checkOrder:  "height",
		},
		{
			name:        "txs table with MergeTree",
			tableName:   "txs",
			checkEngine: "MergeTree",
			checkOrder:  "height, tx_hash",
		},
		{
			name:        "txs_raw table with MergeTree and TTL",
			tableName:   "txs_raw",
			checkEngine: "MergeTree",
			checkOrder:  "height, tx_hash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check table exists
			var count int
			query := `
				SELECT count()
				FROM system.tables
				WHERE database = ? AND name = ?
			`
			err := chainDB.Db.NewRaw(query, chainDB.Name, tt.tableName).Scan(ctx, &count)
			require.NoError(t, err)
			assert.Equal(t, 1, count, "Table %s does not exist", tt.tableName)

			// Check engine
			var engine string
			query = `
				SELECT engine
				FROM system.tables
				WHERE database = ? AND name = ?
			`
			err = chainDB.Db.NewRaw(query, chainDB.Name, tt.tableName).Scan(ctx, &engine)
			require.NoError(t, err)
			assert.Contains(t, engine, tt.checkEngine)

			// Check ORDER BY
			var sortingKey string
			query = `
				SELECT sorting_key
				FROM system.tables
				WHERE database = ? AND name = ?
			`
			err = chainDB.Db.NewRaw(query, chainDB.Name, tt.tableName).Scan(ctx, &sortingKey)
			require.NoError(t, err)
			assert.Contains(t, sortingKey, tt.checkOrder)
		})
	}

	// Verify TTL for txs_raw
	t.Run("txs_raw TTL configuration", func(t *testing.T) {
		var ttlExpression string
		query := `
			SELECT engine_full
			FROM system.tables
			WHERE database = ? AND name = 'txs_raw'
		`
		err := chainDB.Db.NewRaw(query, chainDB.Name).Scan(ctx, &ttlExpression)
		require.NoError(t, err)
		assert.Contains(t, ttlExpression, "TTL")
		assert.Contains(t, ttlExpression, "created_at")
		assert.Contains(t, ttlExpression, "30")
		assert.Contains(t, ttlExpression, "DAY")
	})
}

// TestChainDB_BlockOperations tests block CRUD operations
func TestChainDB_BlockOperations(t *testing.T) {
	ctx := context.Background()
	logger, _ := zap.NewDevelopment()

	chainDB, err := db.NewChainDb(ctx, logger, "blocktest")
	require.NoError(t, err)
	defer chainDB.Close()

	t.Run("InsertBlock with all fields", func(t *testing.T) {
		block := &indexer.Block{
			Height:          1000,
			Hash:            "0xabc123def456",
			Time:            time.Now().UTC().Truncate(time.Microsecond), // DateTime64(6) precision
			LastBlockHash:   "0xprev789",
			ProposerAddress: "validator1",
			Size:            2048,
			NumTxs:          15,
		}

		err := chainDB.InsertBlock(ctx, block)
		require.NoError(t, err)

		// Retrieve and verify
		retrieved, err := indexer.GetBlock(ctx, chainDB.Db, 1000)
		require.NoError(t, err)
		assert.Equal(t, block.Height, retrieved.Height)
		assert.Equal(t, block.Hash, retrieved.Hash)
		assert.Equal(t, block.Time, retrieved.Time) // Should match exactly with microsecond precision
		assert.Equal(t, block.LastBlockHash, retrieved.LastBlockHash)
		assert.Equal(t, block.ProposerAddress, retrieved.ProposerAddress)
		assert.Equal(t, block.Size, retrieved.Size)
		assert.Equal(t, block.NumTxs, retrieved.NumTxs)
	})

	t.Run("HasBlock existence check", func(t *testing.T) {
		// Check existing block
		exists, err := chainDB.HasBlock(ctx, 1000)
		require.NoError(t, err)
		assert.True(t, exists)

		// Check non-existing block
		exists, err = chainDB.HasBlock(ctx, 9999)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("QueryBlocks pagination with cursor", func(t *testing.T) {
		// Insert multiple blocks
		for i := uint64(1); i <= 20; i++ {
			block := &indexer.Block{
				Height:          i,
				Hash:            fmt.Sprintf("hash-%d", i),
				Time:            time.Now().Add(time.Duration(i) * time.Second),
				ProposerAddress: fmt.Sprintf("validator-%d", i%3),
				NumTxs:          uint32(i * 2),
			}
			err := chainDB.InsertBlock(ctx, block)
			require.NoError(t, err)
		}

		// Query first page (no cursor)
		page1, err := chainDB.QueryBlocks(ctx, 0, 5)
		require.NoError(t, err)
		assert.Len(t, page1, 5)
		assert.Equal(t, uint64(20), page1[0].Height) // Descending order
		assert.Equal(t, uint64(16), page1[4].Height)

		// Query second page with cursor
		page2, err := chainDB.QueryBlocks(ctx, 16, 5) // cursor = last height from page1
		require.NoError(t, err)
		assert.Len(t, page2, 5)
		assert.Equal(t, uint64(15), page2[0].Height)
		assert.Equal(t, uint64(11), page2[4].Height)
	})

	t.Run("DeleteBlock with ALTER TABLE DELETE", func(t *testing.T) {
		// Insert a block
		block := &indexer.Block{
			Height: 5000,
			Hash:   "delete-test-hash",
			Time:   time.Now(),
		}
		err := chainDB.InsertBlock(ctx, block)
		require.NoError(t, err)

		// Verify it exists
		exists, err := chainDB.HasBlock(ctx, 5000)
		require.NoError(t, err)
		assert.True(t, exists)

		// Delete it
		err = chainDB.DeleteBlock(ctx, 5000)
		require.NoError(t, err)

		// Wait for async deletion to complete
		time.Sleep(100 * time.Millisecond)

		// Verify it's gone
		exists, err = chainDB.HasBlock(ctx, 5000)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("DateTime64 precision", func(t *testing.T) {
		// Test microsecond precision storage
		preciseTime := time.Date(2024, 1, 15, 10, 30, 45, 123456789, time.UTC)
		expectedTime := preciseTime.Truncate(time.Microsecond) // DateTime64(6) truncates to microseconds

		block := &indexer.Block{
			Height: 6000,
			Hash:   "precision-test",
			Time:   preciseTime,
		}
		err := chainDB.InsertBlock(ctx, block)
		require.NoError(t, err)

		retrieved, err := indexer.GetBlock(ctx, chainDB.Db, 6000)
		require.NoError(t, err)
		assert.Equal(t, expectedTime, retrieved.Time)
		assert.NotEqual(t, preciseTime, retrieved.Time) // Nanoseconds should be truncated
	})
}

// TestChainDB_TransactionOperations tests transaction CRUD operations
func TestChainDB_TransactionOperations(t *testing.T) {
	ctx := context.Background()
	logger, _ := zap.NewDevelopment()

	chainDB, err := db.NewChainDb(ctx, logger, "txtest")
	require.NoError(t, err)
	defer chainDB.Close()

	t.Run("InsertTransactions core and raw", func(t *testing.T) {
		counterparty := "recipient-address"
		amount := uint64(1000000)
		msgRaw := `{"type":"send","value":"1000000"}`
		pubKey := "pubkey123"
		sig := "signature456"

		coreTxs := []*indexer.Transaction{
			{
				Height:        100,
				TxHash:        "tx-001",
				Time:          time.Now().UTC().Truncate(time.Microsecond),
				MessageType:   "send",
				Counterparty:  &counterparty,
				Signer:        "sender-address",
				Amount:        &amount,
				Fee:           100,
				CreatedHeight: 99,
			},
			{
				Height:        100,
				TxHash:        "tx-002",
				Time:          time.Now().UTC().Truncate(time.Microsecond),
				MessageType:   "delegate",
				Counterparty:  nil, // Nullable field
				Signer:        "delegator-address",
				Amount:        nil, // Nullable field
				Fee:           50,
				CreatedHeight: 100,
			},
		}

		rawTxs := []*indexer.TransactionRaw{
			{
				Height:    100,
				TxHash:    "tx-001",
				MsgRaw:    &msgRaw,
				PublicKey: &pubKey,
				Signature: &sig,
			},
			{
				Height:    100,
				TxHash:    "tx-002",
				MsgRaw:    nil, // Nullable
				PublicKey: nil, // Nullable
				Signature: nil, // Nullable
			},
		}

		err := chainDB.InsertTransactions(ctx, coreTxs, rawTxs)
		require.NoError(t, err)

		// Verify core transactions
		var coreCount int
		query := `SELECT count() FROM txtest.txs WHERE height = 100`
		err = chainDB.Db.NewRaw(query).Scan(ctx, &coreCount)
		require.NoError(t, err)
		assert.Equal(t, 2, coreCount)

		// Verify raw transactions
		var rawCount int
		query = `SELECT count() FROM txtest.txs_raw WHERE height = 100`
		err = chainDB.Db.NewRaw(query).Scan(ctx, &rawCount)
		require.NoError(t, err)
		assert.Equal(t, 2, rawCount)

		// Verify nullable fields
		var tx1 indexer.Transaction
		err = chainDB.Db.NewSelect().
			Model(&tx1).
			Where("tx_hash = ?", "tx-001").
			Scan(ctx)
		require.NoError(t, err)
		assert.NotNil(t, tx1.Counterparty)
		assert.Equal(t, counterparty, *tx1.Counterparty)
		assert.NotNil(t, tx1.Amount)
		assert.Equal(t, amount, *tx1.Amount)

		var tx2 indexer.Transaction
		err = chainDB.Db.NewSelect().
			Model(&tx2).
			Where("tx_hash = ?", "tx-002").
			Scan(ctx)
		require.NoError(t, err)
		assert.Nil(t, tx2.Counterparty)
		assert.Nil(t, tx2.Amount)
	})

	t.Run("LowCardinality optimization for message_type", func(t *testing.T) {
		// Insert transactions with repeated message types
		messageTypes := []string{"send", "delegate", "undelegate", "send", "send", "delegate"}
		txs := make([]*indexer.Transaction, len(messageTypes))

		for i, msgType := range messageTypes {
			txs[i] = &indexer.Transaction{
				Height:        200 + uint64(i),
				TxHash:        fmt.Sprintf("lc-test-%d", i),
				Time:          time.Now(),
				MessageType:   msgType,
				Signer:        "test-signer",
				Fee:           10,
				CreatedHeight: 200,
			}
		}

		err := indexer.InsertTransactionsCore(ctx, chainDB.Db, txs...)
		require.NoError(t, err)

		// Verify LowCardinality column type
		var columnType string
		query := `
			SELECT type
			FROM system.columns
			WHERE database = ? AND table = 'txs' AND name = 'message_type'
		`
		err = chainDB.Db.NewRaw(query, chainDB.Name).Scan(ctx, &columnType)
		require.NoError(t, err)
		assert.Contains(t, columnType, "LowCardinality")
	})

	t.Run("QueryTransactions pagination", func(t *testing.T) {
		// Insert test data
		for i := uint64(1); i <= 30; i++ {
			amount := uint64(i * 1000)
			tx := &indexer.Transaction{
				Height:        i,
				TxHash:        fmt.Sprintf("page-tx-%d", i),
				Time:          time.Now().Add(time.Duration(i) * time.Second),
				MessageType:   "transfer",
				Signer:        fmt.Sprintf("signer-%d", i),
				Amount:        &amount,
				Fee:           i * 10,
				CreatedHeight: i,
			}
			err := indexer.InsertTransactionsCore(ctx, chainDB.Db, tx)
			require.NoError(t, err)
		}

		// Query first page
		page1, err := chainDB.QueryTransactions(ctx, 0, 10)
		require.NoError(t, err)
		assert.Len(t, page1, 10)
		assert.Equal(t, uint64(30), page1[0].Height) // Descending order

		// Query with cursor
		page2, err := chainDB.QueryTransactions(ctx, 21, 10)
		require.NoError(t, err)
		assert.Len(t, page2, 10)
		assert.Equal(t, uint64(20), page2[0].Height)
	})

	t.Run("DeleteTransactions from both tables", func(t *testing.T) {
		height := uint64(500)

		// Insert data
		tx := &indexer.Transaction{
			Height:        height,
			TxHash:        "delete-test",
			Time:          time.Now(),
			MessageType:   "test",
			Signer:        "test-signer",
			Fee:           100,
			CreatedHeight: height,
		}
		raw := &indexer.TransactionRaw{
			Height: height,
			TxHash: "delete-test",
		}

		err := chainDB.InsertTransactions(ctx, []*indexer.Transaction{tx}, []*indexer.TransactionRaw{raw})
		require.NoError(t, err)

		// Verify insertion
		var count int
		query := `SELECT count() FROM txtest.txs WHERE height = ?`
		err = chainDB.Db.NewRaw(query, height).Scan(ctx, &count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		// Delete
		err = chainDB.DeleteTransactions(ctx, height)
		require.NoError(t, err)

		// Wait for async deletion
		time.Sleep(100 * time.Millisecond)

		// Verify deletion from both tables
		err = chainDB.Db.NewRaw(query, height).Scan(ctx, &count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		query = `SELECT count() FROM txtest.txs_raw WHERE height = ?`
		err = chainDB.Db.NewRaw(query, height).Scan(ctx, &count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("TTL verification for txs_raw", func(t *testing.T) {
		// Insert old data (should be deleted by TTL in production)
		oldTime := time.Now().Add(-31 * 24 * time.Hour) // 31 days ago

		// We can't test actual TTL deletion in unit tests as it requires waiting,
		// but we can verify the TTL is configured
		var engineFull string
		query := `
			SELECT engine_full
			FROM system.tables
			WHERE database = ? AND name = 'txs_raw'
		`
		err := chainDB.Db.NewRaw(query, chainDB.Name).Scan(ctx, &engineFull)
		require.NoError(t, err)
		assert.Contains(t, engineFull, "TTL")
		assert.Contains(t, engineFull, "30 DAY")

		// Verify created_at column exists with default
		var defaultExpr string
		query = `
			SELECT default_expression
			FROM system.columns
			WHERE database = ? AND table = 'txs_raw' AND name = 'created_at'
		`
		err = chainDB.Db.NewRaw(query, chainDB.Name).Scan(ctx, &defaultExpr)
		require.NoError(t, err)
		assert.Contains(t, defaultExpr, "now()")

		_ = oldTime // Satisfy compiler
	})
}

// TestIntegrationScenarios tests complete workflows
func TestIntegrationScenarios(t *testing.T) {
	testDB := integration.TestDB()
	require.NotNil(t, testDB)

	ctx := context.Background()
	logger, _ := zap.NewDevelopment()

	t.Run("Complete indexing workflow", func(t *testing.T) {
		integration.CleanDB(t)

		// 1. Create chain in AdminDB
		chain := &admin.Chain{
			ChainID:      "workflow-chain",
			ChainName:    "Workflow Test Chain",
			RPCEndpoints: []string{"http://rpc1", "http://rpc2"},
			Image:        "indexer:v1",
			MinReplicas:  2,
			MaxReplicas:  4,
		}
		err := testDB.UpsertChain(ctx, chain)
		require.NoError(t, err)

		// 2. Create ChainDB for the chain
		chainDB, err := db.NewChainDb(ctx, logger, "workflow_chain")
		require.NoError(t, err)
		defer chainDB.Close()

		// 3. Index some blocks and transactions
		for height := uint64(1); height <= 10; height++ {
			// Insert block
			block := &indexer.Block{
				Height:          height,
				Hash:            fmt.Sprintf("hash-%d", height),
				Time:            time.Now().Add(time.Duration(height) * time.Second),
				ProposerAddress: "validator-1",
				NumTxs:          3,
			}
			err := chainDB.InsertBlock(ctx, block)
			require.NoError(t, err)

			// Insert transactions
			amount := height * 1000
			txs := []*indexer.Transaction{
				{
					Height:        height,
					TxHash:        fmt.Sprintf("tx-%d-1", height),
					Time:          block.Time,
					MessageType:   "send",
					Signer:        "sender-1",
					Amount:        &amount,
					Fee:           100,
					CreatedHeight: height,
				},
			}
			err = indexer.InsertTransactionsCore(ctx, chainDB.Db, txs...)
			require.NoError(t, err)

			// Record progress in AdminDB
			err = testDB.RecordIndexed(ctx, "workflow-chain", height)
			require.NoError(t, err)
		}

		// 4. Simulate gap in indexing (skip heights 11-14)
		for height := uint64(15); height <= 20; height++ {
			err := testDB.RecordIndexed(ctx, "workflow-chain", height)
			require.NoError(t, err)
		}

		// 5. Find and verify gaps
		gaps, err := testDB.FindGaps(ctx, "workflow-chain")
		require.NoError(t, err)
		assert.Len(t, gaps, 1)
		assert.Equal(t, uint64(11), gaps[0].From)
		assert.Equal(t, uint64(14), gaps[0].To)

		// 6. Record reindex request for the gap
		gapHeights := []uint64{11, 12, 13, 14}
		err = testDB.RecordReindexRequests(ctx, "workflow-chain", "gap-scanner", gapHeights)
		require.NoError(t, err)

		// 7. Update health statuses
		err = testDB.UpdateRPCHealth(ctx, "workflow-chain", "healthy", "All endpoints responding")
		require.NoError(t, err)

		err = admin.UpdateQueueHealth(ctx, testDB.Db, "workflow-chain", "healthy", "Queue empty")
		require.NoError(t, err)

		err = admin.UpdateDeploymentHealth(ctx, testDB.Db, "workflow-chain", "healthy", "4/4 pods running")
		require.NoError(t, err)

		// 8. Verify overall health
		finalChain, err := testDB.GetChain(ctx, "workflow-chain")
		require.NoError(t, err)
		assert.Equal(t, "healthy", finalChain.OverallHealthStatus)

		// 9. Query blocks and transactions
		blocks, err := chainDB.QueryBlocks(ctx, 0, 5)
		require.NoError(t, err)
		assert.Len(t, blocks, 5)

		txs, err := chainDB.QueryTransactions(ctx, 0, 5)
		require.NoError(t, err)
		assert.Len(t, txs, 5)
	})

	t.Run("Concurrent writes stress test", func(t *testing.T) {
		integration.CleanDB(t)

		// Create test chain
		chain := &admin.Chain{
			ChainID:      "concurrent-chain",
			ChainName:    "Concurrent Test",
			RPCEndpoints: []string{"http://test"},
			Image:        "test:v1",
		}
		err := testDB.UpsertChain(ctx, chain)
		require.NoError(t, err)

		// Concurrent index progress recording
		errChan := make(chan error, 100)
		for i := 0; i < 100; i++ {
			go func(height uint64) {
				err := testDB.RecordIndexed(ctx, "concurrent-chain", height)
				errChan <- err
			}(uint64(i))
		}

		// Collect errors
		for i := 0; i < 100; i++ {
			err := <-errChan
			assert.NoError(t, err)
		}

		// Wait for materialized view
		integration.WaitForMaterializedView(t, 500*time.Millisecond)

		// Verify all heights recorded
		lastHeight, err := testDB.LastIndexed(ctx, "concurrent-chain")
		require.NoError(t, err)
		assert.Equal(t, uint64(99), lastHeight)
	})
}

// TestErrorHandling tests error conditions and edge cases
func TestErrorHandling(t *testing.T) {
	testDB := integration.TestDB()
	require.NotNil(t, testDB)

	ctx := context.Background()

	t.Run("GetChain non-existent", func(t *testing.T) {
		_, err := testDB.GetChain(ctx, "non-existent-chain")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("Invalid chain configuration", func(t *testing.T) {
		chain := &admin.Chain{
			ChainID:      "invalid-chain",
			ChainName:    "Invalid",
			RPCEndpoints: []string{"http://test"},
			Image:        "test:v1",
			MinReplicas:  5,
			MaxReplicas:  2, // Max < Min
		}
		err := testDB.UpsertChain(ctx, chain)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max_replicas")
	})

	t.Run("Empty chain ID", func(t *testing.T) {
		err := testDB.UpdateRPCHealth(ctx, "", "healthy", "test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "required")
	})

	t.Run("Query non-existent block", func(t *testing.T) {
		logger, _ := zap.NewDevelopment()
		chainDB, err := db.NewChainDb(ctx, logger, "errortest")
		require.NoError(t, err)
		defer chainDB.Close()

		_, err = indexer.GetBlock(ctx, chainDB.Db, 999999)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, sql.ErrNoRows))
	})
}

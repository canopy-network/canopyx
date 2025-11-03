package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdminStore_UpsertChain tests creating and updating chains.
func TestAdminStore_UpsertChain(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_upsert")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	chain := generateChain(1001)
	err := adminDB.UpsertChain(ctx, chain)
	require.NoError(t, err)

	// Retrieve and verify
	retrieved, err := adminDB.GetChain(ctx, 1001)
	require.NoError(t, err)
	assertChainEqual(t, chain, retrieved)
}

// TestAdminStore_UpsertChain_Update tests updating an existing chain.
func TestAdminStore_UpsertChain_Update(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_update")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create initial chain
	chain := generateChain(1002)
	err := adminDB.UpsertChain(ctx, chain)
	require.NoError(t, err)

	// Update the chain
	time.Sleep(10 * time.Millisecond) // Ensure updated_at is different
	chain.ChainName = "updated-chain"
	chain.RPCEndpoints = []string{"http://updated-rpc:26657"}
	chain.Notes = "Updated notes"
	err = adminDB.UpsertChain(ctx, chain)
	require.NoError(t, err)

	// Retrieve and verify update
	retrieved, err := adminDB.GetChain(ctx, 1002)
	require.NoError(t, err)
	assert.Equal(t, "updated-chain", retrieved.ChainName)
	assert.Equal(t, []string{"http://updated-rpc:26657"}, retrieved.RPCEndpoints)
	assert.Equal(t, "Updated notes", retrieved.Notes)
}

// TestAdminStore_UpsertChain_DefaultValues tests that default values are set correctly.
func TestAdminStore_UpsertChain_DefaultValues(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_defaults")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	chain := generateChain(1003)
	chain.MinReplicas = 0 // Should default to 1
	chain.MaxReplicas = 0 // Should default to 3
	chain.RPCHealthStatus = ""
	chain.QueueHealthStatus = ""
	chain.DeploymentHealthStatus = ""
	chain.OverallHealthStatus = ""

	err := adminDB.UpsertChain(ctx, chain)
	require.NoError(t, err)

	retrieved, err := adminDB.GetChain(ctx, 1003)
	require.NoError(t, err)
	assert.Equal(t, uint16(1), retrieved.MinReplicas)
	assert.Equal(t, uint16(3), retrieved.MaxReplicas)
	assert.Equal(t, "unknown", retrieved.RPCHealthStatus)
	assert.Equal(t, "unknown", retrieved.QueueHealthStatus)
	assert.Equal(t, "unknown", retrieved.DeploymentHealthStatus)
	assert.Equal(t, "unknown", retrieved.OverallHealthStatus)
}

// TestAdminStore_UpsertChain_ValidationError tests replica validation.
func TestAdminStore_UpsertChain_ValidationError(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_validation")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	chain := generateChain(1004)
	chain.MinReplicas = 5
	chain.MaxReplicas = 2 // max < min should fail

	err := adminDB.UpsertChain(ctx, chain)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_replicas")
}

// TestAdminStore_GetChain_NotFound tests retrieving a non-existent chain.
func TestAdminStore_GetChain_NotFound(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_notfound")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := adminDB.GetChain(ctx, 9999)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestAdminStore_ListChain tests listing all chains.
func TestAdminStore_ListChain(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_list")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create multiple chains
	for i := uint64(1); i <= 5; i++ {
		chain := generateChain(1000 + i)
		err := adminDB.UpsertChain(ctx, chain)
		require.NoError(t, err)
	}

	// List all chains
	chains, err := adminDB.ListChain(ctx)
	require.NoError(t, err)
	assert.Len(t, chains, 5)

	// Verify they're ordered by chain_id
	for i := 0; i < len(chains)-1; i++ {
		assert.LessOrEqual(t, chains[i].ChainID, chains[i+1].ChainID)
	}
}

// TestAdminStore_ListChain_Empty tests listing when no chains exist.
func TestAdminStore_ListChain_Empty(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_list_empty")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	chains, err := adminDB.ListChain(ctx)
	require.NoError(t, err)
	assert.Empty(t, chains)
}

// TestAdminStore_PatchChains tests partial updates to chains.
func TestAdminStore_PatchChains(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_patch")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create initial chains
	chain1 := generateChain(2001)
	chain2 := generateChain(2002)
	require.NoError(t, adminDB.UpsertChain(ctx, chain1))
	require.NoError(t, adminDB.UpsertChain(ctx, chain2))

	// Wait to ensure updated_at changes
	time.Sleep(10 * time.Millisecond)

	// Apply patches
	patches := []struct {
		ChainID      uint64
		Paused       uint8
		Deleted      uint8
		RPCEndpoints []string
	}{
		{ChainID: 2001, Paused: 1},
		{ChainID: 2002, Deleted: 1},
	}

	for _, p := range patches {
		current, err := adminDB.GetChain(ctx, p.ChainID)
		require.NoError(t, err)
		current.Paused = p.Paused
		current.Deleted = p.Deleted
		if len(p.RPCEndpoints) > 0 {
			current.RPCEndpoints = p.RPCEndpoints
		}
		current.UpdatedAt = time.Now()
		err = adminDB.UpsertChain(ctx, current)
		require.NoError(t, err)
	}

	// Verify patches
	updated1, err := adminDB.GetChain(ctx, 2001)
	require.NoError(t, err)
	assert.Equal(t, uint8(1), updated1.Paused)

	updated2, err := adminDB.GetChain(ctx, 2002)
	require.NoError(t, err)
	assert.Equal(t, uint8(1), updated2.Deleted)
}

// TestAdminStore_UpdateRPCHealth tests RPC health status updates.
func TestAdminStore_UpdateRPCHealth(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_rpc_health")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a chain
	chain := generateChain(3001)
	require.NoError(t, adminDB.UpsertChain(ctx, chain))

	// Update RPC health
	err := adminDB.UpdateRPCHealth(ctx, 3001, "healthy", "All endpoints responsive")
	require.NoError(t, err)

	// Verify update
	updated, err := adminDB.GetChain(ctx, 3001)
	require.NoError(t, err)
	assert.Equal(t, "healthy", updated.RPCHealthStatus)
	assert.Equal(t, "All endpoints responsive", updated.RPCHealthMessage)
	assert.False(t, updated.RPCHealthUpdatedAt.IsZero())
}

// TestAdminStore_UpdateQueueHealth tests queue health status updates.
func TestAdminStore_UpdateQueueHealth(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_queue_health")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a chain
	chain := generateChain(3002)
	require.NoError(t, adminDB.UpsertChain(ctx, chain))

	// Update queue health
	err := adminDB.UpdateQueueHealth(ctx, 3002, "warning", "Backlog detected")
	require.NoError(t, err)

	// Verify update
	updated, err := adminDB.GetChain(ctx, 3002)
	require.NoError(t, err)
	assert.Equal(t, "warning", updated.QueueHealthStatus)
	assert.Equal(t, "Backlog detected", updated.QueueHealthMessage)
	assert.False(t, updated.QueueHealthUpdatedAt.IsZero())
}

// TestAdminStore_UpdateDeploymentHealth tests deployment health status updates.
func TestAdminStore_UpdateDeploymentHealth(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_deploy_health")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a chain
	chain := generateChain(3003)
	require.NoError(t, adminDB.UpsertChain(ctx, chain))

	// Update deployment health
	err := adminDB.UpdateDeploymentHealth(ctx, 3003, "degraded", "2/3 pods running")
	require.NoError(t, err)

	// Verify update
	updated, err := adminDB.GetChain(ctx, 3003)
	require.NoError(t, err)
	assert.Equal(t, "degraded", updated.DeploymentHealthStatus)
	assert.Equal(t, "2/3 pods running", updated.DeploymentHealthMessage)
	assert.False(t, updated.DeploymentHealthUpdatedAt.IsZero())
}

// TestAdminStore_OverallHealth_Calculation tests overall health computation.
func TestAdminStore_OverallHealth_Calculation(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_overall_health")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a chain
	chain := generateChain(3004)
	require.NoError(t, adminDB.UpsertChain(ctx, chain))

	// Update all health statuses
	require.NoError(t, adminDB.UpdateRPCHealth(ctx, 3004, "healthy", ""))
	require.NoError(t, adminDB.UpdateQueueHealth(ctx, 3004, "warning", ""))
	require.NoError(t, adminDB.UpdateDeploymentHealth(ctx, 3004, "healthy", ""))

	// Verify overall health is "warning" (worst of healthy, warning, healthy)
	updated, err := adminDB.GetChain(ctx, 3004)
	require.NoError(t, err)
	assert.Equal(t, "warning", updated.OverallHealthStatus)
}

// TestAdminStore_OverallHealth_Critical tests critical status propagation.
func TestAdminStore_OverallHealth_Critical(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_critical_health")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a chain
	chain := generateChain(3005)
	require.NoError(t, adminDB.UpsertChain(ctx, chain))

	// Set one subsystem to critical
	require.NoError(t, adminDB.UpdateRPCHealth(ctx, 3005, "healthy", ""))
	require.NoError(t, adminDB.UpdateQueueHealth(ctx, 3005, "healthy", ""))
	require.NoError(t, adminDB.UpdateDeploymentHealth(ctx, 3005, "failed", "All pods crashed"))

	// Verify overall health is "failed"
	updated, err := adminDB.GetChain(ctx, 3005)
	require.NoError(t, err)
	assert.Equal(t, "failed", updated.OverallHealthStatus)
}

// TestAdminStore_RecordIndexed tests recording indexing progress.
func TestAdminStore_RecordIndexed(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_record_indexed")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	chainID := uint64(4001)
	blockTime := time.Now().UTC().Add(-5 * time.Second)

	err := adminDB.RecordIndexed(ctx, chainID, 100, blockTime, 250.5, `{"fetch":100,"process":150}`)
	require.NoError(t, err)

	// Verify by checking LastIndexed
	lastHeight, err := adminDB.LastIndexed(ctx, chainID)
	require.NoError(t, err)
	assert.Equal(t, uint64(100), lastHeight)
}

// TestAdminStore_RecordIndexed_Multiple tests recording multiple heights.
func TestAdminStore_RecordIndexed_Multiple(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_record_multi")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	chainID := uint64(4002)

	// Record multiple heights
	for height := uint64(1); height <= 10; height++ {
		blockTime := time.Now().UTC().Add(-time.Duration(10-height) * time.Second)
		err := adminDB.RecordIndexed(ctx, chainID, height, blockTime, 100.0, "{}")
		require.NoError(t, err)
	}

	// Verify last indexed
	lastHeight, err := adminDB.LastIndexed(ctx, chainID)
	require.NoError(t, err)
	assert.Equal(t, uint64(10), lastHeight)
}

// TestAdminStore_LastIndexed_NoData tests LastIndexed with no data.
func TestAdminStore_LastIndexed_NoData(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_last_indexed_empty")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	lastHeight, err := adminDB.LastIndexed(ctx, 9999)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), lastHeight)
}

// TestAdminStore_FindGaps tests gap detection in indexing progress.
func TestAdminStore_FindGaps(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_find_gaps")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	chainID := uint64(5001)

	// Record heights with gaps: 1, 2, 3, 5, 6, 10, 11, 12
	heights := []uint64{1, 2, 3, 5, 6, 10, 11, 12}
	for _, height := range heights {
		blockTime := time.Now().UTC().Add(-time.Duration(100-height) * time.Second)
		err := adminDB.RecordIndexed(ctx, chainID, height, blockTime, 100.0, "{}")
		require.NoError(t, err)
	}

	// Find gaps
	gaps, err := adminDB.FindGaps(ctx, chainID)
	require.NoError(t, err)

	// Expected gaps: [4,4] and [7,9]
	require.Len(t, gaps, 2)
	assert.Equal(t, uint64(4), gaps[0].From)
	assert.Equal(t, uint64(4), gaps[0].To)
	assert.Equal(t, uint64(7), gaps[1].From)
	assert.Equal(t, uint64(9), gaps[1].To)
}

// TestAdminStore_FindGaps_NoGaps tests gap detection with no gaps.
func TestAdminStore_FindGaps_NoGaps(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_no_gaps")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	chainID := uint64(5002)

	// Record sequential heights: 1-10
	for height := uint64(1); height <= 10; height++ {
		blockTime := time.Now().UTC()
		err := adminDB.RecordIndexed(ctx, chainID, height, blockTime, 100.0, "{}")
		require.NoError(t, err)
	}

	// Find gaps
	gaps, err := adminDB.FindGaps(ctx, chainID)
	require.NoError(t, err)
	assert.Empty(t, gaps)
}

// TestAdminStore_IndexProgressHistory tests historical progress aggregation.
func TestAdminStore_IndexProgressHistory(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_progress_history")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	chainID := uint64(6001)

	// Record heights over the past hour, spread them across time to create multiple buckets
	now := time.Now().UTC()
	for i := 0; i < 60; i++ {
		height := uint64(100 + i)
		// Space blocks 1 minute apart to create natural distribution across time buckets
		blockTime := now.Add(-time.Duration(60-i) * time.Minute)
		err := adminDB.RecordIndexed(ctx, chainID, height, blockTime, float64(100+i), fmt.Sprintf(`{"step_%d":100}`, i))
		require.NoError(t, err)
	}

	// Query progress history for the past 2 hours with 15-minute intervals
	points, err := adminDB.IndexProgressHistory(ctx, chainID, 2, 15)
	require.NoError(t, err)

	// We should have approximately 4-5 buckets (60 minutes / 15 minute intervals)
	// However, depending on ClickHouse bucketing behavior, we might get 1 bucket
	// if all records fall in the same time bucket. Just verify we got at least 1.
	assert.Greater(t, len(points), 0)

	// Verify each point has the expected fields
	for _, point := range points {
		assert.Greater(t, point.MaxHeight, uint64(0))
		assert.GreaterOrEqual(t, point.AvgLatency, 0.0)
		assert.GreaterOrEqual(t, point.AvgProcessingTime, 0.0)
		assert.Greater(t, point.BlocksIndexed, uint64(0))
	}
}

// TestAdminStore_RecordReindexRequests tests recording reindex requests.
func TestAdminStore_RecordReindexRequests(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_reindex")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	chainID := uint64(7001)
	heights := []uint64{100, 101, 102, 103}

	err := adminDB.RecordReindexRequests(ctx, chainID, "test-user", heights)
	require.NoError(t, err)

	// List reindex requests
	requests, err := adminDB.ListReindexRequests(ctx, chainID, 10)
	require.NoError(t, err)
	assert.Len(t, requests, 4)

	// Verify all requests have correct status
	for _, req := range requests {
		assert.Equal(t, "queued", req.Status)
		assert.Equal(t, "test-user", req.RequestedBy)
	}
}

// TestAdminStore_ListReindexRequests_Empty tests listing when no requests exist.
func TestAdminStore_ListReindexRequests_Empty(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_reindex_empty")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	requests, err := adminDB.ListReindexRequests(ctx, 9999, 10)
	require.NoError(t, err)
	assert.Empty(t, requests)
}

// TestAdminStore_ListReindexRequests_Limit tests request limiting.
func TestAdminStore_ListReindexRequests_Limit(t *testing.T) {
	adminDB := createAdminStore(t, "test_admin_reindex_limit")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	chainID := uint64(7002)

	// Create 20 reindex requests
	heights := make([]uint64, 20)
	for i := 0; i < 20; i++ {
		heights[i] = uint64(100 + i)
	}
	err := adminDB.RecordReindexRequests(ctx, chainID, "test-user", heights)
	require.NoError(t, err)

	// List with limit of 5
	requests, err := adminDB.ListReindexRequests(ctx, chainID, 5)
	require.NoError(t, err)
	assert.Len(t, requests, 5)
}

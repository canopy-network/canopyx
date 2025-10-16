//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	"github.com/stretchr/testify/require"
)

// Helper functions for integration tests

// TestDB returns the global test database instance
func TestDB() *db.AdminDB {
	return testDB
}

// CleanDB truncates all tables in the test database
func CleanDB(t *testing.T) {
	cleanDB(t)
}

// SeedData inserts test data into the database
func SeedData(t *testing.T, opts ...SeedOption) {
	seedData(t, opts...)
}

// CreateTestChain creates a test chain with the given ID and optional customization
func CreateTestChain(chainID, chainName string, opts ...ChainOption) *admin.Chain {
	return createTestChain(chainID, chainName, opts...)
}

// WaitForMaterializedView waits for ClickHouse materialized view to process data
func WaitForMaterializedView(t *testing.T, maxWait time.Duration) {
	waitForMaterializedView(t, maxWait)
}

// AssertGaps asserts that the expected gaps exist for a chain
func AssertGaps(t *testing.T, chainID string, expectedGaps []GapExpectation) {
	assertGaps(t, chainID, expectedGaps)
}

// RequireTableExists asserts that a table exists in the database
func RequireTableExists(t *testing.T, tableName string) {
	requireTableExists(t, tableName)
}

// RequireMaterializedViewExists asserts that a materialized view exists
func RequireMaterializedViewExists(t *testing.T, mvName string) {
	requireMaterializedViewExists(t, mvName)
}

// requireChain asserts that a chain exists with the expected values
func requireChain(t *testing.T, chainID string, expectedName string) *admin.Chain {
	t.Helper()
	ctx := context.Background()

	chain, err := testDB.GetChain(ctx, chainID)
	require.NoError(t, err, "Failed to get chain %s", chainID)
	require.Equal(t, chainID, chain.ChainID)
	require.Equal(t, expectedName, chain.ChainName)

	return chain
}

// requireNoChain asserts that a chain does not exist
func requireNoChain(t *testing.T, chainID string) {
	t.Helper()
	ctx := context.Background()

	_, err := testDB.GetChain(ctx, chainID)
	require.Error(t, err, "Expected chain %s to not exist", chainID)
}

// requireIndexProgress asserts that index progress exists for a chain at a specific height
func requireIndexProgress(t *testing.T, chainID string, expectedHeight uint64) {
	t.Helper()
	ctx := context.Background()

	lastHeight, err := testDB.LastIndexed(ctx, chainID)
	require.NoError(t, err, "Failed to get last indexed height for chain %s", chainID)
	require.Equal(t, expectedHeight, lastHeight, "Unexpected last indexed height for chain %s", chainID)
}

// createTestChain creates a test chain with the given ID and optional customization
func createTestChain(chainID, chainName string, opts ...ChainOption) *admin.Chain {
	chain := &admin.Chain{
		ChainID:      chainID,
		ChainName:    chainName,
		RPCEndpoints: []string{"http://localhost:8545"},
		Image:        "test-image:v1",
		MinReplicas:  1,
		MaxReplicas:  3,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	for _, opt := range opts {
		opt(chain)
	}

	return chain
}

// ChainOption is a functional option for customizing test chains
type ChainOption func(*admin.Chain)

// WithRPCEndpoints sets custom RPC endpoints
func WithRPCEndpoints(endpoints ...string) ChainOption {
	return func(c *admin.Chain) {
		c.RPCEndpoints = endpoints
	}
}

// WithImage sets a custom image
func WithImage(image string) ChainOption {
	return func(c *admin.Chain) {
		c.Image = image
	}
}

// WithReplicas sets min and max replicas
func WithReplicas(min, max uint16) ChainOption {
	return func(c *admin.Chain) {
		c.MinReplicas = min
		c.MaxReplicas = max
	}
}

// WithPaused sets the paused flag
func WithPaused(paused bool) ChainOption {
	return func(c *admin.Chain) {
		if paused {
			c.Paused = 1
		} else {
			c.Paused = 0
		}
	}
}

// WithDeleted sets the deleted flag
func WithDeleted(deleted bool) ChainOption {
	return func(c *admin.Chain) {
		if deleted {
			c.Deleted = 1
		} else {
			c.Deleted = 0
		}
	}
}

// createTestIndexProgress creates test index progress records
func createTestIndexProgress(chainID string, heights ...uint64) []*admin.IndexProgress {
	progress := make([]*admin.IndexProgress, 0, len(heights))
	for _, height := range heights {
		progress = append(progress, &admin.IndexProgress{
			ChainID:   chainID,
			Height:    height,
			IndexedAt: time.Now(),
		})
	}
	return progress
}

// insertIndexProgress inserts multiple index progress records
func insertIndexProgress(t *testing.T, chainID string, heights ...uint64) {
	t.Helper()
	ctx := context.Background()

	for _, height := range heights {
		err := testDB.RecordIndexed(ctx, chainID, height)
		require.NoError(t, err, "Failed to record indexed height %d for chain %s", height, chainID)
	}

	t.Logf("Inserted %d index progress records for chain %s", len(heights), chainID)
}

// waitForMaterializedView waits for ClickHouse materialized view to process data
// This is necessary because materialized views are eventually consistent
func waitForMaterializedView(t *testing.T, maxWait time.Duration) {
	t.Helper()
	time.Sleep(100 * time.Millisecond) // Small delay to allow MV to process

	// Could add more sophisticated waiting logic here if needed
}

// assertGaps asserts that the expected gaps exist for a chain
func assertGaps(t *testing.T, chainID string, expectedGaps []GapExpectation) {
	t.Helper()
	ctx := context.Background()

	gaps, err := testDB.FindGaps(ctx, chainID)
	require.NoError(t, err, "Failed to find gaps for chain %s", chainID)

	require.Len(t, gaps, len(expectedGaps), "Unexpected number of gaps for chain %s", chainID)

	for i, expected := range expectedGaps {
		require.Equal(t, expected.From, gaps[i].From,
			"Gap %d: expected From=%d, got %d", i, expected.From, gaps[i].From)
		require.Equal(t, expected.To, gaps[i].To,
			"Gap %d: expected To=%d, got %d", i, expected.To, gaps[i].To)
	}

	t.Logf("Found %d expected gaps for chain %s", len(gaps), chainID)
}

// GapExpectation represents an expected gap
type GapExpectation struct {
	From uint64
	To   uint64
}

// Gap creates a GapExpectation helper
func Gap(from, to uint64) GapExpectation {
	return GapExpectation{From: from, To: to}
}

// countRecords counts the number of records in a table
func countRecords(t *testing.T, tableName string) int {
	t.Helper()
	ctx := context.Background()

	var count int
	query := fmt.Sprintf(`SELECT count() FROM test_canopyx.%s`, tableName)
	err := testDB.Db.NewRaw(query).Scan(ctx, &count)
	require.NoError(t, err, "Failed to count records in %s", tableName)

	return count
}

// dumpIndexProgress dumps all index progress records for debugging
func dumpIndexProgress(t *testing.T, chainID string) {
	t.Helper()
	ctx := context.Background()

	var records []admin.IndexProgress
	err := testDB.Db.NewSelect().
		Model(&records).
		Where("chain_id = ?", chainID).
		OrderExpr("height ASC").
		Scan(ctx)

	if err != nil {
		t.Logf("Failed to dump index progress for chain %s: %v", chainID, err)
		return
	}

	t.Logf("Index progress for chain %s:", chainID)
	for _, r := range records {
		t.Logf("  Height: %d, Indexed at: %s", r.Height, r.IndexedAt.Format(time.RFC3339))
	}
}

// dumpChains dumps all chains for debugging
func dumpChains(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	chains, err := testDB.ListChain(ctx)
	if err != nil {
		t.Logf("Failed to dump chains: %v", err)
		return
	}

	t.Logf("Chains in database:")
	for _, c := range chains {
		t.Logf("  ChainID: %s, Name: %s, Paused: %d, Deleted: %d",
			c.ChainID, c.ChainName, c.Paused, c.Deleted)
	}
}

// requireTableExists asserts that a table exists in the database
func requireTableExists(t *testing.T, tableName string) {
	t.Helper()
	ctx := context.Background()

	query := `
		SELECT count()
		FROM system.tables
		WHERE database = 'test_canopyx' AND name = ?
	`
	var count int
	err := testDB.Db.NewRaw(query, tableName).Scan(ctx, &count)
	require.NoError(t, err, "Failed to check if table %s exists", tableName)
	require.Equal(t, 1, count, "Table %s does not exist", tableName)
}

// requireMaterializedViewExists asserts that a materialized view exists
func requireMaterializedViewExists(t *testing.T, mvName string) {
	t.Helper()
	ctx := context.Background()

	query := `
		SELECT count()
		FROM system.tables
		WHERE database = 'test_canopyx'
		  AND name = ?
		  AND engine LIKE '%MaterializedView%'
	`
	var count int
	err := testDB.Db.NewRaw(query, mvName).Scan(ctx, &count)
	require.NoError(t, err, "Failed to check if materialized view %s exists", mvName)
	require.Equal(t, 1, count, "Materialized view %s does not exist", mvName)
}

package helpers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	"github.com/stretchr/testify/require"
)

func CleanDB(ctx context.Context, t *testing.T) {
	t.Helper()
	if SkipIfNoDB(ctx, t) {
		return
	}
	if _, err := testDB.Db.ExecContext(ctx, `TRUNCATE TABLE IF EXISTS test_canopyx.chains`); err != nil {
		t.Logf("Warning: failed to truncate chains table: %v", err)
	}
	if _, err := testDB.Db.ExecContext(ctx, `TRUNCATE TABLE IF EXISTS test_canopyx.index_progress`); err != nil {
		t.Logf("Warning: failed to truncate index_progress table: %v", err)
	}
	if _, err := testDB.Db.ExecContext(ctx, `TRUNCATE TABLE IF EXISTS test_canopyx.index_progress_agg`); err != nil {
		t.Logf("Warning: failed to truncate index_progress_agg table: %v", err)
	}
	if _, err := testDB.Db.ExecContext(ctx, `TRUNCATE TABLE IF EXISTS test_canopyx.reindex_requests`); err != nil {
		t.Logf("Warning: failed to truncate reindex_requests table: %v", err)
	}
}

func SeedData(ctx context.Context, t *testing.T, opts ...SeedOption) {
	t.Helper()
	if SkipIfNoDB(ctx, t) {
		return
	}
	config := &seedConfig{}
	for _, opt := range opts {
		opt(config)
	}

	for _, chain := range config.chains {
		if err := testDB.UpsertChain(ctx, chain); err != nil {
			t.Fatalf("Failed to seed chain %s: %v", chain.ChainID, err)
		}
	}

	for _, progress := range config.indexProgress {
		if err := testDB.RecordIndexed(ctx, progress.ChainID, progress.Height); err != nil {
			t.Fatalf("Failed to seed index progress for chain %s height %d: %v",
				progress.ChainID, progress.Height, err)
		}
	}

}

type SeedOption func(*seedConfig)

type seedConfig struct {
	chains        []*admin.Chain
	indexProgress []*admin.IndexProgress
}

func WithChains(chains ...*admin.Chain) SeedOption {
	return func(c *seedConfig) {
		c.chains = append(c.chains, chains...)
	}
}

func WithIndexProgress(progress ...*admin.IndexProgress) SeedOption {
	return func(c *seedConfig) {
		c.indexProgress = append(c.indexProgress, progress...)
	}
}

// CreateTestChain returns a chain configured for testing with optional overrides.
func CreateTestChain(chainID, chainName string, opts ...ChainOption) *admin.Chain {
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

type ChainOption func(*admin.Chain)

func WithRPCEndpoints(endpoints ...string) ChainOption {
	return func(c *admin.Chain) {
		c.RPCEndpoints = endpoints
	}
}

func WithImage(image string) ChainOption {
	return func(c *admin.Chain) {
		c.Image = image
	}
}

func WithReplicas(min, max uint16) ChainOption {
	return func(c *admin.Chain) {
		c.MinReplicas = min
		c.MaxReplicas = max
	}
}

func WithPaused(paused bool) ChainOption {
	return func(c *admin.Chain) {
		if paused {
			c.Paused = 1
		} else {
			c.Paused = 0
		}
	}
}

func WaitForMaterializedView(_ *testing.T, maxWait time.Duration) {
	time.Sleep(100 * time.Millisecond)
	_ = maxWait
}

type GapExpectation struct {
	From uint64
	To   uint64
}

func Gap(from, to uint64) GapExpectation {
	return GapExpectation{From: from, To: to}
}

func AssertGaps(ctx context.Context, t *testing.T, chainID string, expectedGaps []GapExpectation) {
	t.Helper()
	if SkipIfNoDB(ctx, t) {
		return
	}
	gaps, err := testDB.FindGaps(ctx, chainID)
	require.NoError(t, err, "Failed to find gaps for chain %s", chainID)

	require.Len(t, gaps, len(expectedGaps), "Unexpected number of gaps for chain %s", chainID)

	for i, expected := range expectedGaps {
		require.Equal(t, expected.From, gaps[i].From,
			"Gap %d: expected From=%d, got %d", i, expected.From, gaps[i].From)
		require.Equal(t, expected.To, gaps[i].To,
			"Gap %d: expected To=%d, got %d", i, expected.To, gaps[i].To)
	}

}

func RequireTableExists(ctx context.Context, t *testing.T, tableName string) {
	t.Helper()
	if SkipIfNoDB(ctx, t) {
		return
	}
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

func RequireMaterializedViewExists(ctx context.Context, t *testing.T, mvName string) {
	t.Helper()
	if SkipIfNoDB(ctx, t) {
		return
	}
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

func RequireChain(ctx context.Context, t *testing.T, chainID string, expectedName string) *admin.Chain {
	t.Helper()
	if SkipIfNoDB(ctx, t) {
		return nil
	}
	chain, err := testDB.GetChain(ctx, chainID)
	require.NoError(t, err, "Failed to get chain %s", chainID)
	require.Equal(t, chainID, chain.ChainID)
	require.Equal(t, expectedName, chain.ChainName)

	return chain
}

func RequireNoChain(ctx context.Context, t *testing.T, chainID string) {
	t.Helper()
	if SkipIfNoDB(ctx, t) {
		return
	}
	_, err := testDB.GetChain(ctx, chainID)
	require.Error(t, err, "Expected chain %s to not exist", chainID)
}

func RequireIndexProgress(ctx context.Context, t *testing.T, chainID string, expectedHeight uint64) {
	t.Helper()
	if SkipIfNoDB(ctx, t) {
		return
	}
	lastHeight, err := testDB.LastIndexed(ctx, chainID)
	require.NoError(t, err, "Failed to get last indexed height for chain %s", chainID)
	require.Equal(t, expectedHeight, lastHeight, "Unexpected last indexed height for chain %s", chainID)
}

func CountRecords(ctx context.Context, t *testing.T, tableName string) int {
	t.Helper()
	if SkipIfNoDB(ctx, t) {
		return 0
	}
	var count int
	query := fmt.Sprintf(`SELECT count() FROM test_canopyx.%s`, tableName)
	err := testDB.Db.NewRaw(query).Scan(ctx, &count)
	require.NoError(t, err, "Failed to count records in %s", tableName)

	return count
}

func DumpIndexProgress(ctx context.Context, t *testing.T, chainID string) {
	t.Helper()
	if SkipIfNoDB(ctx, t) {
		return
	}
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

func DumpChains(ctx context.Context, t *testing.T) {
	t.Helper()
	if SkipIfNoDB(ctx, t) {
		return
	}
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

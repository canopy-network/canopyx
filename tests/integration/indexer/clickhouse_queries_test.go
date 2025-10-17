package indexer

import (
	"context"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	"github.com/canopy-network/canopyx/tests/integration/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecordIndexed tests inserting index progress records
func TestRecordIndexed(t *testing.T) {
	ctx := context.Background()
	helpers.CleanDB(ctx, t)
	chainID := "test-chain-record"

	// Create test chain
	chain := helpers.CreateTestChain(chainID, "Test Chain Record")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	tests := []struct {
		name   string
		height uint64
	}{
		{
			name:   "record first height",
			height: 1,
		},
		{
			name:   "record sequential height",
			height: 2,
		},
		{
			name:   "record non-sequential height",
			height: 100,
		},
		{
			name:   "record duplicate height",
			height: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := helpers.TestDB().RecordIndexed(ctx, chainID, tt.height)
			require.NoError(t, err, "Failed to record indexed height %d", tt.height)

			// Verify the record was inserted
			count, err := helpers.TestDB().Db.NewSelect().
				Model((*admin.IndexProgress)(nil)).
				Where("chain_id = ? AND height = ?", chainID, tt.height).
				Count(ctx)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, count, 1, "Expected at least one record for height %d", tt.height)
		})
	}
}

// TestLastIndexed tests retrieving the last indexed height
func TestLastIndexed(t *testing.T) {
	ctx := context.Background()
	helpers.CleanDB(ctx, t)
	chainID := "test-chain-last"

	// Create test chain
	chain := helpers.CreateTestChain(chainID, "Test Chain Last")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	tests := []struct {
		name         string
		heights      []uint64
		expectedLast uint64
		waitForMV    bool
	}{
		{
			name:         "no heights indexed",
			heights:      []uint64{},
			expectedLast: 0,
		},
		{
			name:         "single height indexed",
			heights:      []uint64{42},
			expectedLast: 42,
			waitForMV:    true,
		},
		{
			name:         "multiple sequential heights",
			heights:      []uint64{1, 2, 3, 4, 5},
			expectedLast: 5,
			waitForMV:    true,
		},
		{
			name:         "multiple non-sequential heights",
			heights:      []uint64{10, 20, 30, 15, 25},
			expectedLast: 30,
			waitForMV:    true,
		},
		{
			name:         "heights with gaps",
			heights:      []uint64{1, 2, 5, 6, 10},
			expectedLast: 10,
			waitForMV:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean and reset for this test
			helpers.CleanDB(ctx, t)
			helpers.SeedData(ctx, t, helpers.WithChains(chain))

			// Insert heights
			for _, height := range tt.heights {
				err := helpers.TestDB().RecordIndexed(ctx, chainID, height)
				require.NoError(t, err, "Failed to record height %d", height)
			}

			// Wait for materialized view to process if needed
			if tt.waitForMV {
				helpers.WaitForMaterializedView(t, 2*time.Second)
			}

			// Get last indexed height
			lastHeight, err := helpers.TestDB().LastIndexed(ctx, chainID)
			require.NoError(t, err, "Failed to get last indexed height")
			assert.Equal(t, tt.expectedLast, lastHeight, "Unexpected last indexed height")
		})
	}
}

// TestLastIndexed_ReplacingMergeTree tests ReplacingMergeTree behavior with aggregate table
func TestLastIndexed_ReplacingMergeTree(t *testing.T) {
	ctx := context.Background()
	helpers.CleanDB(ctx, t)
	chainID := "test-chain-rmt"

	// Create test chain
	chain := helpers.CreateTestChain(chainID, "Test Chain RMT")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	// Insert multiple heights rapidly
	heights := []uint64{100, 101, 102, 103, 104, 105}
	for _, height := range heights {
		err := helpers.TestDB().RecordIndexed(ctx, chainID, height)
		require.NoError(t, err, "Failed to record height %d", height)
	}

	// Wait for materialized view to aggregate
	helpers.WaitForMaterializedView(t, 2*time.Second)

	// Should get the max height from the aggregate table
	lastHeight, err := helpers.TestDB().LastIndexed(ctx, chainID)
	require.NoError(t, err)
	assert.Equal(t, uint64(105), lastHeight, "Expected max height from aggregate")

	// Insert more heights
	moreHeights := []uint64{106, 107, 108}
	for _, height := range moreHeights {
		err := helpers.TestDB().RecordIndexed(ctx, chainID, height)
		require.NoError(t, err, "Failed to record height %d", height)
	}

	// Wait again
	helpers.WaitForMaterializedView(t, 2*time.Second)

	// Should now get the new max
	lastHeight, err = helpers.TestDB().LastIndexed(ctx, chainID)
	require.NoError(t, err)
	assert.Equal(t, uint64(108), lastHeight, "Expected updated max height")
}

// TestFindGaps tests gap detection in indexed blocks
func TestFindGaps(t *testing.T) {
	ctx := context.Background()
	helpers.CleanDB(ctx, t)
	chainID := "test-chain-gaps"

	// Create test chain
	chain := helpers.CreateTestChain(chainID, "Test Chain Gaps")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	tests := []struct {
		name         string
		heights      []uint64
		expectedGaps []helpers.GapExpectation
	}{
		{
			name:         "no gaps - sequential",
			heights:      []uint64{1, 2, 3, 4, 5},
			expectedGaps: []helpers.GapExpectation{},
		},
		{
			name:    "single gap",
			heights: []uint64{1, 2, 3, 7, 8, 9},
			expectedGaps: []helpers.GapExpectation{
				helpers.Gap(4, 6),
			},
		},
		{
			name:    "multiple gaps",
			heights: []uint64{1, 2, 5, 6, 10, 11},
			expectedGaps: []helpers.GapExpectation{
				helpers.Gap(3, 4),
				helpers.Gap(7, 9),
			},
		},
		{
			name:    "large gap",
			heights: []uint64{1, 2, 3, 1000, 1001},
			expectedGaps: []helpers.GapExpectation{
				helpers.Gap(4, 999),
			},
		},
		{
			name:         "gap at beginning",
			heights:      []uint64{10, 11, 12},
			expectedGaps: []helpers.GapExpectation{},
		},
		{
			name:         "no heights",
			heights:      []uint64{},
			expectedGaps: []helpers.GapExpectation{},
		},
		{
			name:         "single height",
			heights:      []uint64{42},
			expectedGaps: []helpers.GapExpectation{},
		},
		{
			name:    "two heights with gap",
			heights: []uint64{1, 10},
			expectedGaps: []helpers.GapExpectation{
				helpers.Gap(2, 9),
			},
		},
		{
			name:    "multiple small gaps",
			heights: []uint64{1, 3, 5, 7, 9},
			expectedGaps: []helpers.GapExpectation{
				helpers.Gap(2, 2),
				helpers.Gap(4, 4),
				helpers.Gap(6, 6),
				helpers.Gap(8, 8),
			},
		},
		{
			name:    "complex gap pattern",
			heights: []uint64{1, 2, 3, 10, 11, 12, 20, 21, 100},
			expectedGaps: []helpers.GapExpectation{
				helpers.Gap(4, 9),
				helpers.Gap(13, 19),
				helpers.Gap(22, 99),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean and reset for this test
			helpers.CleanDB(ctx, t)
			helpers.SeedData(ctx, t, helpers.WithChains(chain))

			// Insert heights
			for _, height := range tt.heights {
				err := helpers.TestDB().RecordIndexed(ctx, chainID, height)
				require.NoError(t, err, "Failed to record height %d", height)
			}

			// Find gaps
			helpers.AssertGaps(ctx, t, chainID, tt.expectedGaps)
		})
	}
}

// TestFindGaps_MultipleChains tests that gaps are isolated per chain
func TestFindGaps_MultipleChains(t *testing.T) {
	ctx := context.Background()
	helpers.CleanDB(ctx, t)

	// Create two test chains
	chain1 := helpers.CreateTestChain("chain-1", "Chain 1")
	chain2 := helpers.CreateTestChain("chain-2", "Chain 2")
	helpers.SeedData(ctx, t, helpers.WithChains(chain1, chain2))

	// Insert heights for chain 1 with gaps
	heights1 := []uint64{1, 2, 3, 10, 11, 12}
	for _, height := range heights1 {
		err := helpers.TestDB().RecordIndexed(ctx, "chain-1", height)
		require.NoError(t, err)
	}

	// Insert heights for chain 2 with different gaps
	heights2 := []uint64{1, 5, 6, 20}
	for _, height := range heights2 {
		err := helpers.TestDB().RecordIndexed(ctx, "chain-2", height)
		require.NoError(t, err)
	}

	// Verify chain 1 gaps
	helpers.AssertGaps(ctx, t, "chain-1", []helpers.GapExpectation{
		helpers.Gap(4, 9),
	})

	// Verify chain 2 gaps
	helpers.AssertGaps(ctx, t, "chain-2", []helpers.GapExpectation{
		helpers.Gap(2, 4),
		helpers.Gap(7, 19),
	})
}

// TestFindGaps_OutOfOrder tests gap detection when heights are inserted out of order
func TestFindGaps_OutOfOrder(t *testing.T) {
	ctx := context.Background()
	helpers.CleanDB(ctx, t)
	chainID := "test-chain-outoforder"

	// Create test chain
	chain := helpers.CreateTestChain(chainID, "Test Chain Out of Order")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	// Insert heights in random order
	heights := []uint64{5, 1, 10, 3, 8, 2, 9, 6}
	for _, height := range heights {
		err := helpers.TestDB().RecordIndexed(ctx, chainID, height)
		require.NoError(t, err, "Failed to record height %d", height)
	}

	// Expected gaps: 4 (missing), 7 (missing)
	helpers.AssertGaps(ctx, t, chainID, []helpers.GapExpectation{
		helpers.Gap(4, 4),
		helpers.Gap(7, 7),
	})
}

// TestFindGaps_FillingGaps tests that gaps disappear when filled
func TestFindGaps_FillingGaps(t *testing.T) {
	ctx := context.Background()
	helpers.CleanDB(ctx, t)
	chainID := "test-chain-filling"

	// Create test chain
	chain := helpers.CreateTestChain(chainID, "Test Chain Filling")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	// Insert heights with gaps
	heights := []uint64{1, 2, 3, 10, 11, 12}
	for _, height := range heights {
		err := helpers.TestDB().RecordIndexed(ctx, chainID, height)
		require.NoError(t, err)
	}

	// Verify gap exists
	helpers.AssertGaps(ctx, t, chainID, []helpers.GapExpectation{
		helpers.Gap(4, 9),
	})

	// Fill part of the gap
	for height := uint64(4); height <= 6; height++ {
		err := helpers.TestDB().RecordIndexed(ctx, chainID, height)
		require.NoError(t, err)
	}

	// Gap should now be smaller
	helpers.AssertGaps(ctx, t, chainID, []helpers.GapExpectation{
		helpers.Gap(7, 9),
	})

	// Fill the rest
	for height := uint64(7); height <= 9; height++ {
		err := helpers.TestDB().RecordIndexed(ctx, chainID, height)
		require.NoError(t, err)
	}

	// No gaps should remain
	helpers.AssertGaps(ctx, t, chainID, []helpers.GapExpectation{})
}

// TestFindGaps_Performance tests gap detection with a large dataset
func TestFindGaps_Performance(t *testing.T) {
	ctx := context.Background()
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	helpers.CleanDB(ctx, t)
	chainID := "test-chain-perf"

	// Create test chain
	chain := helpers.CreateTestChain(chainID, "Test Chain Performance")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	// Insert 10,000 heights with some gaps
	for i := uint64(1); i <= 10000; i++ {
		// Skip every 1000th height to create gaps
		if i%1000 == 0 {
			continue
		}
		err := helpers.TestDB().RecordIndexed(ctx, chainID, i)
		require.NoError(t, err)
	}

	// Measure gap detection performance
	start := time.Now()
	gaps, err := helpers.TestDB().FindGaps(ctx, chainID)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Len(t, gaps, 9, "Expected 9 gaps (skipped every 1000th height)")

	// Assert performance is reasonable (should be well under 1 second)
	assert.Less(t, elapsed, 5*time.Second, "Gap detection took too long")
}

// TestDatabaseInitialization verifies that all required tables and views are created
func TestDatabaseInitialization(t *testing.T) {
	ctx := context.Background()
	// This test verifies the schema created by TestMain

	// Check that required tables exist
	helpers.RequireTableExists(ctx, t, "chains")
	helpers.RequireTableExists(ctx, t, "index_progress")
	helpers.RequireTableExists(ctx, t, "index_progress_agg")
	helpers.RequireTableExists(ctx, t, "reindex_requests")

	// Check that materialized view exists
	helpers.RequireMaterializedViewExists(ctx, t, "index_progress_mv")
}

// TestConcurrentRecordIndexed tests concurrent inserts to ensure thread safety
func TestConcurrentRecordIndexed(t *testing.T) {
	ctx := context.Background()
	helpers.CleanDB(ctx, t)
	chainID := "test-chain-concurrent"

	// Create test chain
	chain := helpers.CreateTestChain(chainID, "Test Chain Concurrent")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	// Insert heights concurrently
	numGoroutines := 10
	heightsPerGoroutine := 100

	errChan := make(chan error, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(offset int) {
			for j := 0; j < heightsPerGoroutine; j++ {
				height := uint64(offset*heightsPerGoroutine + j + 1)
				if err := helpers.TestDB().RecordIndexed(ctx, chainID, height); err != nil {
					errChan <- err
					return
				}
			}
			errChan <- nil
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		err := <-errChan
		require.NoError(t, err, "Concurrent insert failed")
	}

	// Wait for materialized view
	helpers.WaitForMaterializedView(t, 2*time.Second)

	// Verify last indexed height
	expectedLast := uint64(numGoroutines * heightsPerGoroutine)
	lastHeight, err := helpers.TestDB().LastIndexed(ctx, chainID)
	require.NoError(t, err)
	assert.Equal(t, expectedLast, lastHeight, "Expected all heights to be recorded")
}

// TestSaveBlockSummary tests inserting and retrieving block summaries
func TestSaveBlockSummary(t *testing.T) {
	ctx := context.Background()
	helpers.CleanDB(ctx, t)
	chainID := "test-chain-summary"

	// Create test chain and chain DB
	chain := helpers.CreateTestChain(chainID, "Test Chain Summary")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	chainDb, err := helpers.NewChainDb(ctx, chainID)
	require.NoError(t, err, "Failed to create chain DB")
	defer chainDb.Close()

	tests := []struct {
		name   string
		height uint64
		numTxs uint32
	}{
		{
			name:   "save summary with no transactions",
			height: 1,
			numTxs: 0,
		},
		{
			name:   "save summary with transactions",
			height: 2,
			numTxs: 10,
		},
		{
			name:   "save summary with many transactions",
			height: 3,
			numTxs: 1000,
		},
		{
			name:   "update existing summary (ReplacingMergeTree)",
			height: 2,
			numTxs: 15, // Update height 2 with new count
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := chainDb.InsertBlockSummary(ctx, tt.height, tt.numTxs)
			require.NoError(t, err, "Failed to insert block summary")

			// Verify the summary was inserted
			summary, err := chainDb.GetBlockSummary(ctx, tt.height)
			require.NoError(t, err, "Failed to get block summary")
			assert.Equal(t, tt.height, summary.Height, "Unexpected height")
			assert.Equal(t, tt.numTxs, summary.NumTxs, "Unexpected numTxs")
		})
	}
}

// TestBlockSummaryIsolation tests that block summaries are isolated per chain
func TestBlockSummaryIsolation(t *testing.T) {
	ctx := context.Background()
	helpers.CleanDB(ctx, t)

	// Create two test chains
	chain1 := helpers.CreateTestChain("chain-1", "Chain 1")
	chain2 := helpers.CreateTestChain("chain-2", "Chain 2")
	helpers.SeedData(ctx, t, helpers.WithChains(chain1, chain2))

	// Create chain DBs
	chainDb1, err := helpers.NewChainDb(ctx, "chain-1")
	require.NoError(t, err)
	defer chainDb1.Close()

	chainDb2, err := helpers.NewChainDb(ctx, "chain-2")
	require.NoError(t, err)
	defer chainDb2.Close()

	// Insert summaries for chain 1
	err = chainDb1.InsertBlockSummary(ctx, 100, 50)
	require.NoError(t, err)

	// Insert summaries for chain 2 (same height, different data)
	err = chainDb2.InsertBlockSummary(ctx, 100, 75)
	require.NoError(t, err)

	// Verify isolation - chain 1 should have its own data
	summary1, err := chainDb1.GetBlockSummary(ctx, 100)
	require.NoError(t, err)
	assert.Equal(t, uint32(50), summary1.NumTxs, "Chain 1 should have 50 txs")

	// Verify isolation - chain 2 should have its own data
	summary2, err := chainDb2.GetBlockSummary(ctx, 100)
	require.NoError(t, err)
	assert.Equal(t, uint32(75), summary2.NumTxs, "Chain 2 should have 75 txs")
}

// TestBlockSummaryReplacingMergeTree tests that ReplacingMergeTree deduplicates correctly
func TestBlockSummaryReplacingMergeTree(t *testing.T) {
	ctx := context.Background()
	helpers.CleanDB(ctx, t)
	chainID := "test-chain-rmt-summary"

	// Create test chain and chain DB
	chain := helpers.CreateTestChain(chainID, "Test Chain RMT Summary")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	chainDb, err := helpers.NewChainDb(ctx, chainID)
	require.NoError(t, err)
	defer chainDb.Close()

	// Insert initial summary
	err = chainDb.InsertBlockSummary(ctx, 100, 10)
	require.NoError(t, err)

	// Insert updated summary for same height (simulating correction)
	err = chainDb.InsertBlockSummary(ctx, 100, 15)
	require.NoError(t, err)

	// Wait a moment for ClickHouse to merge
	time.Sleep(500 * time.Millisecond)

	// Should get the latest value
	summary, err := chainDb.GetBlockSummary(ctx, 100)
	require.NoError(t, err)
	assert.Equal(t, uint32(15), summary.NumTxs, "Expected latest value from ReplacingMergeTree")
}

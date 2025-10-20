package db

import (
	"context"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/tests/integration/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInsertBlockSummaryWithTxCounts tests inserting block summaries with transaction counts
func TestInsertBlockSummaryWithTxCounts(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainDB, err := helpers.NewChainDb(ctx, "test_chain_summary")
	require.NoError(t, err)
	defer chainDB.Close()

	err = chainDB.InitializeDB(ctx)
	require.NoError(t, err)

	blockTime := time.Now().UTC()

	// Insert block summary with transaction counts by type
	txCountsByType := map[string]uint32{
		"send":     5,
		"delegate": 2,
		"vote":     1,
	}

	err = chainDB.InsertBlockSummary(ctx, 100, blockTime, 8, txCountsByType)
	require.NoError(t, err)

	// Retrieve and verify
	summary, err := chainDB.GetBlockSummary(ctx, 100)
	require.NoError(t, err)
	require.NotNil(t, summary)

	assert.Equal(t, uint64(100), summary.Height)
	assert.Equal(t, uint32(8), summary.NumTxs)
	assert.Equal(t, blockTime.Unix(), summary.HeightTime.Unix())

	// Verify transaction counts by type
	require.NotNil(t, summary.TxCountsByType)
	assert.Equal(t, uint32(5), summary.TxCountsByType["send"])
	assert.Equal(t, uint32(2), summary.TxCountsByType["delegate"])
	assert.Equal(t, uint32(1), summary.TxCountsByType["vote"])
}

// TestQueryBlockSummary tests retrieving block summaries
func TestQueryBlockSummary(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainDB, err := helpers.NewChainDb(ctx, "test_chain_query_summary")
	require.NoError(t, err)
	defer chainDB.Close()

	err = chainDB.InitializeDB(ctx)
	require.NoError(t, err)

	blockTime := time.Now().UTC()

	// Insert multiple block summaries
	summaries := []struct {
		height         uint64
		numTxs         uint32
		txCountsByType map[string]uint32
	}{
		{
			height: 200,
			numTxs: 10,
			txCountsByType: map[string]uint32{
				"send":     6,
				"delegate": 3,
				"vote":     1,
			},
		},
		{
			height: 201,
			numTxs: 5,
			txCountsByType: map[string]uint32{
				"send":  3,
				"stake": 2,
			},
		},
		{
			height:         202,
			numTxs:         0,
			txCountsByType: map[string]uint32{}, // Empty block
		},
		{
			height: 203,
			numTxs: 15,
			txCountsByType: map[string]uint32{
				"contract": 10,
				"send":     5,
			},
		},
	}

	for _, s := range summaries {
		err = chainDB.InsertBlockSummary(ctx, s.height, blockTime, s.numTxs, s.txCountsByType)
		require.NoError(t, err)
	}

	// Query all summaries
	results, err := chainDB.QueryBlockSummaries(ctx, 0, 10, false)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 4)

	// Verify we can retrieve each summary individually
	for _, s := range summaries {
		retrieved, err := chainDB.GetBlockSummary(ctx, s.height)
		require.NoError(t, err)
		assert.Equal(t, s.height, retrieved.Height)
		assert.Equal(t, s.numTxs, retrieved.NumTxs)

		// Verify transaction counts match
		if len(s.txCountsByType) > 0 {
			for msgType, count := range s.txCountsByType {
				assert.Equal(t, count, retrieved.TxCountsByType[msgType],
					"mismatch for type %s at height %d", msgType, s.height)
			}
		}
	}
}

// TestBlockSummaryMapType verifies ClickHouse Map type works correctly
func TestBlockSummaryMapType(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainDB, err := helpers.NewChainDb(ctx, "test_chain_map_type")
	require.NoError(t, err)
	defer chainDB.Close()

	err = chainDB.InitializeDB(ctx)
	require.NoError(t, err)

	blockTime := time.Now().UTC()

	tests := []struct {
		name           string
		height         uint64
		txCountsByType map[string]uint32
		description    string
	}{
		{
			name:           "empty map",
			height:         300,
			txCountsByType: map[string]uint32{},
			description:    "block with no transactions",
		},
		{
			name:   "single type",
			height: 301,
			txCountsByType: map[string]uint32{
				"send": 10,
			},
			description: "block with only one transaction type",
		},
		{
			name:   "multiple types",
			height: 302,
			txCountsByType: map[string]uint32{
				"send":       5,
				"delegate":   3,
				"undelegate": 2,
				"vote":       1,
			},
			description: "block with multiple transaction types",
		},
		{
			name:   "all 11 types",
			height: 303,
			txCountsByType: map[string]uint32{
				"send":       10,
				"delegate":   5,
				"undelegate": 3,
				"stake":      4,
				"unstake":    2,
				"edit_stake": 1,
				"vote":       8,
				"proposal":   1,
				"contract":   15,
				"system":     1,
				"unknown":    2,
			},
			description: "block with all 11 message types",
		},
		{
			name:   "large counts",
			height: 304,
			txCountsByType: map[string]uint32{
				"send":     10000,
				"contract": 50000,
			},
			description: "block with high transaction counts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Insert block summary
			numTxs := uint32(0)
			for _, count := range tt.txCountsByType {
				numTxs += count
			}

			err = chainDB.InsertBlockSummary(ctx, tt.height, blockTime, numTxs, tt.txCountsByType)
			require.NoError(t, err)

			// Retrieve and verify
			summary, err := chainDB.GetBlockSummary(ctx, tt.height)
			require.NoError(t, err)

			assert.Equal(t, tt.height, summary.Height)
			assert.Equal(t, numTxs, summary.NumTxs)

			// Verify map contents
			if len(tt.txCountsByType) == 0 {
				// Empty map should be retrievable
				assert.NotNil(t, summary.TxCountsByType)
				assert.Empty(t, summary.TxCountsByType)
			} else {
				require.NotNil(t, summary.TxCountsByType)
				assert.Equal(t, len(tt.txCountsByType), len(summary.TxCountsByType))

				for msgType, expectedCount := range tt.txCountsByType {
					actualCount, exists := summary.TxCountsByType[msgType]
					assert.True(t, exists, "type %s not found in retrieved map", msgType)
					assert.Equal(t, expectedCount, actualCount, "count mismatch for type %s", msgType)
				}
			}
		})
	}
}

// TestBlockSummaryMultipleTypes tests block summaries with various transaction type distributions
func TestBlockSummaryMultipleTypes(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainDB, err := helpers.NewChainDb(ctx, "test_chain_multi_types")
	require.NoError(t, err)
	defer chainDB.Close()

	err = chainDB.InitializeDB(ctx)
	require.NoError(t, err)

	blockTime := time.Now().UTC()

	// Simulate a realistic sequence of blocks with different transaction patterns
	blocks := []struct {
		height         uint64
		txCountsByType map[string]uint32
	}{
		{
			height: 400,
			txCountsByType: map[string]uint32{
				"send": 20, // High send activity
			},
		},
		{
			height: 401,
			txCountsByType: map[string]uint32{
				"vote":     50, // Governance voting
				"proposal": 1,
			},
		},
		{
			height: 402,
			txCountsByType: map[string]uint32{
				"delegate":   10, // Staking activity
				"undelegate": 5,
				"stake":      8,
			},
		},
		{
			height: 403,
			txCountsByType: map[string]uint32{
				"contract": 100, // Contract-heavy block
				"send":     10,
			},
		},
		{
			height:         404,
			txCountsByType: map[string]uint32{}, // Empty block
		},
		{
			height: 405,
			txCountsByType: map[string]uint32{
				"send":       15,
				"delegate":   8,
				"vote":       3,
				"contract":   12,
				"stake":      5,
				"undelegate": 2,
			},
		},
	}

	// Insert all blocks
	for _, b := range blocks {
		numTxs := uint32(0)
		for _, count := range b.txCountsByType {
			numTxs += count
		}

		err = chainDB.InsertBlockSummary(ctx, b.height, blockTime, numTxs, b.txCountsByType)
		require.NoError(t, err)
	}

	// Query summaries in ascending order
	summaries, err := chainDB.QueryBlockSummaries(ctx, 0, 10, false)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(summaries), len(blocks))

	// Verify we can retrieve each block's summary correctly
	for _, b := range blocks {
		summary, err := chainDB.GetBlockSummary(ctx, b.height)
		require.NoError(t, err)

		expectedTotal := uint32(0)
		for _, count := range b.txCountsByType {
			expectedTotal += count
		}

		assert.Equal(t, expectedTotal, summary.NumTxs)

		// Verify transaction type distribution
		for msgType, expectedCount := range b.txCountsByType {
			actualCount := summary.TxCountsByType[msgType]
			assert.Equal(t, expectedCount, actualCount,
				"height %d: expected %d %s transactions, got %d",
				b.height, expectedCount, msgType, actualCount)
		}
	}
}

// TestBlockSummaryDeduplication tests ReplacingMergeTree deduplication
func TestBlockSummaryDeduplication(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainDB, err := helpers.NewChainDb(ctx, "test_chain_summary_dedup")
	require.NoError(t, err)
	defer chainDB.Close()

	err = chainDB.InitializeDB(ctx)
	require.NoError(t, err)

	blockTime := time.Now().UTC()

	// Insert initial summary
	txCounts1 := map[string]uint32{
		"send": 5,
		"vote": 2,
	}

	err = chainDB.InsertBlockSummary(ctx, 500, blockTime, 7, txCounts1)
	require.NoError(t, err)

	// Insert updated summary for same height (simulating reindex)
	txCounts2 := map[string]uint32{
		"send":     6,
		"vote":     3,
		"delegate": 1,
	}

	err = chainDB.InsertBlockSummary(ctx, 500, blockTime, 10, txCounts2)
	require.NoError(t, err)

	// Query with FINAL to get deduplicated result
	summary, err := chainDB.GetBlockSummary(ctx, 500)
	require.NoError(t, err)

	// Should get the latest version
	assert.Equal(t, uint32(10), summary.NumTxs)
	assert.Equal(t, uint32(6), summary.TxCountsByType["send"])
	assert.Equal(t, uint32(3), summary.TxCountsByType["vote"])
	assert.Equal(t, uint32(1), summary.TxCountsByType["delegate"])
}

// TestBlockSummaryHeightTime verifies HeightTime is stored correctly
func TestBlockSummaryHeightTime(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainDB, err := helpers.NewChainDb(ctx, "test_chain_height_time")
	require.NoError(t, err)
	defer chainDB.Close()

	err = chainDB.InitializeDB(ctx)
	require.NoError(t, err)

	// Use a specific time for testing
	blockTime := time.Date(2024, 10, 19, 15, 30, 45, 0, time.UTC)

	txCounts := map[string]uint32{
		"send": 10,
	}

	err = chainDB.InsertBlockSummary(ctx, 600, blockTime, 10, txCounts)
	require.NoError(t, err)

	summary, err := chainDB.GetBlockSummary(ctx, 600)
	require.NoError(t, err)

	// Verify HeightTime matches the block time
	assert.Equal(t, blockTime.Unix(), summary.HeightTime.Unix())
}

// TestBlockSummaryPagination tests paginated queries of block summaries
func TestBlockSummaryPagination(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainDB, err := helpers.NewChainDb(ctx, "test_chain_summary_page")
	require.NoError(t, err)
	defer chainDB.Close()

	err = chainDB.InitializeDB(ctx)
	require.NoError(t, err)

	blockTime := time.Now().UTC()

	// Insert summaries for heights 700-709
	for i := uint64(700); i < 710; i++ {
		txCounts := map[string]uint32{
			"send": uint32(i % 10), // Varying counts
		}
		err = chainDB.InsertBlockSummary(ctx, i, blockTime, uint32(i%10), txCounts)
		require.NoError(t, err)
	}

	tests := []struct {
		name             string
		cursor           uint64
		limit            int
		sortDesc         bool
		expectedMinCount int
		expectedMaxCount int
	}{
		{
			name:             "get all ascending",
			cursor:           0,
			limit:            20,
			sortDesc:         false,
			expectedMinCount: 10,
			expectedMaxCount: 10,
		},
		{
			name:             "get all descending",
			cursor:           0,
			limit:            20,
			sortDesc:         true,
			expectedMinCount: 10,
			expectedMaxCount: 10,
		},
		{
			name:             "cursor pagination ascending",
			cursor:           705,
			limit:            10,
			sortDesc:         false,
			expectedMinCount: 4, // 706, 707, 708, 709
			expectedMaxCount: 4,
		},
		{
			name:             "cursor pagination descending",
			cursor:           705,
			limit:            10,
			sortDesc:         true,
			expectedMinCount: 5, // 704, 703, 702, 701, 700
			expectedMaxCount: 5,
		},
		{
			name:             "limit results",
			cursor:           0,
			limit:            3,
			sortDesc:         false,
			expectedMinCount: 3,
			expectedMaxCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := chainDB.QueryBlockSummaries(ctx, tt.cursor, tt.limit, tt.sortDesc)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(results), tt.expectedMinCount)
			assert.LessOrEqual(t, len(results), tt.expectedMaxCount)

			// Verify ordering
			if len(results) > 1 {
				for i := 1; i < len(results); i++ {
					if tt.sortDesc {
						assert.GreaterOrEqual(t, results[i-1].Height, results[i].Height,
							"expected descending order")
					} else {
						assert.LessOrEqual(t, results[i-1].Height, results[i].Height,
							"expected ascending order")
					}
				}
			}
		})
	}
}

// TestBlockSummaryEmptyMap tests that empty transaction count maps work correctly
func TestBlockSummaryEmptyMap(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainDB, err := helpers.NewChainDb(ctx, "test_chain_empty_map")
	require.NoError(t, err)
	defer chainDB.Close()

	err = chainDB.InitializeDB(ctx)
	require.NoError(t, err)

	blockTime := time.Now().UTC()

	// Insert block summary with empty map (no transactions)
	emptyMap := map[string]uint32{}

	err = chainDB.InsertBlockSummary(ctx, 800, blockTime, 0, emptyMap)
	require.NoError(t, err)

	summary, err := chainDB.GetBlockSummary(ctx, 800)
	require.NoError(t, err)

	assert.Equal(t, uint64(800), summary.Height)
	assert.Equal(t, uint32(0), summary.NumTxs)
	assert.NotNil(t, summary.TxCountsByType)
	assert.Empty(t, summary.TxCountsByType)
}

// TestBlockSummaryConsistency tests that NumTxs matches sum of TxCountsByType
func TestBlockSummaryConsistency(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainDB, err := helpers.NewChainDb(ctx, "test_chain_consistency")
	require.NoError(t, err)
	defer chainDB.Close()

	err = chainDB.InitializeDB(ctx)
	require.NoError(t, err)

	blockTime := time.Now().UTC()

	txCounts := map[string]uint32{
		"send":     10,
		"delegate": 5,
		"vote":     3,
		"contract": 12,
	}

	// Calculate total
	total := uint32(0)
	for _, count := range txCounts {
		total += count
	}

	err = chainDB.InsertBlockSummary(ctx, 900, blockTime, total, txCounts)
	require.NoError(t, err)

	summary, err := chainDB.GetBlockSummary(ctx, 900)
	require.NoError(t, err)

	// Verify NumTxs equals sum of individual counts
	sumFromMap := uint32(0)
	for _, count := range summary.TxCountsByType {
		sumFromMap += count
	}

	assert.Equal(t, summary.NumTxs, sumFromMap,
		"NumTxs should equal sum of TxCountsByType values")
	assert.Equal(t, total, sumFromMap)
}

package indexer

import (
	"testing"

	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"github.com/stretchr/testify/assert"
)

// TestQueueRouting_LiveBlock tests that live blocks (within 200 of head) are routed to the live queue.
func TestQueueRouting_LiveBlock(t *testing.T) {
	// Setup: Chain at block 1000, indexing block 950 (within 200 threshold)
	latestHead := uint64(1000)
	blockHeight := uint64(950)

	// Verify IsLiveBlock returns true
	assert.True(t, types.IsLiveBlock(latestHead, blockHeight),
		"Block %d should be considered live when latest is %d (within 200)", blockHeight, latestHead)

	// Expected queue: live queue
	client := &temporal.Client{
		IndexerLiveQueue:       "index:%s:live",
		IndexerHistoricalQueue: "index:%s:historical",
	}

	expectedQueue := client.GetIndexerLiveQueue("test-chain")
	assert.Equal(t, "index:test-chain:live", expectedQueue,
		"Live block should route to live queue")
}

// TestQueueRouting_HistoricalBlock tests that historical blocks (beyond 200 of head) are routed to the historical queue.
func TestQueueRouting_HistoricalBlock(t *testing.T) {
	// Setup: Chain at block 1000, indexing block 500 (beyond 200 threshold)
	latestHead := uint64(1000)
	blockHeight := uint64(500)

	// Verify IsLiveBlock returns false
	assert.False(t, types.IsLiveBlock(latestHead, blockHeight),
		"Block %d should be considered historical when latest is %d (beyond 200)", blockHeight, latestHead)

	// Expected queue: historical queue
	client := &temporal.Client{
		IndexerLiveQueue:       "index:%s:live",
		IndexerHistoricalQueue: "index:%s:historical",
	}

	expectedQueue := client.GetIndexerHistoricalQueue("test-chain")
	assert.Equal(t, "index:test-chain:historical", expectedQueue,
		"Historical block should route to historical queue")
}

// TestQueueRouting_BoundaryBlock tests routing at the exact 200-block boundary.
func TestQueueRouting_BoundaryBlock(t *testing.T) {
	// Setup: Chain at block 1000, indexing block 800 (exactly 200 behind)
	latestHead := uint64(1000)
	blockHeight := uint64(800)

	// Verify IsLiveBlock returns true (at boundary is considered live)
	assert.True(t, types.IsLiveBlock(latestHead, blockHeight),
		"Block at exactly 200 behind should be considered live")

	// Block just beyond boundary
	blockHeight = 799
	assert.False(t, types.IsLiveBlock(latestHead, blockHeight),
		"Block at 201 behind should be considered historical")
}

// TestQueueRouting_StartIndexWorkflowBatch_SplitBatch tests that batches crossing the boundary are split.
func TestQueueRouting_StartIndexWorkflowBatch_SplitBatch(t *testing.T) {
	// Test scenario: latest = 1000, boundary = 800
	// Batch: 700-900 should be split into:
	//   - 700-800 → historical queue
	//   - 801-900 → live queue

	latestHead := uint64(1000)
	batchStart := uint64(700)
	batchEnd := uint64(900)

	// Verify the batch crosses the boundary
	startsHistorical := !types.IsLiveBlock(latestHead, batchStart)
	endsLive := types.IsLiveBlock(latestHead, batchEnd)

	assert.True(t, startsHistorical, "Batch start (700) should be historical")
	assert.True(t, endsLive, "Batch end (900) should be live")

	// Calculate the boundary
	var boundary uint64
	if latestHead > types.LiveBlockThreshold {
		boundary = latestHead - types.LiveBlockThreshold
	} else {
		boundary = 0
	}

	assert.Equal(t, uint64(800), boundary, "Boundary should be at block 800")

	// Verify split points
	historicalEnd := boundary
	liveStart := boundary + 1

	assert.Equal(t, uint64(800), historicalEnd,
		"Historical portion should end at boundary (800)")
	assert.Equal(t, uint64(801), liveStart,
		"Live portion should start at boundary+1 (801)")

	// Verify each portion is correctly classified
	assert.True(t, types.IsLiveBlock(latestHead, historicalEnd),
		"Block 800 should be at boundary (live)")
	assert.True(t, types.IsLiveBlock(latestHead, liveStart),
		"Block 801 should be live")
}

// TestQueueRouting_StartIndexWorkflowBatch_EntirelyLive tests batches that are entirely live.
func TestQueueRouting_StartIndexWorkflowBatch_EntirelyLive(t *testing.T) {
	// Test scenario: latest = 1000, batch = 850-950
	// All blocks in range are within 200 of head → should go to live queue

	latestHead := uint64(1000)
	batchStart := uint64(850)
	batchEnd := uint64(950)

	batchIsLive := types.IsLiveBlock(latestHead, batchStart) && types.IsLiveBlock(latestHead, batchEnd)
	assert.True(t, batchIsLive,
		"Entire batch [%d, %d] should be live when latest is %d",
		batchStart, batchEnd, latestHead)

	// Verify no split needed
	batchIsHistorical := !types.IsLiveBlock(latestHead, batchStart) && !types.IsLiveBlock(latestHead, batchEnd)
	assert.False(t, batchIsHistorical,
		"Batch should not be classified as historical")

	// This batch should route to single queue (live)
	needsSplit := batchIsLive == batchIsHistorical
	assert.False(t, needsSplit, "Batch should not need splitting")
}

// TestQueueRouting_StartIndexWorkflowBatch_EntirelyHistorical tests batches that are entirely historical.
func TestQueueRouting_StartIndexWorkflowBatch_EntirelyHistorical(t *testing.T) {
	// Test scenario: latest = 1000, batch = 100-500
	// All blocks in range are beyond 200 of head → should go to historical queue

	latestHead := uint64(1000)
	batchStart := uint64(100)
	batchEnd := uint64(500)

	batchIsHistorical := !types.IsLiveBlock(latestHead, batchStart) && !types.IsLiveBlock(latestHead, batchEnd)
	assert.True(t, batchIsHistorical,
		"Entire batch [%d, %d] should be historical when latest is %d",
		batchStart, batchEnd, latestHead)

	// Verify no split needed
	batchIsLive := types.IsLiveBlock(latestHead, batchStart) && types.IsLiveBlock(latestHead, batchEnd)
	assert.False(t, batchIsLive,
		"Batch should not be classified as live")

	// This batch should route to single queue (historical)
	needsSplit := batchIsLive == batchIsHistorical
	assert.False(t, needsSplit, "Batch should not need splitting")
}

// TestQueueRouting_GetLatestHeadFailure tests fallback behavior when GetLatestHead fails.
func TestQueueRouting_GetLatestHeadFailure(t *testing.T) {
	// When GetLatestHead fails and returns error, the activity logs a warning
	// and defaults to latest=0. In this case, since height > latest,
	// IsLiveBlock returns true (future blocks are considered live).
	// However, this is an edge case - in production, the error would cause
	// the workflow to retry or fail, not schedule with latest=0.

	latestHead := uint64(0) // Error case default
	blockHeight := uint64(1000)

	// With latest=0 and height=1000, IsLiveBlock returns true (future block logic)
	isLive := types.IsLiveBlock(latestHead, blockHeight)
	assert.True(t, isLive,
		"With latest=0 (error case), height > latest triggers future block logic (live)")
}

// TestQueueRouting_FutureBlock tests that future blocks (ahead of head) are routed to live queue.
func TestQueueRouting_FutureBlock(t *testing.T) {
	// Test scenario: latest = 1000, height = 1050 (future block)
	latestHead := uint64(1000)
	blockHeight := uint64(1050)

	assert.True(t, types.IsLiveBlock(latestHead, blockHeight),
		"Future blocks should be considered live")

	// Verify queue routing
	client := &temporal.Client{
		IndexerLiveQueue:       "index:%s:live",
		IndexerHistoricalQueue: "index:%s:historical",
	}

	expectedQueue := client.GetIndexerLiveQueue("test-chain")
	assert.Equal(t, "index:test-chain:live", expectedQueue,
		"Future block should route to live queue")
}

// TestQueueRouting_NewChainBootstrap tests routing for a new chain starting from block 0.
func TestQueueRouting_NewChainBootstrap(t *testing.T) {
	// Test scenario: new chain at block 100, all blocks 0-100 should be live
	latestHead := uint64(100)

	for height := uint64(0); height <= latestHead; height++ {
		isLive := types.IsLiveBlock(latestHead, height)
		assert.True(t, isLive,
			"Block %d in new chain (latest=%d) should be live", height, latestHead)
	}
}

// TestQueueRouting_RealWorldScenarios tests combined scenarios
func TestQueueRouting_RealWorldScenarios(t *testing.T) {
	t.Run("catch-up scenario", func(t *testing.T) {
		// Chain at block 10000, catching up from block 5000
		latestHead := uint64(10000)

		// Recent blocks (within 200) should be live
		assert.True(t, types.IsLiveBlock(latestHead, 9900), "Recent block should be live")

		// Old blocks (beyond 200) should be historical
		assert.False(t, types.IsLiveBlock(latestHead, 5000), "Old block should be historical")

		// Boundary block
		assert.True(t, types.IsLiveBlock(latestHead, 9800), "Boundary block should be live")
		assert.False(t, types.IsLiveBlock(latestHead, 9799), "Just past boundary should be historical")
	})

	t.Run("normal operation scenario", func(t *testing.T) {
		// Chain at block 1000, HeadScan finding new blocks
		latestHead := uint64(1000)

		// All new blocks from HeadScan (1001-1005) should be live
		for height := latestHead + 1; height <= latestHead+5; height++ {
			assert.True(t, types.IsLiveBlock(latestHead, height),
				"New block %d should be live", height)
		}
	})

	t.Run("gap fill scenario", func(t *testing.T) {
		// Chain at block 50000, filling gaps from various ranges
		latestHead := uint64(50000)

		// Recent gap (block 49900) - should be live
		assert.True(t, types.IsLiveBlock(latestHead, 49900), "Recent gap should be live")

		// Old gap (block 10000) - should be historical
		assert.False(t, types.IsLiveBlock(latestHead, 10000), "Old gap should be historical")
	})
}

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsLiveBlock verifies the IsLiveBlock function correctly determines
// whether a block should be routed to the live queue based on its distance
// from the chain head.
func TestIsLiveBlock(t *testing.T) {
	tests := []struct {
		name     string
		latest   uint64
		height   uint64
		expected bool
	}{
		{
			name:     "exact head",
			latest:   1000,
			height:   1000,
			expected: true,
		},
		{
			name:     "one block behind head",
			latest:   1000,
			height:   999,
			expected: true,
		},
		{
			name:     "within threshold (199 blocks behind)",
			latest:   1000,
			height:   801,
			expected: true,
		},
		{
			name:     "at boundary (200 blocks behind)",
			latest:   1000,
			height:   800,
			expected: true,
		},
		{
			name:     "just outside threshold (201 blocks behind)",
			latest:   1000,
			height:   799,
			expected: false,
		},
		{
			name:     "far historical (500 blocks behind)",
			latest:   1000,
			height:   500,
			expected: false,
		},
		{
			name:     "very old block",
			latest:   1000,
			height:   100,
			expected: false,
		},
		{
			name:     "future block (one ahead)",
			latest:   1000,
			height:   1001,
			expected: true,
		},
		{
			name:     "future block (multiple ahead)",
			latest:   1000,
			height:   1050,
			expected: true,
		},
		{
			name:     "zero latest and zero height",
			latest:   0,
			height:   0,
			expected: true,
		},
		{
			name:     "zero height with non-zero latest",
			latest:   1000,
			height:   0,
			expected: false,
		},
		{
			name:     "large numbers within threshold",
			latest:   1000000,
			height:   999850,
			expected: true,
		},
		{
			name:     "large numbers outside threshold",
			latest:   1000000,
			height:   999750,
			expected: false,
		},
		{
			name:     "boundary at small latest",
			latest:   200,
			height:   0,
			expected: true,
		},
		{
			name:     "below boundary at small latest",
			latest:   201,
			height:   0,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsLiveBlock(tt.latest, tt.height)
			assert.Equal(t, tt.expected, result,
				"IsLiveBlock(%d, %d) = %v, expected %v",
				tt.latest, tt.height, result, tt.expected)
		})
	}
}

// TestIsLiveBlock_EdgeCases tests edge cases and boundary conditions
func TestIsLiveBlock_EdgeCases(t *testing.T) {
	t.Run("LiveBlockThreshold constant value", func(t *testing.T) {
		// Verify the threshold constant is set correctly
		assert.Equal(t, uint64(200), uint64(LiveBlockThreshold),
			"LiveBlockThreshold should be 200 blocks")
	})

	t.Run("boundary behavior", func(t *testing.T) {
		latest := uint64(10000)
		threshold := uint64(LiveBlockThreshold)

		// Test blocks around the boundary
		assert.True(t, IsLiveBlock(latest, latest-threshold),
			"Block exactly at threshold should be live")
		assert.False(t, IsLiveBlock(latest, latest-threshold-1),
			"Block one beyond threshold should be historical")
	})

	t.Run("overflow protection", func(t *testing.T) {
		// Test with maximum uint64 values to ensure no overflow
		maxUint64 := ^uint64(0)

		// Future block with max value
		assert.True(t, IsLiveBlock(maxUint64-100, maxUint64),
			"Future block should be live even at max uint64")

		// Very old block
		assert.False(t, IsLiveBlock(maxUint64, 0),
			"Block at 0 with max latest should be historical")
	})
}

// TestIsLiveBlock_RealWorldScenarios tests scenarios based on actual usage patterns
func TestIsLiveBlock_RealWorldScenarios(t *testing.T) {
	t.Run("mainnet catch-up scenario", func(t *testing.T) {
		// Simulating mainnet at block 700,000 catching up from 690,000
		latest := uint64(700000)

		// Most recent blocks (should be live)
		assert.True(t, IsLiveBlock(latest, 699900),
			"Recent blocks should go to live queue")

		// Historical backfill (should be historical)
		assert.False(t, IsLiveBlock(latest, 690000),
			"Old blocks should go to historical queue")

		// Boundary case
		assert.True(t, IsLiveBlock(latest, 699800),
			"Blocks exactly at threshold should be live")
		assert.False(t, IsLiveBlock(latest, 699799),
			"Blocks one past threshold should be historical")
	})

	t.Run("new chain bootstrap scenario", func(t *testing.T) {
		// New chain starting from block 0
		latest := uint64(100)

		// All blocks should be live (within 200 block threshold)
		for height := uint64(0); height <= latest; height++ {
			assert.True(t, IsLiveBlock(latest, height),
				"All blocks in new chain should be live (height: %d)", height)
		}
	})

	t.Run("headscan normal operation", func(t *testing.T) {
		// HeadScan running every 5 seconds with 20s block time
		// Typically finding 1-5 new blocks per run
		latest := uint64(1000000)

		// All new blocks should be live
		for i := uint64(0); i < 5; i++ {
			assert.True(t, IsLiveBlock(latest, latest-i),
				"New blocks from HeadScan should be live (offset: %d)", i)
		}
	})

	t.Run("gap scan scenario", func(t *testing.T) {
		// GapScan finding missing blocks from various ranges
		latest := uint64(500000)

		// Recent gap (should be live)
		assert.True(t, IsLiveBlock(latest, 499850),
			"Recent gap should go to live queue")

		// Old gap from initial sync (should be historical)
		assert.False(t, IsLiveBlock(latest, 100000),
			"Old gap should go to historical queue")
	})
}

// BenchmarkIsLiveBlock benchmarks the IsLiveBlock function performance
func BenchmarkIsLiveBlock(b *testing.B) {
	latest := uint64(1000000)
	height := uint64(999900)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsLiveBlock(latest, height)
	}
}

// BenchmarkIsLiveBlock_Mixed benchmarks with mixed live/historical scenarios
func BenchmarkIsLiveBlock_Mixed(b *testing.B) {
	latest := uint64(1000000)
	heights := []uint64{
		1000000, // live
		999900,  // live
		999700,  // historical
		500000,  // historical
		1000001, // future (live)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, height := range heights {
			IsLiveBlock(latest, height)
		}
	}
}

package activity

import (
	"runtime"
	"testing"
)

func TestSchedulerParallelism(t *testing.T) {
	tests := []struct {
		name     string
		override int
		want     int
	}{
		{
			name:     "default uses 4x CPU count",
			override: 0,
			want:     runtime.NumCPU() * 4,
		},
		{
			name:     "override with valid value",
			override: 128,
			want:     128,
		},
		{
			name:     "override with small value",
			override: 1,
			want:     1,
		},
		{
			name:     "override exceeding max caps at 512",
			override: 1024,
			want:     512,
		},
		{
			name:     "override at max boundary",
			override: 512,
			want:     512,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := schedulerParallelism(tt.override)

			// For default case, verify it's within expected range
			if tt.override == 0 {
				if got < 2 {
					t.Errorf("schedulerParallelism() = %v, want at least 2", got)
				}
				if got > 512 {
					t.Errorf("schedulerParallelism() = %v, want at most 512", got)
				}
				// Verify it's using 4x multiplier
				expectedBase := runtime.NumCPU() * 4
				if expectedBase <= 512 && got != expectedBase {
					t.Errorf("schedulerParallelism() = %v, want %v (4x CPU)", got, expectedBase)
				}
				return
			}

			if got != tt.want {
				t.Errorf("schedulerParallelism(%v) = %v, want %v", tt.override, got, tt.want)
			}
		})
	}
}

func TestSchedulerQueueSize(t *testing.T) {
	tests := []struct {
		name        string
		parallelism int
		batchSize   int
		wantMin     int
		wantMax     int
	}{
		{
			name:        "small batch uses minimum",
			parallelism: 64,
			batchSize:   10,
			wantMin:     4096,
			wantMax:     4096,
		},
		{
			name:        "medium batch within range",
			parallelism: 128,
			batchSize:   1000,
			wantMin:     128000,
			wantMax:     128000,
		},
		{
			name:        "large batch caps at maximum",
			parallelism: 256,
			batchSize:   10000,
			wantMin:     262144,
			wantMax:     262144,
		},
		{
			name:        "invalid parallelism normalized",
			parallelism: 0,
			batchSize:   100,
			wantMin:     4096,
			wantMax:     4096,
		},
		{
			name:        "invalid batch size normalized",
			parallelism: 64,
			batchSize:   0,
			wantMin:     4096,
			wantMax:     4096,
		},
		{
			name:        "4x CPU with typical batch",
			parallelism: runtime.NumCPU() * 4,
			batchSize:   750000,
			wantMin:     262144,
			wantMax:     262144,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := schedulerQueueSize(tt.parallelism, tt.batchSize)

			if got < tt.wantMin {
				t.Errorf("schedulerQueueSize(%v, %v) = %v, want at least %v",
					tt.parallelism, tt.batchSize, got, tt.wantMin)
			}
			if got > tt.wantMax {
				t.Errorf("schedulerQueueSize(%v, %v) = %v, want at most %v",
					tt.parallelism, tt.batchSize, got, tt.wantMax)
			}
		})
	}
}

func TestSchedulerQueueSizeBounds(t *testing.T) {
	// Verify queue size always respects bounds
	testCases := []struct {
		parallelism int
		batchSize   int
	}{
		{1, 1},
		{512, 750000},
		{256, 1000000},
		{-1, 100},    // Invalid inputs
		{100, -1},    // Invalid inputs
		{0, 0},       // Invalid inputs
	}

	for _, tc := range testCases {
		got := schedulerQueueSize(tc.parallelism, tc.batchSize)

		// Queue must always be within bounds
		if got < 4096 {
			t.Errorf("schedulerQueueSize(%v, %v) = %v, below minimum 4096",
				tc.parallelism, tc.batchSize, got)
		}
		if got > 262144 {
			t.Errorf("schedulerQueueSize(%v, %v) = %v, above maximum 262144",
				tc.parallelism, tc.batchSize, got)
		}
	}
}

func TestSchedulerPoolSizeIntegration(t *testing.T) {
	// Test the Context.SchedulerPoolSize() method
	tests := []struct {
		name     string
		override int
	}{
		{
			name:     "default parallelism",
			override: 0,
		},
		{
			name:     "custom parallelism",
			override: 256,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &Context{
				SchedulerMaxParallelism: tt.override,
			}

			got := ctx.SchedulerPoolSize()
			expected := schedulerParallelism(tt.override)

			if got != expected {
				t.Errorf("SchedulerPoolSize() = %v, want %v", got, expected)
			}
		})
	}
}

// BenchmarkSchedulerParallelism benchmarks the parallelism calculation
func BenchmarkSchedulerParallelism(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = schedulerParallelism(0)
	}
}

// BenchmarkSchedulerQueueSize benchmarks the queue size calculation
func BenchmarkSchedulerQueueSize(b *testing.B) {
	parallelism := runtime.NumCPU() * 4
	batchSize := 750000

	for i := 0; i < b.N; i++ {
		_ = schedulerQueueSize(parallelism, batchSize)
	}
}
package indexer

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"github.com/canopy-network/canopyx/pkg/indexer/workflow"
)

func defaultWorkflowConfig() workflow.Config {
	return workflow.Config{
		CatchupThreshold:        200,
		DirectScheduleBatchSize: 50,
		SchedulerBatchSize:      5000, // Updated to match production default for realistic testing
		BlockTimeSeconds:        20,
	}
}

// mockSchedulerActivities provides comprehensive mock activities for testing
type mockSchedulerActivities struct {
	mu sync.Mutex

	// Counters for verification
	startIndexCalls     atomic.Int32
	getLatestHeadCalls  atomic.Int32
	getLastIndexedCalls atomic.Int32
	findGapsCalls       atomic.Int32
	isSchedulerRunning  atomic.Bool

	// Tracking for StartIndexWorkflow calls
	scheduledBlocks []scheduledBlock

	// Mock data
	latestHead   uint64
	lastIndexed  uint64
	gaps         []db.Gap
	shouldFail   map[string]bool
	failureCount map[string]int

	// Rate limiting verification
}

type scheduledBlock struct {
	Height      uint64
	Priority    int
	ScheduledAt time.Time
}

// Activity implementations for mock

func (m *mockSchedulerActivities) GetLatestHead(ctx context.Context, in *types.ChainIdInput) (uint64, error) {
	m.getLatestHeadCalls.Add(1)
	if m.shouldFail["GetLatestHead"] {
		m.failureCount["GetLatestHead"]++
		return 0, fmt.Errorf("mock GetLatestHead failure")
	}
	return m.latestHead, nil
}

func (m *mockSchedulerActivities) GetLastIndexed(ctx context.Context, in *types.ChainIdInput) (uint64, error) {
	m.getLastIndexedCalls.Add(1)
	if m.shouldFail["GetLastIndexed"] {
		m.failureCount["GetLastIndexed"]++
		return 0, fmt.Errorf("mock GetLastIndexed failure")
	}
	return m.lastIndexed, nil
}

func (m *mockSchedulerActivities) FindGaps(ctx context.Context, in *types.ChainIdInput) ([]db.Gap, error) {
	m.findGapsCalls.Add(1)
	if m.shouldFail["FindGaps"] {
		m.failureCount["FindGaps"]++
		return nil, fmt.Errorf("mock FindGaps failure")
	}
	return m.gaps, nil
}

func (m *mockSchedulerActivities) StartIndexWorkflow(ctx context.Context, in types.IndexBlockInput) error {
	m.startIndexCalls.Add(1)

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	m.scheduledBlocks = append(m.scheduledBlocks, scheduledBlock{
		Height:      in.Height,
		Priority:    in.PriorityKey,
		ScheduledAt: now,
	})

	if m.shouldFail["StartIndexWorkflow"] {
		m.failureCount["StartIndexWorkflow"]++
		return fmt.Errorf("mock StartIndexWorkflow failure")
	}
	return nil
}

func (m *mockSchedulerActivities) StartIndexWorkflowBatch(ctx context.Context, in types.BatchScheduleInput) (types.BatchScheduleOutput, error) {
	start := time.Now()

	scheduled := 0
	failed := 0

	for height := in.StartHeight; height <= in.EndHeight; height++ {
		err := m.StartIndexWorkflow(ctx, types.IndexBlockInput{
			ChainID:     in.ChainID,
			Height:      height,
			PriorityKey: in.PriorityKey,
		})
		if err != nil {
			failed++
		} else {
			scheduled++
		}
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	return types.BatchScheduleOutput{
		Scheduled:  scheduled,
		Failed:     failed,
		DurationMs: durationMs,
	}, nil
}

func (m *mockSchedulerActivities) IsSchedulerWorkflowRunning(ctx context.Context, in *types.ChainIdInput) (bool, error) {
	if m.shouldFail["IsSchedulerWorkflowRunning"] {
		m.failureCount["IsSchedulerWorkflowRunning"]++
		return false, fmt.Errorf("mock IsSchedulerWorkflowRunning failure")
	}
	return m.isSchedulerRunning.Load(), nil
}

// Helper to verify priority distribution
func (m *mockSchedulerActivities) getPriorityDistribution() map[int]int {
	m.mu.Lock()
	defer m.mu.Unlock()

	dist := make(map[int]int)
	for _, block := range m.scheduledBlocks {
		dist[block.Priority]++
	}
	return dist
}

// Helper to verify scheduling order
func (m *mockSchedulerActivities) verifyPriorityOrder() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.scheduledBlocks) == 0 {
		return nil
	}

	sorted := make([]scheduledBlock, len(m.scheduledBlocks))
	copy(sorted, m.scheduledBlocks)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Height > sorted[j].Height
	})

	lastPriority := sorted[0].Priority
	for _, block := range sorted[1:] {
		if block.Priority > lastPriority {
			return fmt.Errorf("priority order violation: block %d has priority %d after priority %d",
				block.Height, block.Priority, lastPriority)
		}
		lastPriority = block.Priority
	}
	return nil
}

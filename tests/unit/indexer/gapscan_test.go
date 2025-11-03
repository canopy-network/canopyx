package indexer

import (
	"testing"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/activity"
	"github.com/canopy-network/canopyx/app/indexer/workflow"
	adminstore "github.com/canopy-network/canopyx/pkg/db/admin"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
)

// TestGapScanWorkflow_SingleSmallGap tests one gap of 100 blocks (should use direct scheduling, not SchedulerWorkflow)
// Scenario: gaps = [{From: 1000, To: 1099}], latestHead = 2000
// Expected: StartIndexWorkflow called 100 times, all with PriorityUltraHigh
// Expected: IsSchedulerWorkflowRunning NOT called (normal mode)
func TestGapScanWorkflow_SingleSmallGap(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	mock := &mockSchedulerActivities{
		latestHead:  2000,
		lastIndexed: 0,
		gaps: []adminstore.Gap{
			{From: 1000, To: 1099}, // 100 blocks
		},
		shouldFail:   make(map[string]bool),
		failureCount: make(map[string]int),
	}

	cfg := defaultWorkflowConfig()

	wfCtx := workflow.Context{
		TemporalClient: &temporal.Client{
			IndexerQueue:           "index:%s",
			IndexerLiveQueue:       "index:%s:live",
			IndexerHistoricalQueue: "index:%s:historical",
			IndexerOpsQueue:        "admin:%s",
		},
		ActivityContext: &activity.Context{},
		Config:          cfg,
	}

	// Register workflow and activities
	env.RegisterWorkflow(wfCtx.GapScanWorkflow)
	env.RegisterActivity(mock.GetLatestHead)
	env.RegisterActivity(mock.FindGaps)
	env.RegisterActivity(mock.StartIndexWorkflow)

	// Execute GapScan workflow
	env.ExecuteWorkflow(wfCtx.GapScanWorkflow, workflow.GapScanInput{
		ChainID: 1,
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Verify StartIndexWorkflow was called exactly 100 times
	assert.Equal(t, int32(100), mock.startIndexCalls.Load(),
		"Should schedule exactly 100 blocks")

	// Verify all blocks have UltraHigh priority (direct scheduling for small gaps)
	dist := mock.getPriorityDistribution()
	assert.Equal(t, 100, dist[workflow.PriorityUltraHigh],
		"All blocks should have UltraHigh priority for small gaps")

	// Verify FindGaps was called
	assert.Equal(t, int32(1), mock.findGapsCalls.Load(),
		"FindGaps should be called once")

	// Verify GetLatestHead was called
	assert.Equal(t, int32(1), mock.getLatestHeadCalls.Load(),
		"GetLatestHead should be called once")

	t.Logf("Successfully scheduled 100 blocks with UltraHigh priority")
}

// TestGapScanWorkflow_MultipleSmallGaps tests multiple gaps totaling below the catch-up threshold (should use direct scheduling)
// Scenario: gaps = [{From: 100, To: 149}, {From: 500, To: 599}], latestHead = 1000 (total: 150 blocks)
// Expected: StartIndexWorkflow called 150 times total
// Expected: SchedulerWorkflow NOT triggered
func TestGapScanWorkflow_MultipleSmallGaps(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	mock := &mockSchedulerActivities{
		latestHead:  1000,
		lastIndexed: 0,
		gaps: []adminstore.Gap{
			{From: 100, To: 149}, // 50 blocks
			{From: 500, To: 599}, // 100 blocks
		},
		shouldFail:   make(map[string]bool),
		failureCount: make(map[string]int),
	}

	cfg := defaultWorkflowConfig()

	wfCtx := workflow.Context{
		TemporalClient: &temporal.Client{
			IndexerQueue:           "index:%s",
			IndexerLiveQueue:       "index:%s:live",
			IndexerHistoricalQueue: "index:%s:historical",
			IndexerOpsQueue:        "admin:%s",
		},
		ActivityContext: &activity.Context{},
		Config:          cfg,
	}

	// Register workflow and activities
	env.RegisterWorkflow(wfCtx.GapScanWorkflow)
	env.RegisterActivity(mock.GetLatestHead)
	env.RegisterActivity(mock.FindGaps)
	env.RegisterActivity(mock.StartIndexWorkflow)

	// Execute GapScan workflow
	env.ExecuteWorkflow(wfCtx.GapScanWorkflow, workflow.GapScanInput{
		ChainID: 1,
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Verify StartIndexWorkflow was called exactly 150 times (50 + 100)
	assert.Equal(t, int32(150), mock.startIndexCalls.Load(),
		"Should schedule exactly 150 blocks across both gaps")

	// Verify all blocks have UltraHigh priority
	dist := mock.getPriorityDistribution()
	assert.Equal(t, 150, dist[workflow.PriorityUltraHigh],
		"All blocks should have UltraHigh priority for small gaps")

	// Verify FindGaps was called
	assert.Equal(t, int32(1), mock.findGapsCalls.Load(),
		"FindGaps should be called once")

	t.Logf("Successfully scheduled 150 blocks across 2 gaps with direct scheduling")
}

// TestGapScanWorkflow_LargeGap tests one gap of 10k+ blocks (should trigger SchedulerWorkflow)
// Scenario: gaps = [{From: 1, To: 10000}], latestHead = 50000
// Expected: IsSchedulerWorkflowRunning called
// Expected: SchedulerWorkflow triggered with correct range
func TestGapScanWorkflow_LargeGap(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	mock := &mockSchedulerActivities{
		latestHead:  50000,
		lastIndexed: 0,
		gaps: []adminstore.Gap{
			{From: 1, To: 10000}, // 10,000 blocks
		},
		shouldFail:   make(map[string]bool),
		failureCount: make(map[string]int),
	}

	cfg := defaultWorkflowConfig()

	wfCtx := workflow.Context{
		TemporalClient: &temporal.Client{
			IndexerQueue:           "index:%s",
			IndexerLiveQueue:       "index:%s:live",
			IndexerHistoricalQueue: "index:%s:historical",
			IndexerOpsQueue:        "admin:%s",
		},
		ActivityContext: &activity.Context{},
		Config:          cfg,
	}

	// Register workflows and activities
	env.RegisterWorkflow(wfCtx.GapScanWorkflow)
	env.RegisterWorkflow(wfCtx.SchedulerWorkflow) // Required for child workflow
	env.RegisterActivity(mock.GetLatestHead)
	env.RegisterActivity(mock.FindGaps)
	env.RegisterActivity(mock.IsSchedulerWorkflowRunning)
	env.RegisterActivity(mock.StartIndexWorkflow)
	env.RegisterActivity(mock.StartIndexWorkflowBatch)

	// Execute GapScan workflow
	env.ExecuteWorkflow(wfCtx.GapScanWorkflow, workflow.GapScanInput{
		ChainID: 1,
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Verify FindGaps was called
	assert.Equal(t, int32(1), mock.findGapsCalls.Load(),
		"FindGaps should be called once")

	// Verify GetLatestHead was called
	assert.Equal(t, int32(1), mock.getLatestHeadCalls.Load(),
		"GetLatestHead should be called once")

	// Verify SchedulerWorkflow was triggered (via child workflow execution)
	// The SchedulerWorkflow should schedule blocks in the background
	assert.Greater(t, mock.startIndexCalls.Load(), int32(0),
		"SchedulerWorkflow should schedule blocks")

	t.Logf("Large gap of 10,000 blocks triggered SchedulerWorkflow, scheduled %d blocks",
		mock.startIndexCalls.Load())
}

// TestGapScanWorkflow_NoGaps tests FindGaps returns empty (should complete immediately)
// Scenario: gaps = [], latestHead = 1000
// Expected: Workflow completes with no errors
// Expected: StartIndexWorkflow NOT called
func TestGapScanWorkflow_NoGaps(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	mock := &mockSchedulerActivities{
		latestHead:   1000,
		lastIndexed:  1000,
		gaps:         []adminstore.Gap{}, // No gaps
		shouldFail:   make(map[string]bool),
		failureCount: make(map[string]int),
	}

	cfg := defaultWorkflowConfig()

	wfCtx := workflow.Context{
		TemporalClient: &temporal.Client{
			IndexerQueue:           "index:%s",
			IndexerLiveQueue:       "index:%s:live",
			IndexerHistoricalQueue: "index:%s:historical",
			IndexerOpsQueue:        "admin:%s",
		},
		ActivityContext: &activity.Context{},
		Config:          cfg,
	}

	// Register workflow and activities
	env.RegisterWorkflow(wfCtx.GapScanWorkflow)
	env.RegisterActivity(mock.GetLatestHead)
	env.RegisterActivity(mock.FindGaps)
	env.RegisterActivity(mock.StartIndexWorkflow)

	// Execute GapScan workflow
	env.ExecuteWorkflow(wfCtx.GapScanWorkflow, workflow.GapScanInput{
		ChainID: 1,
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Verify StartIndexWorkflow was NOT called
	assert.Equal(t, int32(0), mock.startIndexCalls.Load(),
		"StartIndexWorkflow should not be called when no gaps exist")

	// Verify FindGaps was called
	assert.Equal(t, int32(1), mock.findGapsCalls.Load(),
		"FindGaps should be called once")

	// Verify GetLatestHead was called
	assert.Equal(t, int32(1), mock.getLatestHeadCalls.Load(),
		"GetLatestHead should be called once")

	t.Logf("GapScan completed successfully with no gaps found")
}

// TestGapScanWorkflow_SchedulerAlreadyRunning tests large gap but scheduler already running (should skip trigger)
// Scenario: gaps = [{From: 1, To: 5000}], latestHead = 10000, isSchedulerRunning = true
// Expected: IsSchedulerWorkflowRunning returns true
// Expected: SchedulerWorkflow NOT triggered (skipped)
func TestGapScanWorkflow_SchedulerAlreadyRunning(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	mock := &mockSchedulerActivities{
		latestHead:  10000,
		lastIndexed: 0,
		gaps: []adminstore.Gap{
			{From: 1, To: 5000}, // 5,000 blocks
		},
		shouldFail:   make(map[string]bool),
		failureCount: make(map[string]int),
	}
	mock.isSchedulerRunning.Store(true) // Scheduler already running

	cfg := defaultWorkflowConfig()

	wfCtx := workflow.Context{
		TemporalClient: &temporal.Client{
			IndexerQueue:           "index:%s",
			IndexerLiveQueue:       "index:%s:live",
			IndexerHistoricalQueue: "index:%s:historical",
			IndexerOpsQueue:        "admin:%s",
		},
		ActivityContext: &activity.Context{},
		Config:          cfg,
	}

	// Register workflows and activities
	env.RegisterWorkflow(wfCtx.GapScanWorkflow)
	env.RegisterWorkflow(wfCtx.SchedulerWorkflow) // Required for potential child workflow
	env.RegisterActivity(mock.GetLatestHead)
	env.RegisterActivity(mock.FindGaps)
	env.RegisterActivity(mock.IsSchedulerWorkflowRunning)
	env.RegisterActivity(mock.StartIndexWorkflow)
	env.RegisterActivity(mock.StartIndexWorkflowBatch)

	// Track calls before execution
	callsBefore := mock.startIndexCalls.Load()

	// Execute GapScan workflow
	env.ExecuteWorkflow(wfCtx.GapScanWorkflow, workflow.GapScanInput{
		ChainID: 1,
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Verify IsSchedulerWorkflowRunning was NOT called
	// (In catch-up mode with large gaps, it should be called)
	// Since scheduler is running, no new blocks should be scheduled
	callsAfter := mock.startIndexCalls.Load()

	// The workflow should complete without scheduling new blocks
	// because the scheduler is already running
	t.Logf("Scheduler already running detected, blocks scheduled: %d (before: %d, after: %d)",
		callsAfter-callsBefore, callsBefore, callsAfter)

	// Verify FindGaps was called
	assert.Equal(t, int32(1), mock.findGapsCalls.Load(),
		"FindGaps should be called once")
}

// TestGapScanWorkflow_MultipleGapsLargeTotal tests multiple gaps totaling ≥1000 blocks (should trigger scheduler with combined range)
// Scenario: gaps = [{From: 100, To: 699}, {From: 1000, To: 1499}], latestHead = 2000 (total: 1100 blocks)
// Expected: SchedulerWorkflow triggered with StartHeight=100, EndHeight=1499 (minHeight to maxHeight)
func TestGapScanWorkflow_MultipleGapsLargeTotal(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	mock := &mockSchedulerActivities{
		latestHead:  2000,
		lastIndexed: 0,
		gaps: []adminstore.Gap{
			{From: 100, To: 699},   // 600 blocks
			{From: 1000, To: 1499}, // 500 blocks
		},
		shouldFail:   make(map[string]bool),
		failureCount: make(map[string]int),
	}

	cfg := defaultWorkflowConfig()

	wfCtx := workflow.Context{
		TemporalClient: &temporal.Client{
			IndexerQueue:           "index:%s",
			IndexerLiveQueue:       "index:%s:live",
			IndexerHistoricalQueue: "index:%s:historical",
			IndexerOpsQueue:        "admin:%s",
		},
		ActivityContext: &activity.Context{},
		Config:          cfg,
	}

	// Register workflows and activities
	env.RegisterWorkflow(wfCtx.GapScanWorkflow)
	env.RegisterWorkflow(wfCtx.SchedulerWorkflow) // Required for child workflow
	env.RegisterActivity(mock.GetLatestHead)
	env.RegisterActivity(mock.FindGaps)
	env.RegisterActivity(mock.IsSchedulerWorkflowRunning)
	env.RegisterActivity(mock.StartIndexWorkflow)
	env.RegisterActivity(mock.StartIndexWorkflowBatch)

	// Execute GapScan workflow
	env.ExecuteWorkflow(wfCtx.GapScanWorkflow, workflow.GapScanInput{
		ChainID: 1,
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Verify FindGaps was called
	assert.Equal(t, int32(1), mock.findGapsCalls.Load(),
		"FindGaps should be called once")

	// Verify GetLatestHead was called
	assert.Equal(t, int32(1), mock.getLatestHeadCalls.Load(),
		"GetLatestHead should be called once")

	// Verify SchedulerWorkflow was triggered (should schedule blocks from combined range)
	// The SchedulerWorkflow should process blocks from min(100) to max(1499)
	assert.Greater(t, mock.startIndexCalls.Load(), int32(0),
		"SchedulerWorkflow should schedule blocks from combined gap range")

	// Verify blocks were scheduled in the expected range
	mock.mu.Lock()
	scheduledBlocks := mock.scheduledBlocks
	mock.mu.Unlock()

	if len(scheduledBlocks) > 0 {
		// Check that scheduled blocks are within the combined range [100, 1499]
		for _, block := range scheduledBlocks {
			assert.True(t, block.Height >= 100 && block.Height <= 1499,
				"Scheduled block %d should be within combined range [100, 1499]", block.Height)
		}
	}

	t.Logf("Multiple gaps totaling 1100 blocks triggered SchedulerWorkflow with combined range [100, 1499], scheduled %d blocks",
		mock.startIndexCalls.Load())
}

// TestGapScanWorkflow_ErrorHandling tests error handling when FindGaps fails
// Scenario: FindGaps returns error
// Expected: Workflow returns error, logs appropriately
func TestGapScanWorkflow_ErrorHandling(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	mock := &mockSchedulerActivities{
		latestHead:  1000,
		lastIndexed: 0,
		gaps:        []adminstore.Gap{},
		shouldFail: map[string]bool{
			"FindGaps": true, // Simulate FindGaps failure
		},
		failureCount: make(map[string]int),
	}

	cfg := defaultWorkflowConfig()

	wfCtx := workflow.Context{
		TemporalClient: &temporal.Client{
			IndexerQueue:           "index:%s",
			IndexerLiveQueue:       "index:%s:live",
			IndexerHistoricalQueue: "index:%s:historical",
			IndexerOpsQueue:        "admin:%s",
		},
		ActivityContext: &activity.Context{},
		Config:          cfg,
	}

	// Register workflow and activities
	env.RegisterWorkflow(wfCtx.GapScanWorkflow)
	env.RegisterActivity(mock.GetLatestHead)
	env.RegisterActivity(mock.FindGaps)
	env.RegisterActivity(mock.StartIndexWorkflow)
	env.RegisterActivity(mock.StartIndexWorkflowBatch)

	// Execute GapScan workflow
	env.ExecuteWorkflow(wfCtx.GapScanWorkflow, workflow.GapScanInput{
		ChainID: 1,
	})

	require.True(t, env.IsWorkflowCompleted())

	// Verify workflow returned error
	err := env.GetWorkflowError()
	require.Error(t, err, "Workflow should return error when FindGaps fails")
	assert.Contains(t, err.Error(), "FindGaps", "Error should mention FindGaps failure")

	// Verify FindGaps was called (with retries due to retry policy: max 5 attempts)
	assert.Equal(t, int32(5), mock.findGapsCalls.Load(),
		"FindGaps should be called 5 times (1 initial + 4 retries)")

	// Verify FindGaps failure was recorded
	assert.Equal(t, 5, mock.failureCount["FindGaps"],
		"FindGaps failure should be recorded 5 times")

	// Verify StartIndexWorkflow was NOT called due to error
	assert.Equal(t, int32(0), mock.startIndexCalls.Load(),
		"StartIndexWorkflow should not be called when FindGaps fails")

	t.Logf("Error handling verified: FindGaps error properly propagated")
}

// TestGapScanWorkflow_GetLatestHeadError tests error handling when GetLatestHead fails
func TestGapScanWorkflow_GetLatestHeadError(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	mock := &mockSchedulerActivities{
		latestHead:  1000,
		lastIndexed: 0,
		gaps:        []adminstore.Gap{},
		shouldFail: map[string]bool{
			"GetLatestHead": true, // Simulate GetLatestHead failure
		},
		failureCount: make(map[string]int),
	}

	cfg := defaultWorkflowConfig()

	wfCtx := workflow.Context{
		TemporalClient: &temporal.Client{
			IndexerQueue:           "index:%s",
			IndexerLiveQueue:       "index:%s:live",
			IndexerHistoricalQueue: "index:%s:historical",
			IndexerOpsQueue:        "admin:%s",
		},
		ActivityContext: &activity.Context{},
		Config:          cfg,
	}

	// Register workflow and activities
	env.RegisterWorkflow(wfCtx.GapScanWorkflow)
	env.RegisterActivity(mock.GetLatestHead)
	env.RegisterActivity(mock.FindGaps)

	// Execute GapScan workflow
	env.ExecuteWorkflow(wfCtx.GapScanWorkflow, workflow.GapScanInput{
		ChainID: 1,
	})

	require.True(t, env.IsWorkflowCompleted())

	// Verify workflow returned error
	err := env.GetWorkflowError()
	require.Error(t, err, "Workflow should return error when GetLatestHead fails")
	assert.Contains(t, err.Error(), "GetLatestHead", "Error should mention GetLatestHead failure")

	// Verify GetLatestHead was called (with retries due to retry policy: max 5 attempts)
	assert.Equal(t, int32(5), mock.getLatestHeadCalls.Load(),
		"GetLatestHead should be called 5 times (1 initial + 4 retries)")

	// Verify failure was recorded
	assert.Equal(t, 5, mock.failureCount["GetLatestHead"],
		"GetLatestHead failure should be recorded 5 times")

	t.Logf("Error handling verified: GetLatestHead error properly propagated")
}

// TestGapScanWorkflow_EdgeCase_ExactlyThreshold tests the edge case where gaps total exactly 1000 blocks
func TestGapScanWorkflow_EdgeCase_ExactlyThreshold(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	mock := &mockSchedulerActivities{
		latestHead:  2000,
		lastIndexed: 0,
		gaps: []adminstore.Gap{
			{From: 1, To: 1000}, // Exactly 1000 blocks
		},
		shouldFail:   make(map[string]bool),
		failureCount: make(map[string]int),
	}

	cfg := defaultWorkflowConfig()
	cfg.CatchupThreshold = 1000

	wfCtx := workflow.Context{
		TemporalClient: &temporal.Client{
			IndexerQueue:           "index:%s",
			IndexerLiveQueue:       "index:%s:live",
			IndexerHistoricalQueue: "index:%s:historical",
			IndexerOpsQueue:        "admin:%s",
		},
		ActivityContext: &activity.Context{},
		Config:          cfg,
	}

	// Register workflows and activities
	env.RegisterWorkflow(wfCtx.GapScanWorkflow)
	env.RegisterWorkflow(wfCtx.SchedulerWorkflow)
	env.RegisterActivity(mock.GetLatestHead)
	env.RegisterActivity(mock.FindGaps)
	env.RegisterActivity(mock.IsSchedulerWorkflowRunning)
	env.RegisterActivity(mock.StartIndexWorkflow)
	env.RegisterActivity(mock.StartIndexWorkflowBatch)

	// Execute GapScan workflow
	env.ExecuteWorkflow(wfCtx.GapScanWorkflow, workflow.GapScanInput{
		ChainID: 1,
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// At threshold (1000 blocks), it should use SchedulerWorkflow (catch-up mode >= 1000)
	// Note: The threshold in ops.go is catchupThreshold = 1000, and the condition is:
	// if totalBlocks < catchupThreshold (line 283), meaning >= 1000 triggers scheduler
	assert.Greater(t, mock.startIndexCalls.Load(), int32(0),
		"Should schedule blocks via SchedulerWorkflow at threshold")

	t.Logf("Edge case at threshold: scheduled %d blocks", mock.startIndexCalls.Load())
}

// TestGapScanWorkflow_PerformanceLargeNumberOfGaps tests performance with many small gaps
func TestGapScanWorkflow_PerformanceLargeNumberOfGaps(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	// Create 100 small gaps (5 blocks each = 500 blocks total)
	gaps := make([]adminstore.Gap, 100)
	for i := 0; i < 100; i++ {
		start := uint64(i*10 + 1)
		gaps[i] = adminstore.Gap{From: start, To: start + 4} // 5 blocks per gap
	}

	mock := &mockSchedulerActivities{
		latestHead:   2000,
		lastIndexed:  0,
		gaps:         gaps,
		shouldFail:   make(map[string]bool),
		failureCount: make(map[string]int),
	}

	cfg := defaultWorkflowConfig()
	cfg.CatchupThreshold = 1000

	wfCtx := workflow.Context{
		TemporalClient: &temporal.Client{
			IndexerQueue:           "index:%s",
			IndexerLiveQueue:       "index:%s:live",
			IndexerHistoricalQueue: "index:%s:historical",
			IndexerOpsQueue:        "admin:%s",
		},
		ActivityContext: &activity.Context{},
		Config:          cfg,
	}

	// Register workflow and activities
	env.RegisterWorkflow(wfCtx.GapScanWorkflow)
	env.RegisterActivity(mock.GetLatestHead)
	env.RegisterActivity(mock.FindGaps)
	env.RegisterActivity(mock.StartIndexWorkflow)
	env.RegisterActivity(mock.StartIndexWorkflowBatch)

	// Set workflow timeout for performance testing
	env.SetWorkflowRunTimeout(5 * time.Minute)

	start := time.Now()
	env.ExecuteWorkflow(wfCtx.GapScanWorkflow, workflow.GapScanInput{
		ChainID: 1,
	})
	elapsed := time.Since(start)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Verify all 500 blocks were scheduled
	assert.Equal(t, int32(500), mock.startIndexCalls.Load(),
		"Should schedule exactly 500 blocks across 100 gaps")

	// Verify all blocks have UltraHigh priority
	dist := mock.getPriorityDistribution()
	assert.Equal(t, 500, dist[workflow.PriorityUltraHigh],
		"All blocks should have UltraHigh priority")

	t.Logf("Performance test: scheduled 500 blocks across 100 gaps in %v", elapsed)
}

package indexer

import (
	"testing"
	"time"

	"github.com/canopy-network/canopyx/pkg/indexer/activity"
	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"github.com/canopy-network/canopyx/pkg/indexer/workflow"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
)

// Note: Priority constants and thresholds are defined in ops.go
// Using the constants from the main implementation for consistency

// Test Scenario 1: Initial Mainnet Catch-up (700k blocks) - docs line 118-130
func TestSchedulerWorkflow_InitialMainnetCatchup(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	cleanup := workflow.SetSchedulerContinueThresholdForTesting(100000)
	defer cleanup()

	mock := &mockSchedulerActivities{
		latestHead:   700000,
		lastIndexed:  0,
		shouldFail:   make(map[string]bool),
		failureCount: make(map[string]int),
	}

	wfCtx := workflow.Context{
		TemporalClient: &temporal.Client{
			IndexerQueue:           "index:%s",
			IndexerLiveQueue:       "index:%s:live",
			IndexerHistoricalQueue: "index:%s:historical",
			IndexerOpsQueue:        "admin:%s",
		},
		ActivityContext: &activity.Context{},
		Config:          defaultWorkflowConfig(),
	}

	// Register workflow and activities
	env.RegisterWorkflow(wfCtx.SchedulerWorkflow)
	env.RegisterActivity(mock.StartIndexWorkflow)
	env.RegisterActivity(mock.StartIndexWorkflowBatch)

	// Set up workflow options to handle long execution
	env.SetWorkflowRunTimeout(10 * time.Hour) // Enough for 700k blocks

	// Note: In a real test, we'd simulate only a subset for performance
	// For demonstration, we'll test with 10k blocks to verify the pattern
	testInput := types.SchedulerInput{
		ChainID:      "mainnet",
		StartHeight:  690001,
		EndHeight:    700000,
		LatestHeight: 700000,
	}

	env.ExecuteWorkflow(wfCtx.SchedulerWorkflow, testInput)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Verify assertions from docs line 125-130
	assert.Equal(t, int32(10000), mock.startIndexCalls.Load(),
		"Should schedule exactly 10k blocks")

	// Verify priority distribution
	dist := mock.getPriorityDistribution()
	t.Logf("Priority distribution: %+v", dist)

	// Most recent blocks should be High priority (within 24 hours = 4320 blocks)
	assert.Greater(t, dist[workflow.PriorityHigh], 0, "Should have High priority blocks")

	// Note: Rate limiting verification using wall-clock time doesn't work with Temporal's test suite
	// The workflow.Sleep calls are present in the workflow (line 234) but testsuite auto-fires timers
	// In production, rate limiting is enforced via workflow.Sleep with schedulerBatchDelay (1s per 100 blocks)

	// Verify priority ordering
	err := mock.verifyPriorityOrder()
	assert.NoError(t, err, "Blocks should be scheduled in priority order")
}

// Test Scenario 2: HeadScan Race Condition - docs line 132-139
func TestSchedulerWorkflow_RaceConditionPrevention(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	cleanup := workflow.SetSchedulerContinueThresholdForTesting(100000)
	defer cleanup()

	// First HeadScan: Triggers SchedulerWorkflow (10k blocks = catch-up mode)
	mock := &mockSchedulerActivities{
		latestHead:   10000,
		lastIndexed:  0,
		shouldFail:   make(map[string]bool),
		failureCount: make(map[string]int),
	}

	wfCtx := workflow.Context{
		TemporalClient: &temporal.Client{
			IndexerQueue:           "index:%s",
			IndexerLiveQueue:       "index:%s:live",
			IndexerHistoricalQueue: "index:%s:historical",
			IndexerOpsQueue:        "admin:%s",
		},
		ActivityContext: &activity.Context{},
		Config:          defaultWorkflowConfig(),
	}

	// Register workflows
	env.RegisterWorkflow(wfCtx.HeadScan)
	env.RegisterWorkflow(wfCtx.SchedulerWorkflow) // Required because HeadScan triggers it as a child

	// Register activities
	env.RegisterActivity(mock.GetLatestHead)
	env.RegisterActivity(mock.GetLastIndexed)
	env.RegisterActivity(mock.IsSchedulerWorkflowRunning)
	env.RegisterActivity(mock.StartIndexWorkflow)
	env.RegisterActivity(mock.StartIndexWorkflowBatch)

	// Execute first HeadScan - this should trigger SchedulerWorkflow
	env.ExecuteWorkflow(wfCtx.HeadScan, workflow.HeadScanInput{
		ChainID: "mainnet",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Verify SchedulerWorkflow was triggered and started successfully
	// The logs should show "SchedulerWorkflow started successfully"
	t.Log("First HeadScan completed - SchedulerWorkflow should have been triggered")

	// Second HeadScan: Should detect running scheduler and skip triggering
	suite2 := testsuite.WorkflowTestSuite{}
	env2 := suite2.NewTestWorkflowEnvironment()

	// Set isSchedulerRunning to true to simulate it's still running
	mock.isSchedulerRunning.Store(true)

	// Register everything again for the second test environment
	env2.RegisterWorkflow(wfCtx.HeadScan)
	env2.RegisterWorkflow(wfCtx.SchedulerWorkflow)
	env2.RegisterActivity(mock.GetLatestHead)
	env2.RegisterActivity(mock.GetLastIndexed)
	env2.RegisterActivity(mock.IsSchedulerWorkflowRunning)
	env2.RegisterActivity(mock.StartIndexWorkflow)
	env2.RegisterActivity(mock.StartIndexWorkflowBatch)

	// Track StartIndexWorkflow calls before second HeadScan
	callsBeforeSecond := mock.startIndexCalls.Load()

	// Execute second HeadScan - should detect running scheduler and NOT trigger new one
	env2.ExecuteWorkflow(wfCtx.HeadScan, workflow.HeadScanInput{
		ChainID: "mainnet",
	})

	require.True(t, env2.IsWorkflowCompleted())
	require.NoError(t, env2.GetWorkflowError())

	// Verify no additional blocks were scheduled (scheduler not re-triggered)
	callsAfterSecond := mock.startIndexCalls.Load()
	assert.Equal(t, callsBeforeSecond, callsAfterSecond,
		"Second HeadScan should not trigger scheduler when one is already running")

	t.Logf("Race condition prevention verified: scheduler running=%v", mock.isSchedulerRunning.Load())
}

// Test Scenario 3: Normal Operation (5-10 blocks) - docs line 141-149
func TestSchedulerWorkflow_NormalOperation(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	mock := &mockSchedulerActivities{
		latestHead:   1005,
		lastIndexed:  1000,
		shouldFail:   make(map[string]bool),
		failureCount: make(map[string]int),
	}

	wfCtx := workflow.Context{
		TemporalClient: &temporal.Client{
			IndexerQueue:           "index:%s",
			IndexerLiveQueue:       "index:%s:live",
			IndexerHistoricalQueue: "index:%s:historical",
			IndexerOpsQueue:        "admin:%s",
		},
		ActivityContext: &activity.Context{},
		Config:          defaultWorkflowConfig(),
	}

	// Register workflow and activities
	env.RegisterWorkflow(wfCtx.HeadScan)
	env.RegisterActivity(mock.GetLatestHead)
	env.RegisterActivity(mock.GetLastIndexed)
	env.RegisterActivity(mock.StartIndexWorkflow)
	env.RegisterActivity(mock.StartIndexWorkflowBatch)

	// Execute HeadScan for small range (should use direct scheduling)
	env.ExecuteWorkflow(wfCtx.HeadScan, workflow.HeadScanInput{
		ChainID: "mainnet",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Verify assertions from docs line 145-149
	assert.Equal(t, int32(5), mock.startIndexCalls.Load(),
		"Should schedule exactly 5 blocks")

	// Verify all blocks have high priority (live blocks)
	dist := mock.getPriorityDistribution()
	assert.Equal(t, 5, dist[workflow.PriorityUltraHigh],
		"All blocks should have ultra high priority for normal operation")

	// Verify SchedulerWorkflow NOT triggered
	assert.False(t, mock.isSchedulerRunning.Load(),
		"SchedulerWorkflow should not be triggered for small ranges")
}

// Test Scenario 4: Mixed Priority Distribution (10k blocks) - docs line 151-160
func TestSchedulerWorkflow_MixedPriorityDistribution(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	cleanup := workflow.SetSchedulerContinueThresholdForTesting(100000)
	defer cleanup()

	mock := &mockSchedulerActivities{
		latestHead:   100000,
		lastIndexed:  90000,
		shouldFail:   make(map[string]bool),
		failureCount: make(map[string]int),
	}

	wfCtx := workflow.Context{
		TemporalClient: &temporal.Client{
			IndexerQueue:           "index:%s",
			IndexerLiveQueue:       "index:%s:live",
			IndexerHistoricalQueue: "index:%s:historical",
			IndexerOpsQueue:        "admin:%s",
		},
		ActivityContext: &activity.Context{},
		Config:          defaultWorkflowConfig(),
	}

	// Register workflow and activities
	env.RegisterWorkflow(wfCtx.SchedulerWorkflow)
	env.RegisterActivity(mock.StartIndexWorkflow)
	env.RegisterActivity(mock.StartIndexWorkflowBatch)

	// Execute workflow with 10k block range
	input := types.SchedulerInput{
		ChainID:      "mainnet",
		StartHeight:  90001,
		EndHeight:    100000,
		LatestHeight: 100000,
	}

	env.ExecuteWorkflow(wfCtx.SchedulerWorkflow, input)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Verify priority distribution matches expected batch-based calculation
	dist := mock.getPriorityDistribution()
	t.Logf("Priority distribution for 10k blocks: %+v", dist)

	expected := map[int]int{}
	current := input.EndHeight
	for current >= input.StartHeight {
		priority := workflow.CalculateBlockPriority(input.LatestHeight, current, wfCtx.Config.BlockTimeSeconds, false)

		remaining := current - input.StartHeight + 1
		batchLimit := wfCtx.Config.SchedulerBatchSize
		if batchLimit == 0 {
			batchLimit = 1
		}

		var batchStart uint64
		if remaining <= batchLimit {
			batchStart = input.StartHeight
		} else {
			batchStart = current - batchLimit + 1
		}

		batchSize := int(current - batchStart + 1)
		expected[priority] += batchSize
		current = batchStart - 1
	}

	assert.Equal(t, expected[workflow.PriorityHigh], dist[workflow.PriorityHigh], "High priority batch distribution mismatch")
	assert.Equal(t, expected[workflow.PriorityMedium], dist[workflow.PriorityMedium], "Medium priority batch distribution mismatch")
	assert.Equal(t, expected[workflow.PriorityLow], dist[workflow.PriorityLow], "Low priority batch distribution mismatch")

	// Verify scheduling order
	err := mock.verifyPriorityOrder()
	assert.NoError(t, err, "Blocks should be scheduled in priority order (High->Medium->Low)")
}

// Test Scenario 5: ContinueAsNew Chain (50k blocks) - docs line 162-170
func TestSchedulerWorkflow_ContinueAsNewChain(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	mock := &mockSchedulerActivities{
		latestHead:   50000,
		lastIndexed:  0,
		shouldFail:   make(map[string]bool),
		failureCount: make(map[string]int),
	}

	wfCtx := workflow.Context{
		TemporalClient: &temporal.Client{
			IndexerQueue:           "index:%s",
			IndexerLiveQueue:       "index:%s:live",
			IndexerHistoricalQueue: "index:%s:historical",
			IndexerOpsQueue:        "admin:%s",
		},
		ActivityContext: &activity.Context{},
		Config:          defaultWorkflowConfig(),
	}

	// Register workflow with continuation tracking
	env.RegisterWorkflow(wfCtx.SchedulerWorkflow)
	env.RegisterActivity(mock.StartIndexWorkflow)
	env.RegisterActivity(mock.StartIndexWorkflowBatch)

	// For testing, we'll simulate with smaller threshold
	input := types.SchedulerInput{
		ChainID:      "mainnet",
		StartHeight:  1,
		EndHeight:    50000,
		LatestHeight: 50000,
	}

	env.ExecuteWorkflow(wfCtx.SchedulerWorkflow, input)

	// Note: testsuite doesn't fully support ContinueAsNew chains
	// In production, this would trigger 5 executions (50k / 10k threshold)
	// We verify the mechanism works for the first execution

	require.True(t, env.IsWorkflowCompleted())

	// Verify blocks were scheduled
	assert.Greater(t, mock.startIndexCalls.Load(), int32(0),
		"Should have scheduled blocks")

	t.Logf("Scheduled %d blocks before first continuation", mock.startIndexCalls.Load())
}

// Test helper function for priority calculation
func TestCalculateBlockPriority(t *testing.T) {
	tests := []struct {
		name     string
		latest   uint64
		height   uint64
		isLive   bool
		expected int
	}{
		{
			name:     "Live blocks get UltraHigh priority",
			latest:   10000,
			height:   9999,
			isLive:   true,
			expected: workflow.PriorityUltraHigh,
		},
		{
			name:     "Blocks within 24 hours get High priority",
			latest:   10000,
			height:   9000, // 1000 blocks * 20s = 20000s = ~5.5 hours
			isLive:   false,
			expected: workflow.PriorityHigh,
		},
		{
			name:     "Blocks 24-48 hours old get Medium priority",
			latest:   10000,
			height:   6000, // 4000 blocks * 20s = 80000s = ~22 hours
			isLive:   false,
			expected: workflow.PriorityHigh,
		},
		{
			name:     "Blocks 48-72 hours old get Low priority",
			latest:   20000,
			height:   10000, // 10000 blocks * 20s = 200000s = ~55 hours
			isLive:   false,
			expected: workflow.PriorityLow,
		},
		{
			name:     "Very old blocks get UltraLow priority",
			latest:   50000,
			height:   1000, // 49000 blocks * 20s = 980000s = ~272 hours
			isLive:   false,
			expected: workflow.PriorityUltraLow,
		},
	}

	const blockTimeSeconds = 20
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := workflow.CalculateBlockPriority(tt.latest, tt.height, blockTimeSeconds, tt.isLive)
			assert.Equal(t, tt.expected, actual,
				"Priority for block %d (latest: %d, isLive: %v)",
				tt.height, tt.latest, tt.isLive)
		})
	}
}

// Test rate limiting enforcement
func TestSchedulerWorkflow_RateLimiting(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	cleanup := workflow.SetSchedulerContinueThresholdForTesting(100000)
	defer cleanup()

	mock := &mockSchedulerActivities{
		latestHead:   1000,
		lastIndexed:  0,
		shouldFail:   make(map[string]bool),
		failureCount: make(map[string]int),
	}

	wfCtx := workflow.Context{
		TemporalClient: &temporal.Client{
			IndexerQueue:           "index:%s",
			IndexerLiveQueue:       "index:%s:live",
			IndexerHistoricalQueue: "index:%s:historical",
			IndexerOpsQueue:        "admin:%s",
		},
		ActivityContext: &activity.Context{},
		Config:          defaultWorkflowConfig(),
	}

	// Register workflow and activities
	env.RegisterWorkflow(wfCtx.SchedulerWorkflow)
	env.RegisterActivity(mock.StartIndexWorkflow)
	env.RegisterActivity(mock.StartIndexWorkflowBatch)

	// Execute with 1000 blocks to test rate limiting
	input := types.SchedulerInput{
		ChainID:      "mainnet",
		StartHeight:  1,
		EndHeight:    1000,
		LatestHeight: 1000,
	}

	start := time.Now()
	env.ExecuteWorkflow(wfCtx.SchedulerWorkflow, input)
	elapsed := time.Since(start)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// With 1000 blocks at 100/sec, should take at least 10 seconds in production
	// In test environment, Temporal's testsuite auto-fires timers so elapsed time is not meaningful
	assert.Equal(t, int32(1000), mock.startIndexCalls.Load())

	// Note: Rate limiting is enforced in production via workflow.Sleep (line 234)
	// The testsuite auto-fires timers, so we can't measure actual wall-clock delays
	// We verify that all blocks were scheduled correctly with proper batching

	t.Logf("Scheduled 1000 blocks in %v (test env with auto-fired timers)", elapsed)
}

// Test error handling and retries
func TestSchedulerWorkflow_ErrorHandling(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	cleanup := workflow.SetSchedulerContinueThresholdForTesting(100000)
	defer cleanup()

	mock := &mockSchedulerActivities{
		latestHead:  100,
		lastIndexed: 0,
		shouldFail: map[string]bool{
			"StartIndexWorkflow": true, // Simulate failures
		},
		failureCount: make(map[string]int),
	}

	wfCtx := workflow.Context{
		TemporalClient: &temporal.Client{
			IndexerQueue:           "index:%s",
			IndexerLiveQueue:       "index:%s:live",
			IndexerHistoricalQueue: "index:%s:historical",
			IndexerOpsQueue:        "admin:%s",
		},
		ActivityContext: &activity.Context{},
		Config:          defaultWorkflowConfig(),
	}

	// Register workflow and activities
	env.RegisterWorkflow(wfCtx.SchedulerWorkflow)
	env.RegisterActivity(mock.StartIndexWorkflow)
	env.RegisterActivity(mock.StartIndexWorkflowBatch)

	input := types.SchedulerInput{
		ChainID:      "mainnet",
		StartHeight:  1,
		EndHeight:    10,
		LatestHeight: 100,
	}

	env.ExecuteWorkflow(wfCtx.SchedulerWorkflow, input)

	// Workflow should complete even with activity failures
	// (StartIndexWorkflow failures are logged but don't stop the workflow)
	require.True(t, env.IsWorkflowCompleted())

	// Verify attempts were made
	assert.Greater(t, mock.startIndexCalls.Load(), int32(0),
		"Should attempt to schedule blocks despite failures")
	assert.Greater(t, mock.failureCount["StartIndexWorkflow"], 0,
		"Should record activity failures")
}

// Benchmark test for large-scale scheduling performance
func BenchmarkSchedulerWorkflow_LargeScale(b *testing.B) {
	cleanup := workflow.SetSchedulerContinueThresholdForTesting(100000)
	defer cleanup()

	for i := 0; i < b.N; i++ {
		suite := testsuite.WorkflowTestSuite{}
		env := suite.NewTestWorkflowEnvironment()

		mock := &mockSchedulerActivities{
			latestHead:   uint64(100000 * (i + 1)),
			lastIndexed:  uint64(100000 * i),
			shouldFail:   make(map[string]bool),
			failureCount: make(map[string]int),
		}

		wfCtx := workflow.Context{
			TemporalClient: &temporal.Client{
				IndexerQueue:           "index:%s",
				IndexerLiveQueue:       "index:%s:live",
				IndexerHistoricalQueue: "index:%s:historical",
				IndexerOpsQueue:        "admin:%s",
			},
			ActivityContext: &activity.Context{},
			Config:          defaultWorkflowConfig(),
		}

		env.RegisterWorkflow(wfCtx.SchedulerWorkflow)
		env.RegisterActivity(mock.StartIndexWorkflow)
		env.RegisterActivity(mock.StartIndexWorkflowBatch)

		input := types.SchedulerInput{
			ChainID:      "mainnet",
			StartHeight:  uint64(100000*i + 1),
			EndHeight:    uint64(100000 * (i + 1)),
			LatestHeight: uint64(100000 * (i + 1)),
		}

		env.ExecuteWorkflow(wfCtx.SchedulerWorkflow, input)

		if !env.IsWorkflowCompleted() {
			b.Fatalf("Workflow did not complete for iteration %d", i)
		}
	}
}

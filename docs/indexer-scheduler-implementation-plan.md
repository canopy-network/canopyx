# Indexer Priority-Based Scheduler Implementation Plan

## Status: ✅ IMPLEMENTED
Last Updated: 2025-10-14

---

## Executive Summary

Implement a priority-based block scheduling system for the indexer that efficiently handles both normal operations (5-10 missing blocks) and massive catch-up scenarios (700k+ missing blocks) without overwhelming Temporal's gRPC API.

### Key Design Decisions

1. **Keep IndexBlockWorkflow isolated** - Each block indexes in its own workflow
2. **Adaptive scheduling** - Normal mode for small ranges, SchedulerWorkflow for large backlogs
3. **Priority-based ordering** - 5 priority levels based on block age (20s per block)
4. **Local activities** - All scheduling operations use local activities for performance
5. **Race condition prevention** - Use Temporal workflow queries to detect running scheduler

---

## Architecture Overview

```
HeadScan (every 10s on admin:chainID)
    ↓
    Query: IsSchedulerWorkflowRunning? (Regular Activity)
    ↓
    ├─ If NO + ≥1000 blocks → Trigger SchedulerWorkflow
    │                         └─ Priority-ordered batch scheduling
    │                            (Local Activities, 100 wf/sec rate limit)
    │
    └─ If <1000 blocks → Direct schedule (Local Activities, Ultra High priority)
```

---

## Priority System Design

### Block Age Calculation
```
blockTime = 20 seconds
blockAge = (latest - height) × 20 seconds
hoursAgo = blockAge / 3600
```

### Priority Levels
1. **Ultra High (5)** - Live blocks from HeadScan (normal operation)
2. **High (4)** - Last 24 hours (4,320 blocks)
3. **Medium (3)** - 24-48 hours (4,320 blocks)
4. **Low (2)** - 48-72 hours (4,320 blocks)
5. **Ultra Low (1)** - Older than 72 hours (remaining blocks)

### Scheduling Order
SchedulerWorkflow processes: High → Medium → Low → Ultra Low

---

## Implementation Phases

### Phase 1: Core Infrastructure ✅ COMPLETED
**Files to modify:**
- `pkg/indexer/workflow/ops.go` - Add priority calculation, SchedulerWorkflow
- `pkg/indexer/activity/ops.go` - Add IsSchedulerWorkflowRunning activity
- `pkg/indexer/types/types.go` - Add SchedulerInput type
- `pkg/temporal/client.go` - Add GetSchedulerWorkflowID helper
- `app/indexer/app.go` - Register SchedulerWorkflow and activities

**Key Components:**
1. `calculateBlockPriority(latest, height, isLive)` - Time-based priority calculation
2. `SchedulerWorkflow(ctx, SchedulerInput)` - Long-running batch scheduler
3. `IsSchedulerWorkflowRunning(ctx, chainID)` - Race condition detection
4. `scheduleHeightBatch()` - Adaptive mode detection (normal vs catch-up)
5. `scheduleDirectly()` - Fast path for small ranges

### Phase 2: Local Activity Migration ✅ COMPLETED
**Files to modify:**
- `pkg/indexer/workflow/ops.go` - Convert activities to local execution

**Activities to convert:**
- `StartIndexWorkflow` → Local (critical path)
- `GetLastIndexed` → Local (fast DB query)
- `GetLatestHead` → Local (RPC has 15s timeout)
- `FindGaps` → Local (fast DB query)
- `IsSchedulerWorkflowRunning` → Regular (Temporal API call)

**Local Activity Configuration:**
```go
localAo := workflow.LocalActivityOptions{
    ScheduleToCloseTimeout: 20 * time.Second,  // Covers 15s RPC + buffer
    RetryPolicy: &sdktemporal.RetryPolicy{
        InitialInterval:    500 * time.Millisecond,
        BackoffCoefficient: 2.0,
        MaximumInterval:    5 * time.Second,
        MaximumAttempts:    5,
    },
}
```

### Phase 3: Testing Infrastructure ✅ COMPLETED

#### 3.1 Unit Tests
**File:** `pkg/indexer/workflow/ops_test.go`

**Test Cases:**
- `TestCalculateBlockPriority` - Verify priority calculation for all time ranges
- `TestScheduleHeightBatch_SmallRange` - Normal mode (<1000 blocks)
- `TestScheduleHeightBatch_LargeRange` - Catch-up mode (≥1000 blocks)
- `TestSchedulerWorkflow_PriorityOrdering` - Verify High→Medium→Low→UltraLow order
- `TestSchedulerWorkflow_RateLimiting` - Verify 100 wf/sec limit
- `TestSchedulerWorkflow_ContinueAsNew` - Verify continuation at 10k blocks

#### 3.2 High-Load Simulation Tests
**File:** `pkg/indexer/workflow/scheduler_load_test.go`

**Simulation Scenarios:**

1. **Scenario: Initial Mainnet Catch-up (700k blocks)**
   - Mock: `GetLatestHead=700000`, `GetLastIndexed=0`
   - Expected behavior:
     - HeadScan triggers SchedulerWorkflow
     - 700k blocks grouped by priority
     - Scheduled in priority order at 100 wf/sec
     - 70 ContinueAsNew executions (every 10k blocks)
   - Assertions:
     - `StartIndexWorkflow` called 700k times
     - Priority distribution matches expected (calculate based on current time)
     - Rate limiting respected (max 100 calls/sec)
     - No duplicate workflow triggers

2. **Scenario: HeadScan Race Condition (10s intervals)**
   - Mock: 2 HeadScan executions 10s apart during active SchedulerWorkflow
   - Expected behavior:
     - First HeadScan triggers SchedulerWorkflow
     - Second HeadScan detects running scheduler, skips trigger
   - Assertions:
     - Only 1 SchedulerWorkflow started
     - `IsSchedulerWorkflowRunning` returns true on second call

3. **Scenario: Normal Operation (5-10 blocks)**
   - Mock: `GetLatestHead=1005`, `GetLastIndexed=1000`
   - Expected behavior:
     - HeadScan uses direct scheduling (no SchedulerWorkflow)
     - All blocks scheduled as Ultra High priority
   - Assertions:
     - `StartIndexWorkflow` called 5 times
     - All calls have `PriorityKey=5`
     - SchedulerWorkflow NOT triggered

4. **Scenario: Mixed Priority Distribution (10k blocks)**
   - Mock: `GetLatestHead=100000`, `GetLastIndexed=90000`
   - Current time = simulated to produce expected distribution
   - Expected behavior:
     - SchedulerWorkflow triggered
     - Blocks grouped into High/Medium/Low/UltraLow buckets
     - Scheduled in priority order
   - Assertions:
     - Verify priority bucket sizes
     - Verify scheduling order (High first, UltraLow last)

5. **Scenario: ContinueAsNew Chain (50k blocks)**
   - Mock: `GetLatestHead=50000`, `GetLastIndexed=0`
   - Expected behavior:
     - Initial SchedulerWorkflow processes 10k blocks
     - ContinueAsNew triggered 4 times (5 total executions)
   - Assertions:
     - 5 SchedulerWorkflow executions
     - Each processes ~10k blocks
     - State properly carried across continuations

**Test Infrastructure Requirements:**
- Mock Temporal test environment
- Mock ClickHouse queries (GetLastIndexed, FindGaps)
- Mock RPC client (GetLatestHead)
- Call counter/tracker for StartIndexWorkflow
- Time control for priority calculations
- Rate limiter verification

#### 3.3 Integration Tests
**File:** `pkg/indexer/workflow/scheduler_integration_test.go`

**Test with real Temporal test server:**
- End-to-end HeadScan → SchedulerWorkflow → IndexBlockWorkflow flow
- Verify workflow history sizes stay under limits
- Verify ContinueAsNew chain execution
- Verify priority ordering in Temporal task queues

---

## Configuration Parameters

### Constants (in `pkg/indexer/workflow/ops.go`)

```go
const (
    // Priority levels
    PriorityUltraHigh = 5
    PriorityHigh      = 4
    PriorityMedium    = 3
    PriorityLow       = 2
    PriorityUltraLow  = 1

    // Thresholds
    catchupThreshold       = 1000  // Switch to SchedulerWorkflow at this many blocks
    directScheduleBatchSize = 50   // Batch size for direct scheduling

    // SchedulerWorkflow settings
    schedulerBatchSize     = 100   // Schedule 100 workflows at a time
    schedulerBatchDelay    = 1 * time.Second  // 1s between batches = 100 wf/sec
    schedulerContinueThreshold = 10000  // ContinueAsNew after 10k blocks

    // Block timing
    blockTime = 20  // seconds per block
)
```

### Tunable Parameters (future config file)
- Rate limit (workflows/sec)
- Batch sizes
- ContinueAsNew threshold
- Priority time windows

---

## Performance Expectations

### Current Implementation (Regular Activities)
- 700k blocks × 20ms per schedule = 14,000 seconds ≈ **4 hours**
- Task queue overhead dominates

### New Implementation (Local Activities + Rate Limiting)
- 700k blocks ÷ 100 wf/sec = 7,000 seconds ≈ **2 hours**
- Rate limiting dominates (intentional throttling)
- Can adjust rate limit if Temporal handles more load

### Normal Operation
- 5-10 blocks scheduled in < 1 second
- No SchedulerWorkflow overhead

---

## Rollout Strategy

### Stage 1: Implementation & Unit Tests
- Implement all components
- Write comprehensive unit tests
- Test engineer validates test coverage

### Stage 2: High-Load Simulation
- Run simulation tests with mocked 700k block scenario
- Verify rate limiting behavior
- Verify priority ordering
- Verify no race conditions

### Stage 3: Integration Testing
- Deploy to test environment
- Test with real Temporal server
- Monitor workflow history sizes
- Verify ContinueAsNew chains

### Stage 4: Gradual Production Rollout
- Deploy to single test chain
- Monitor metrics (scheduling rate, priority distribution)
- Scale to all chains if successful

---

## Metrics & Observability

### Metrics to Add
1. **Scheduler metrics:**
   - `scheduler_blocks_scheduled_total{priority, chain_id}`
   - `scheduler_batch_duration_seconds{chain_id}`
   - `scheduler_active{chain_id}` (gauge: 0 or 1)

2. **HeadScan metrics:**
   - `headscan_mode{mode=normal|catchup, chain_id}`
   - `headscan_blocks_pending{chain_id}`

3. **Priority distribution:**
   - `blocks_by_priority_bucket{priority, chain_id}`

### Logging
- Log scheduler start/stop with range and priority distribution
- Log ContinueAsNew events with progress
- Log when scheduler is skipped due to already running

---

## Risk Mitigation

### Risk 1: Scheduler gets stuck
**Mitigation:**
- Add workflow timeout (4 hours max for 700k blocks)
- Add monitoring alerts for long-running schedulers
- ClearSchedulerActive cleanup on timeout

### Risk 2: Priority calculation incorrect
**Mitigation:**
- Comprehensive unit tests for edge cases
- Logging of priority distribution on each run
- Dashboard to visualize priority assignments

### Risk 3: Race condition on scheduler trigger
**Mitigation:**
- Temporal workflow ID deduplication
- IsSchedulerWorkflowRunning check
- Integration tests for concurrent HeadScans

### Risk 4: ContinueAsNew chain breaks
**Mitigation:**
- State validation on resume
- Integration tests for long chains
- Monitoring for incomplete scheduler runs

---

## Success Criteria

1. ✅ 700k block catch-up completes in < 3 hours
2. ✅ Normal operation (5-10 blocks) completes in < 1 second
3. ✅ No duplicate SchedulerWorkflow triggers during concurrent HeadScans
4. ✅ Priority ordering verified (High blocks scheduled before Low blocks)
5. ✅ Workflow history sizes stay under 30k events per execution
6. ✅ Rate limiting respected (≤ 100 workflows/sec)
7. ✅ All unit tests pass
8. ✅ All high-load simulation tests pass
9. ✅ Integration tests pass with real Temporal server
10. ✅ No goroutine explosions or memory leaks

---

## Next Steps

1. [ ] Review and approve this plan
2. [ ] Use go-senior-engineer agent to implement Phase 1
3. [ ] Use go-senior-engineer agent to implement Phase 2
4. [ ] Use go-senior-engineer agent to implement Phase 3 (tests)
5. [ ] Run tests and validate behavior
6. [ ] Iterate based on test results
7. [ ] Deploy to test environment
8. [ ] Production rollout

---

## Agent Usage Plan

### Implementation (go-senior-engineer)
**Prompt:** "Implement priority-based SchedulerWorkflow for indexer based on docs/indexer-scheduler-implementation-plan.md Phase 1. Follow AGENTS.md guidelines: cite line numbers, use summaries not full output, batch related changes."

### Testing (go-senior-engineer)
**Prompt:** "Create comprehensive high-load simulation tests for SchedulerWorkflow based on docs/indexer-scheduler-implementation-plan.md Phase 3.2. Mock 700k block scenario, verify priority ordering, rate limiting, and race condition handling."

---

## Notes

- User wants test engineer approach: simulate high loads without actual 700k blocks
- All activities except IsSchedulerWorkflowRunning should be local activities
- RPC client has 15s timeout, safe for local activities
- SchedulerWorkflow uses ContinueAsNew to avoid history limits
- HeadScan runs every 10s, must handle race conditions
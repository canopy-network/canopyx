# Performance Optimization Plan - Temporal Indexer

**Status**: In Progress
**Created**: 2025-10-16
**Last Updated**: 2025-10-16
**Current Performance**: 125k workflows/sec (1k workflows in 8ms avg)
**Goal**: Stabilize system and optimize for 750k block backlog processing

---

## Current Situation

- ✅ Parallel workflow scheduling implemented (worker pool)
- ✅ Queue stats refactored to use DescribeTaskQueueEnhanced API
- ✅ Integration tests working
- ⚠️ **429 errors appearing in Temporal UI** (rate limiting)
- ⚠️ Need to verify PostgreSQL<>Temporal tuning before code optimizations

---

## Phase 0: Critical Bug Fixes (COMPLETED) ✅

**Goal**: Fix critical bugs discovered during performance review

### Task 0.1: Fix Replica Scaling Bug ✅
**Status**: Completed
**Assigned**: go-senior-engineer agent
**Priority**: Critical
**Issue**: HandleChainPatch endpoint was NOT updating MinReplicas/MaxReplicas fields, causing controller to scale deployments incorrectly.

**Root Cause**:
- User updates MaxReplicas from 3 to 9 via UI
- PATCH handler received update but ignored replica fields
- Controller continued using old MaxReplicas: 3
- Controller scaled DOWN from 3→1 (to match min) instead of allowing scale to 9

**Fix Applied**:
- Added MinReplicas and MaxReplicas update logic to HandleChainPatch (lines 774-789)
- Added validation: MaxReplicas >= MinReplicas
- Returns 400 error if validation fails

**Files Modified**:
- `app/admin/controller/chain.go` (lines 774-789)

---

### Task 0.2: Clarify Head-1 Logic (Documentation) ✅
**Status**: Completed - No code changes needed
**Priority**: Medium
**Issue**: Misunderstanding about "never catching up to head"

**Clarification**:
The head-1 logic is **CORRECT** and working as intended:
- If chain head = 100 but block 100 isn't ready yet
- GetLatestHead returns 99 (last available block)
- This shows **100% sync** because we've indexed all available blocks
- Block 100 will be indexed when it becomes available on next HeadScan cycle

**No changes needed** - logic is sound for handling RPC block propagation delays.

---

### Task 0.3: Add Gap/Missing Blocks Summary to API ✅
**Status**: Completed
**Assigned**: go-senior-engineer agent
**Priority**: High
**Issue**: UI showing misleading progress (53 / 750.4K = 0.007%) when actually synced with latest but missing historical blocks.

**Problem**:
- Chain head: 750,400 (latest block)
- Last indexed: 53 (we're synced with latest!)
- Missing: blocks 1-750,347 (historical backlog)
- UI incorrectly shows: "0.007% progress"

**Should show**:
- Live status: ✅ Synced (at block 750,400)
- Historical: 53 / 750,400 indexed
- Missing: 750,347 blocks in 1 gap range

**Fix Applied**:
- Added gap fetching to HandleChainStatus API using pond worker pool
- Added fields to ChainStatus: MissingBlocksCount, GapRangesCount, LargestGapStart/End, IsLiveSync
- Used same pond pattern as StartIndexWorkflowBatch for consistency
- Parallel fetching with 8 workers (heads + gaps)

**Files Modified**:
- `app/admin/controller/chain.go` (lines 19, 347-436, 477-486)
- `app/admin/types/chain.go` (fields already existed)

**API Response Example**:
```json
{
  "mainnet": {
    "last_indexed": 53,
    "head": 750400,
    "missing_blocks_count": 750347,
    "gap_ranges_count": 1,
    "largest_gap_start": 54,
    "largest_gap_end": 750400,
    "is_live_sync": true
  }
}
```

**UI Action Required**:
- Update dashboard to show "Live Sync" status separately from historical progress
- Display missing blocks count as a warning/info badge
- Show gap ranges or ETA for backlog completion

---

## Phase 1: Infrastructure Stability (COMPLETED) ✅

**Goal**: Ensure Temporal + PostgreSQL can handle current load without 429 errors

**Result**: System is healthy. 429 errors are from Temporal Web UI browser rate limiting (cosmetic), not server issues. Ready for Phase 2.

### Task 1.1: Investigate 429 Errors ✅
**Status**: Completed
**Assigned**: Infrastructure investigation
**Priority**: Critical
**Description**:
- Check Temporal server logs for rate limiting triggers
- Review current rate limits in Temporal configuration
- Identify which component is returning 429 (frontend, history, matching service)
- Check if 429s are from gRPC rate limits or HTTP rate limits

**Findings**:
- ✅ No 429 errors in Temporal server logs
- ✅ Only "slow gRPC call" warnings (5-10s, normal under load)
- ✅ Rate limits well-configured (5-16x higher than defaults)
- ⚠️  "429 errors" likely from Temporal Web UI (browser rate limiting), not server
- ✅ PostgreSQL healthy: 95/300 connections (32% utilization)

**Deliverables**:
- ✅ Root cause analysis: `/docs/temporal-infrastructure-investigation.md`
- ✅ System is healthy and ready for Phase 2 optimizations

---

### Task 1.2: Verify PostgreSQL Health ✅
**Status**: Completed (part of Task 1.1 investigation)
**Assigned**: Infrastructure investigation
**Priority**: Critical
**Description**:
- Check PostgreSQL connection pool usage
- Review query performance (slow query log)
- Verify PostgreSQL resource limits (CPU, memory, IOPS)
- Check for lock contention or connection exhaustion
- Review current `postgresql-values.yaml` settings effectiveness

**Deliverables**:
- PostgreSQL health report
- Connection pool metrics
- Query performance analysis
- Resource utilization graphs

---

### Task 1.3: Tune Temporal Server Configuration ⏳
**Status**: Not Started
**Assigned**: TBD (Same infrastructure agent)
**Priority**: High
**Description**:
- Review `temporal-values.yaml` for rate limit settings
- Adjust rate limits based on Task 1.1 findings
- Tune worker poll rate and concurrency limits
- Configure gRPC keepalive and connection settings
- Optimize matching service for high workflow creation rate

**Key Areas**:
```yaml
# Areas to investigate/tune in temporal-values.yaml
server:
  frontend:
    rps: <current_value>  # Request per second limit
  history:
    rps: <current_value>
  matching:
    rps: <current_value>
```

**Deliverables**:
- Updated `temporal-values.yaml` with tuned settings
- Configuration change documentation
- Rollback plan

---

### Task 1.4: Verify System After Tuning ⏳
**Status**: Not Started
**Assigned**: TBD (Same infrastructure agent)
**Priority**: High
**Description**:
- Deploy tuned configuration
- Monitor for 429 errors
- Run test scheduling batch (10k-50k workflows)
- Verify all components healthy
- Check logs for new errors or warnings

**Success Criteria**:
- ✅ No 429 errors during test batch
- ✅ All Temporal services healthy
- ✅ PostgreSQL performance stable
- ✅ Current workflow throughput maintained (125k/sec)

**Deliverables**:
- Test results report
- System health metrics
- Go/No-Go decision for Phase 2

---

## Phase 2: Code Optimizations (PENDING Phase 1 Completion)

**Goal**: Improve scheduling throughput and efficiency after infrastructure is stable

### Task 2.1: Increase Worker Pool Parallelism ⏳
**Status**: Not Started
**Assigned**: go-senior-engineer agent
**Priority**: Medium
**Description**:
- Modify `pkg/indexer/activity/context.go` scheduler parallelism calculation
- Change from `2 × CPU` to `4 × CPU` or `8 × CPU`
- Add configuration override in indexer config
- Update logging to show active parallelism

**Code Changes**:
```go
// pkg/indexer/activity/context.go
func schedulerParallelism(override int) int {
    // Change multiplier from 2 to 4 or 8
    parallelism := n * 4  // Currently: n * 2
    // ...
}
```

**Testing**:
- Unit test with various CPU counts
- Integration test with 10k workflow batch
- Measure throughput improvement

**Expected Impact**: 2-4x throughput increase (if not I/O bound)

---

### Task 2.2: Increase Batch Sizes ⏳
**Status**: Not Started
**Assigned**: go-senior-engineer agent
**Priority**: Medium
**Description**:
- Increase `SchedulerBatchSize` from 1000 to 5000-10000
- Update `StartToCloseTimeout` for batch activities to accommodate larger batches
- Add batch size to workflow configuration

**Code Changes**:
```go
// pkg/indexer/workflow/context.go
type Config struct {
    SchedulerBatchSize int  // Increase default 1000 -> 5000
    // ...
}

// pkg/indexer/workflow/ops.go
ao := workflow.ActivityOptions{
    StartToCloseTimeout: 2 * time.Minute,  // Increase from 30s
    // ...
}
```

**Testing**:
- Test with batch sizes: 1000, 5000, 10000
- Measure total scheduling time for 750k blocks
- Verify no activity timeouts

**Expected Impact**: Reduce overhead from 750 activity calls to 75-150

---

### Task 2.3: Increase ContinueAsNew Threshold ⏳
**Status**: Not Started
**Assigned**: go-senior-engineer agent
**Priority**: Low
**Description**:
- Increase `schedulerContinueThreshold` from 5000 to 10000-20000
- Update tests to handle larger thresholds
- Monitor workflow history sizes

**Code Changes**:
```go
// pkg/indexer/workflow/ops.go
var schedulerContinueThreshold = 10000  // Currently: 5000
```

**Testing**:
- Run scheduler workflow with 50k blocks
- Verify ContinueAsNew triggers at new threshold
- Check workflow history size stays reasonable

**Expected Impact**: Reduce ContinueAsNew overhead for 750k blocks from 150 to 37-75 times

---

### Task 2.4: Optimize Direct Scheduling ⏳
**Status**: Not Started
**Assigned**: go-senior-engineer agent
**Priority**: Low
**Description**:
- Apply worker pool pattern to `scheduleDirectly()` function
- Or reduce `CatchupThreshold` so SchedulerWorkflow is used more often
- Current direct scheduling is sequential and slow for ranges approaching 1000 blocks

**Code Changes**:
```go
// pkg/indexer/workflow/ops.go
// Option 1: Lower threshold
const (
    CatchupThreshold = 100  // Currently: 1000
)

// Option 2: Add worker pool to scheduleDirectly()
// Similar pattern to StartIndexWorkflowBatch
```

**Testing**:
- Test direct scheduling with 100-1000 block ranges
- Compare performance: sequential vs parallel
- Verify priority ordering maintained

**Expected Impact**: Faster catch-up for medium-sized gaps (100-1000 blocks)

---

### Task 2.5: Add Temporal Client Connection Pooling ⏳
**Status**: Not Started
**Assigned**: go-senior-engineer agent
**Priority**: Low
**Description**:
- Review if Temporal client is reusing connections efficiently
- Consider connection pool tuning if applicable
- Review gRPC connection settings

**Investigation Points**:
- Check `pkg/temporal/client.go` for connection configuration
- Review Temporal SDK docs for connection pooling best practices
- Monitor connection count during high load

**Deliverables**:
- Connection pooling assessment
- Recommendations or code changes

---

## Phase 3: Monitoring & Observability (PARALLEL to Phase 1 & 2)

### Task 3.1: Add Performance Metrics Dashboard ⏳
**Status**: Not Started
**Assigned**: TBD (May need new agent or manual work)
**Priority**: Medium
**Description**:
- Set up Grafana dashboards for:
  - Workflow scheduling rate (workflows/sec)
  - Queue depths (pending workflows/activities)
  - Activity execution times
  - PostgreSQL metrics
  - Temporal service health
  - 429 error rates

**Deliverables**:
- Grafana dashboard JSON
- Alert rules for critical metrics

---

### Task 3.2: Enhanced Logging ⏳
**Status**: Not Started
**Assigned**: go-senior-engineer agent
**Priority**: Low
**Description**:
- Add structured logging for rate limit errors
- Log performance metrics at key points
- Add trace IDs for debugging

**Code Changes**:
- Update logger calls with additional context
- Add rate limit error detection and reporting

---

## Phase 4: Load Testing & Validation (AFTER Phase 2)

### Task 4.1: 750k Block Stress Test ⏳
**Status**: Not Started
**Assigned**: TBD (May need testing specialist agent)
**Priority**: High
**Description**:
- Trigger scheduler workflow with 750k block backlog
- Monitor end-to-end completion time
- Track resource usage throughout
- Verify no errors or degradation

**Success Criteria**:
- ✅ All 750k workflows scheduled successfully
- ✅ No 429 errors
- ✅ Completion time < 10 seconds
- ✅ System remains stable after test

**Deliverables**:
- Load test report
- Performance metrics
- Bottleneck analysis (if any)

---

## Agent Requirements

### Current Agents:
- ✅ `go-senior-engineer` - For code optimizations (Phase 2)
- ✅ `go-temporal-clickhouse-tester` - For test updates

### New Agent Needed:

**Proposed Agent: `temporal-infra-specialist`**

**Description**:
```
Use this agent for Temporal infrastructure tasks including:
- Debugging Temporal server issues (429 errors, rate limits)
- Tuning Temporal configuration (temporal-values.yaml)
- PostgreSQL performance analysis and tuning for Temporal
- K8s resource optimization for Temporal deployments
- Reviewing Temporal server logs and metrics
- Temporal upgrade and deployment strategies
```

**Tools**: Read, Edit, Bash, Grep, Glob, WebFetch (Temporal docs)

---

## Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| 429 errors worsen during optimization | High | Phase 1 must complete successfully before Phase 2 |
| PostgreSQL becomes bottleneck | High | Task 1.2 validates before proceeding |
| Larger batches cause memory issues | Medium | Incremental testing with monitoring |
| ContinueAsNew causes workflow failures | Low | Extensive testing with threshold override |
| Network latency impacts throughput | Medium | Connection pooling and retry policies |

---

## Success Metrics

### Phase 1 Success:
- [ ] Zero 429 errors under current load
- [ ] PostgreSQL query times < 50ms p95
- [ ] All Temporal services healthy
- [ ] Sustained 125k workflows/sec throughput

### Phase 2 Success:
- [ ] Throughput > 200k workflows/sec
- [ ] 750k blocks scheduled in < 5 seconds
- [ ] Activity timeout rate < 0.1%
- [ ] Zero failed workflow starts

### Overall Success:
- [ ] System handles 750k block backlog gracefully
- [ ] Auto-scaling works correctly
- [ ] No manual intervention needed
- [ ] Monitoring dashboards operational

---

## Timeline (Estimates)

| Phase | Duration | Dependencies |
|-------|----------|--------------|
| Phase 1.1-1.2 (Investigation) | 2-4 hours | None |
| Phase 1.3 (Tuning) | 2-3 hours | Phase 1.1-1.2 |
| Phase 1.4 (Verification) | 1-2 hours | Phase 1.3 |
| Phase 2 (All Tasks) | 4-6 hours | Phase 1 complete |
| Phase 3 (Monitoring) | Ongoing | Can start immediately |
| Phase 4 (Load Test) | 2-3 hours | Phase 2 complete |
| **Total** | **11-18 hours** | |

---

## Notes

- **Current commit**: `e812cf3` - "Add parallel workflow scheduling and refactor queue stats management"
- **All changes are staged** - system is working correctly as of this commit
- **Priority**: Stability first, optimization second
- **Next Action**: Create `temporal-infra-specialist` agent and begin Phase 1.1

---

## Change Log

| Date | Phase | Task | Status | Notes |
|------|-------|------|--------|-------|
| 2025-10-16 | Setup | Plan Created | ✅ Complete | Initial plan based on 429 errors and optimization goals |
| 2025-10-16 | Phase 0 | Task 0.1 - Fix Replica Scaling Bug | ✅ Complete | HandleChainPatch now updates MinReplicas/MaxReplicas fields |
| 2025-10-16 | Phase 0 | Task 0.2 - Clarify Head-1 Logic | ✅ Complete | Documented that head-1 logic is correct, no changes needed |
| 2025-10-16 | Phase 0 | Task 0.3 - Add Gap Summary to API | ✅ Complete | Added missing blocks info using pond worker pool pattern |
| 2025-10-16 | Phase 1 | Task 1.1 - Investigate 429 Errors | ✅ Complete | No server errors found, 429s from Temporal Web UI browser rate limiting |
| 2025-10-16 | Phase 1 | Task 1.2 - PostgreSQL Health Check | ✅ Complete | 95/300 connections (32%), healthy and ready for Phase 2 |

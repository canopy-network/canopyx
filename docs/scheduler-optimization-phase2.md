# Scheduler Optimization Phase 2: Worker Pool Parallelism Increase

## Overview

This document summarizes the changes made to increase the Temporal workflow scheduler throughput from 125k workflows/sec to 200k+ workflows/sec by optimizing worker pool parallelism.

## Changes Made

### 1. File: `/pkg/indexer/activity/context.go`

**Lines Modified:**
- **Line 50**: Updated comment from "two workers per CPU" to "four workers per CPU"
- **Line 86-87**: Changed multiplier from `n * 2` to `n * 4` with explanatory comment

**Before:**
```go
parallelism := n * 2
```

**After:**
```go
// Use 4x CPU multiplier for increased throughput (target: 200k+ workflows/sec)
parallelism := n * 4
```

### 2. File: `/pkg/indexer/activity/context_test.go` (NEW)

Created comprehensive test suite with:
- **TestSchedulerParallelism**: Validates parallelism calculation with various overrides
- **TestSchedulerQueueSize**: Validates queue sizing logic
- **TestSchedulerQueueSizeBounds**: Ensures queue size always respects min/max bounds
- **TestSchedulerPoolSizeIntegration**: Tests the Context.SchedulerPoolSize() method
- **Benchmarks**: Performance benchmarks for both functions

## Performance Impact

### Current System (32 CPUs)
- **Old Parallelism**: 64 workers (2x CPU)
- **New Parallelism**: 128 workers (4x CPU)
- **Multiplier**: 2.00x
- **Expected Throughput Increase**: ~100%

### Projected Performance
- **Current**: 125k workflows/sec (1k workflows in 8ms)
- **Target**: 200k+ workflows/sec (1k workflows in ~5ms)
- **750k block batch**: ~3 seconds (down from ~6 seconds)

## Safety Measures Maintained

1. **Override Mechanism**: `SchedulerMaxParallelism` still works for testing/tuning
2. **Maximum Cap**: 512 workers (safety limit unchanged)
3. **Minimum Workers**: 2 workers (safety floor unchanged)
4. **Queue Bounds**: 4,096 - 262,144 (prevents memory exhaustion)

## Queue Size Behavior

The queue size calculation automatically scales with parallelism:
- Formula: `queue = parallelism * batchSize`
- With 128 workers and 750k batch: caps at 262,144 (max limit)
- This provides sufficient buffering without excessive memory usage

## Testing Strategy

### Unit Tests (Completed)
```bash
go test ./pkg/indexer/activity/... -v
```
All tests pass, including new scheduler-specific tests.

### Integration Testing (Recommended)

1. **Baseline Measurement**
   - Deploy to staging/dev environment
   - Monitor logs for "Batch workflow scheduling completed" messages
   - Record: `parallelism`, `duration_ms`, `workflows_per_sec`

2. **Load Test with 750k Blocks**
   - Trigger batch scheduling of 750k blocks
   - Expected results:
     - Parallelism: 128 (on 32-CPU system)
     - Duration: ~3-4 seconds
     - Throughput: 200k+ workflows/sec
   - Monitor: PostgreSQL CPU, Temporal API latency, goroutine count

3. **Verify Logging**
   Check logs contain parallelism field (already present in ops.go line 324):
   ```
   logger.Info("Batch workflow scheduling completed",
       zap.String("chain_id", in.ChainID),
       zap.Int("scheduled", scheduledValue),
       zap.Int("failed", failedValue),
       zap.Float64("duration_ms", durationMs),
       zap.Float64("workflows_per_sec", rate),
       zap.Int("parallelism", parallelism),  // ← Verify this logs 128
   )
   ```

4. **Stress Testing**
   - Run multiple concurrent batches
   - Monitor for goroutine leaks
   - Verify pond pool cleanup

## Monitoring & Observability

### Key Metrics to Watch

1. **Scheduler Throughput**
   - Metric: `workflows_per_sec` in logs
   - Target: >200k workflows/sec
   - Alert if: <150k workflows/sec sustained

2. **PostgreSQL Utilization**
   - Current baseline: 32% CPU
   - Expected increase: ~50-60% (still healthy)
   - Alert if: >85% CPU

3. **Temporal API Latency**
   - Monitor `ExecuteWorkflow` RPC latency
   - Should remain <10ms p99
   - Alert if: >50ms p99

4. **Memory Usage**
   - Worker pool + queue: ~few MB per worker
   - 128 workers: ~10-20 MB overhead
   - Monitor for leaks

5. **Goroutine Count**
   - Baseline: ~100-200
   - With 128 workers: ~250-350
   - Alert if: sustained >1000

## Rollback Plan

If issues arise, you can:

1. **Environment Variable Override** (Quick)
   - Set `SCHEDULER_MAX_PARALLELISM=64` (revert to 2x)
   - Requires application restart

2. **Code Rollback** (Full)
   - Revert commit with: `git revert <commit-hash>`
   - Change multiplier back to `n * 2`

## Potential Risks & Mitigations

### Risk 1: Temporal API Rate Limiting
**Symptoms**: High error rate, "resource exhausted" errors
**Mitigation**:
- Monitor Temporal server metrics
- The 512-worker cap prevents excessive load
- Can override with lower value if needed

### Risk 2: PostgreSQL Connection Pool Exhaustion
**Symptoms**: "too many connections" errors
**Mitigation**:
- Current infrastructure shows headroom (32% utilization)
- Database connection pool should scale with workers
- Monitor `pg_stat_activity` for connection count

### Risk 3: Memory Pressure
**Symptoms**: OOM kills, high swap usage
**Mitigation**:
- Queue cap at 262,144 prevents unbounded growth
- Monitor container memory usage
- Can reduce parallelism if needed

### Risk 4: Increased Temporal Server Load
**Symptoms**: Temporal UI slow, workflow start latency increases
**Mitigation**:
- Phase 1 confirmed infrastructure readiness
- Temporal is designed for high throughput
- Can tune batch sizes if needed

## Next Steps

1. **Deploy to Dev/Staging**
   - Verify parallelism value in logs (should be 128 on 32-CPU)
   - Run 750k block batch test
   - Measure actual throughput

2. **Production Rollout** (if dev tests successful)
   - Deploy during low-traffic period
   - Monitor metrics closely for 1 hour
   - Gradual rollout if multi-region

3. **Phase 3 Considerations** (if further optimization needed)
   - Tune batch sizes (currently dynamic)
   - Optimize Temporal RPC client pooling
   - Database query optimization
   - Consider 6x or 8x multiplier (if infrastructure supports)

## Configuration Reference

### Default Behavior
- **CPU Count**: Detected via `runtime.NumCPU()`
- **Multiplier**: 4x
- **Min Workers**: 2
- **Max Workers**: 512

### Override via Code
```go
ctx := &activity.Context{
    SchedulerMaxParallelism: 256,  // Force specific parallelism
}
```

### Production Tuning Guide
| CPU Count | 2x (Old) | 4x (New) | 6x | 8x |
|-----------|----------|----------|----|----|
| 8         | 16       | 32       | 48 | 64 |
| 16        | 32       | 64       | 96 | 128 |
| 32        | 64       | 128      | 192 | 256 |
| 64        | 128      | 256      | 384 | 512* |
| 128       | 256      | 512*     | 512* | 512* |

\* Capped at 512 (safety limit)

## References

- **Original Issue**: Optimize scheduler throughput to 200k+ workflows/sec
- **Phase 1**: Infrastructure investigation (completed - PostgreSQL at 32% utilization)
- **Phase 2**: This optimization (worker pool parallelism increase)
- **Temporal Docs**: [High Throughput Best Practices](https://docs.temporal.io/dev-guide/worker-performance)
- **Pond Library**: [alitto/pond](https://github.com/alitto/pond) - Worker pool implementation

## Validation Checklist

Before deploying to production:

- [x] Unit tests pass
- [x] Code compiles without warnings
- [x] Logging includes parallelism value
- [ ] Dev/staging deployment successful
- [ ] Throughput meets target (200k+ workflows/sec)
- [ ] PostgreSQL utilization <85%
- [ ] Temporal API latency <50ms p99
- [ ] No memory leaks observed
- [ ] No goroutine leaks observed
- [ ] 750k block batch completes in <5 seconds
- [ ] Production deployment plan approved

## Author Notes

This optimization is a conservative 2x increase in parallelism. The infrastructure investigation (Phase 1) confirmed we have significant headroom:
- PostgreSQL at 32% CPU utilization
- Temporal handling load well
- No I/O bottlenecks observed

If this 4x multiplier proves successful and we need even higher throughput, we can consider:
- 6x multiplier: 192 workers on 32-CPU (3x current)
- 8x multiplier: 256 workers on 32-CPU (4x current)

The 512-worker cap provides safety while allowing growth on larger systems.
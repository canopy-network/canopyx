# Hard Delete Workflow Architecture Plan

## Overview
Implement asynchronous hard delete using Temporal workflows with proper state tracking to prevent REPLICA_ALREADY_EXISTS errors.

## Problem Statement
Current hard delete implementation:
- Uses `DROP DATABASE ON CLUSTER` without `SYNC`
- Doesn't wait for replica cleanup completion
- Leaves orphaned metadata in ClickHouse Keeper
- Causes REPLICA_ALREADY_EXISTS errors on re-registration
- Blocks HTTP requests during deletion

## Solution Architecture

### Chain Deletion States

```go
const (
    ChainActive       = 0  // Active chain (normal operation)
    ChainSoftDeleted  = 1  // Soft deleted (recoverable, schedules paused)
    ChainHardDeleting = 2  // Hard delete in progress (LOCKED - no re-registration)
    // After hard delete completes: chain record removed entirely
)
```

### Workflow Design

#### Workflow 1: InitiateHardDeleteWorkflow

**Triggered by**: HTTP DELETE /api/chains/{id}?hard=true
**Duration**: ~5 seconds
**Returns**: Immediately with 202 Accepted

**Steps**:
```
1. Activity: MarkChainDeleting
   - UPDATE chains SET deleted = 2 WHERE id = {chainID}
   - This LOCKS the chain ID from re-registration

2. Activity: DropDatabaseAsync
   - Execute: DROP DATABASE IF EXISTS "chain_{id}" ON CLUSTER canopyx
   - Note: NO SYNC - returns immediately
   - Database drop happens asynchronously across replicas

3. Activity: CreateCleanupSchedule
   - Create Temporal schedule: "hard-delete-check-{chainID}"
   - Runs CheckDeleteProgressWorkflow every 10 seconds
   - Schedule continues until deletion completes
```

**Error Handling**:
- If MarkChainDeleting fails: return error to user
- If DropDatabaseAsync fails: rollback to deleted=1, return error
- If CreateCleanupSchedule fails: log warning, manual cleanup needed

#### Workflow 2: CheckDeleteProgressWorkflow

**Triggered by**: Temporal Schedule (every 10s)
**Duration**: <10 seconds per execution
**Purpose**: Poll for database deletion completion and cleanup

**Steps**:
```
1. Activity: CheckDatabaseExists
   - Query: SELECT count() FROM clusterAllReplicas('canopyx', system.databases)
            WHERE name = 'chain_{id}'
   - Returns: true if database exists on ANY replica

2. IF database still exists:
   - Log progress: "Database chain_{id} still exists, check #{count}"
   - Return (schedule runs again in 10s)

3. IF database fully dropped (exists = false):
   - Activity: CleanupReplicaMetadata
     - For each table (52 tables total):
       - For each replica (chi-canopyx-canopyx-0-0, chi-canopyx-canopyx-0-1):
         - SYSTEM DROP REPLICA '{replica}' FROM ZKPATH '/clickhouse/tables/0/chain_{id}/{table}'
     - Confirms all Keeper metadata is removed

   - Activity: RemoveCrossChainSync
     - Call adminDB.RemoveCrossChainSync(chainID)

   - Activity: DeleteTemporalSchedules
     - Delete schedule: chain:{id}:headscan
     - Delete schedule: chain:{id}:gapscan
     - Delete schedule: chain:{id}:pollsnapshot

   - Activity: DeleteHealthRecords
     - adminDB.DeleteEndpointsForChain(chainID)

   - Activity: DeleteIndexProgress
     - adminDB.DeleteIndexProgressForChain(chainID)

   - Activity: DeleteReindexRequests
     - adminDB.DeleteReindexRequestsForChain(chainID)

   - Activity: DeleteCleanupSchedule
     - Delete this schedule: hard-delete-check-{chainID}

   - Activity: DeleteChainRecord
     - DELETE FROM chains WHERE id = {chainID}
     - Chain record completely removed

   - Mark workflow as completed
   - No more scheduled runs
```

## Database Schema Changes

### chains table modification

```sql
ALTER TABLE canopyx_indexer.chains
ADD COLUMN IF NOT EXISTS deleted UInt8 DEFAULT 0
COMMENT '0=active, 1=soft deleted, 2=hard delete in progress';
```

## Code Implementation Files

### New Files

1. **app/admin/workflow/hard_delete_chain.go**
   ```go
   package workflow

   // InitiateHardDeleteWorkflow starts async hard delete
   func InitiateHardDeleteWorkflow(ctx workflow.Context, chainID uint64) error

   // CheckDeleteProgressWorkflow polls for completion
   func CheckDeleteProgressWorkflow(ctx workflow.Context, chainID uint64, checkCount int) error
   ```

2. **app/admin/activity/hard_delete_chain.go**
   ```go
   package activity

   // Activities for hard delete workflow
   func (a *Activities) MarkChainDeleting(ctx context.Context, chainID uint64) error
   func (a *Activities) DropDatabaseAsync(ctx context.Context, chainID uint64) error
   func (a *Activities) CreateCleanupSchedule(ctx context.Context, chainID uint64, scheduleID string) error
   func (a *Activities) CheckDatabaseExists(ctx context.Context, chainID uint64) (bool, error)
   func (a *Activities) CleanupReplicaMetadata(ctx context.Context, chainID uint64) error
   func (a *Activities) RemoveCrossChainSync(ctx context.Context, chainID uint64) error
   func (a *Activities) DeleteTemporalSchedules(ctx context.Context, chainID uint64) error
   func (a *Activities) DeleteHealthRecords(ctx context.Context, chainID uint64) error
   func (a *Activities) DeleteIndexProgress(ctx context.Context, chainID uint64) error
   func (a *Activities) DeleteReindexRequests(ctx context.Context, chainID uint64) error
   func (a *Activities) DeleteCleanupSchedule(ctx context.Context, scheduleID string) error
   func (a *Activities) DeleteChainRecord(ctx context.Context, chainID uint64) error
   ```

### Modified Files

1. **pkg/db/models/admin/chain.go**
   - Add field: `Deleted uint8 json:"deleted"`

2. **pkg/db/admin/chain.go**
   - Add: `UpdateChainDeleted(ctx context.Context, chainID uint64, deleted uint8) error`
   - Modify: `GetChain()` to include deleted column
   - Add: `GetChainIncludingDeleted()` for status checks

3. **app/admin/controller/chain.go**
   - Modify: `HandleChainDelete()` - async pattern for hard delete
   - Add: `HandleChainDeleteStatus()` - GET /api/chains/{id}/delete-status
   - Modify: `HandleChainCreate()` - validate deleted state

4. **pkg/db/admin/db.go**
   - Remove: `cleanupReplicaMetadata()` (moved to activity)
   - Modify: `DropChainDatabase()` - just drop, no cleanup
   - Or deprecate entirely in favor of activity

## HTTP API Changes

### DELETE /api/chains/{id}?hard=true

**Before** (synchronous, ~6 seconds):
```
DELETE /api/chains/5?hard=true
Authorization: Bearer devtoken

Response 200 OK:
{"ok": "1"}
```

**After** (asynchronous, <1 second):
```
DELETE /api/chains/5?hard=true
Authorization: Bearer devtoken

Response 202 Accepted:
{
  "status": "deleting",
  "workflow_id": "hard-delete-init-5",
  "check_status_url": "/api/chains/5/delete-status",
  "message": "Hard delete in progress. Check status at the provided URL."
}
```

### GET /api/chains/{id}/delete-status (NEW)

**Purpose**: Check hard delete progress

```
GET /api/chains/5/delete-status
Authorization: Bearer devtoken

Response 200 OK (in progress):
{
  "chain_id": 5,
  "status": "deleting",
  "deleted_state": 2,
  "workflow_id": "hard-delete-init-5",
  "cleanup_schedule_id": "hard-delete-check-5",
  "checks_performed": 42,
  "elapsed_minutes": 7.0,
  "message": "Database drop in progress, polling..."
}

Response 200 OK (completed):
{
  "chain_id": 5,
  "status": "deleted",
  "message": "Chain completely removed"
}

Response 404 Not Found:
{
  "error": "Chain not found"
}
```

## Chain Registration Validation

### HandleChainCreate validation logic

```go
func (c *Controller) HandleChainCreate(w http.ResponseWriter, r *http.Request) {
    // ... parse request ...

    // Check if chain ID is locked or deleted
    existingChain, err := c.App.AdminDB.GetChainIncludingDeleted(ctx, chainID)
    if err == nil {
        switch existingChain.Deleted {
        case 2:
            http.Error(w, fmt.Sprintf(
                "Chain ID %d is locked - hard delete in progress. "+
                "Check status at /api/chains/%d/delete-status",
                chainID, chainID),
                http.StatusConflict)
            return
        case 1:
            http.Error(w, fmt.Sprintf(
                "Chain ID %d is soft-deleted. Recover it first at "+
                "POST /api/chains/%d/recover",
                chainID, chainID),
                http.StatusConflict)
            return
        default: // deleted = 0
            http.Error(w,
                fmt.Sprintf("Chain ID %d already exists", chainID),
                http.StatusConflict)
            return
        }
    }

    // Chain doesn't exist, proceed with creation
    // ...
}
```

## Activity Configuration

### Timeouts and Retry Policies

```go
// Short activities (mark, drop, create schedule)
shortActivityOptions := workflow.ActivityOptions{
    StartToCloseTimeout: 90 * time.Second,
    RetryPolicy: &temporal.RetryPolicy{
        MaximumAttempts: 3,
        InitialInterval: 1 * time.Second,
    },
}

// Check activities (polling database)
checkActivityOptions := workflow.ActivityOptions{
    StartToCloseTimeout: 60 * time.Second,
    RetryPolicy: &temporal.RetryPolicy{
        MaximumAttempts: 3,
        InitialInterval: 2 * time.Second,
    },
}

// Cleanup activities (SYSTEM DROP REPLICA)
cleanupActivityOptions := workflow.ActivityOptions{
    StartToCloseTimeout: 5 * time.Minute, // 52 tables * 2 replicas = 104 commands
    RetryPolicy: &temporal.RetryPolicy{
        MaximumAttempts: 3,
        InitialInterval: 5 * time.Second,
    },
}
```

## Schedule Configuration

```go
scheduleOptions := client.ScheduleOptions{
    ID: fmt.Sprintf("hard-delete-check-%d", chainID),
    Spec: client.ScheduleSpec{
        Intervals: []client.ScheduleIntervalSpec{
            {Every: 30 * time.Second},
        },
    },
    Action: &client.ScheduleWorkflowAction{
        Workflow: CheckDeleteProgressWorkflow,
        Args:     []interface{}{chainID, 0},
        TaskQueue: "admin:maintenance",
    },
    Overlap: enums.SCHEDULE_OVERLAP_POLICY_SKIP, // Skip if previous still running
}
```

## Monitoring and Observability

### Metrics to Track
- `chain_hard_delete_initiated_total` - Counter
- `chain_hard_delete_completed_total` - Counter
- `chain_hard_delete_failed_total` - Counter
- `chain_hard_delete_duration_seconds` - Histogram
- `chain_hard_delete_checks_count` - Histogram

### Logs to Emit
- Chain marked as deleted=2
- Database drop initiated
- Cleanup schedule created
- Each check execution (with count and elapsed time)
- Database fully dropped detected
- Each cleanup step completion
- Final chain record deletion

### Temporal UI Visibility
- Workflow execution history
- Schedule execution history
- Activity retry attempts
- Error stack traces

## Testing Strategy

### Unit Tests
1. Test `MarkChainDeleting` activity
2. Test `CheckDatabaseExists` with mock ClickHouse responses
3. Test schedule creation/deletion

### Integration Tests
1. Full hard delete flow with small test chain
2. Test interrupted delete (kill workflow mid-execution)
3. Test cleanup schedule resilience
4. Test chain ID locking during delete

### Manual Testing Checklist
- [ ] Hard delete chain with 100 blocks
- [ ] Hard delete chain with 10,000 blocks
- [ ] Verify REPLICA_ALREADY_EXISTS doesn't occur on re-add
- [ ] Test interrupting delete (kill admin pod)
- [ ] Verify schedule auto-recovers
- [ ] Check delete status endpoint accuracy
- [ ] Attempt to register chain while deleted=2 (should fail)

## Migration Plan

### Phase 1: Schema Update
```sql
-- Add deleted column to existing chains table
ALTER TABLE canopyx_indexer.chains
ADD COLUMN IF NOT EXISTS deleted UInt8 DEFAULT 0;

-- Migrate existing soft-deleted chains
-- (if you have a deleted boolean column, migrate it)
ALTER TABLE canopyx_indexer.chains
UPDATE deleted = 1 WHERE deleted_old = true;
```

### Phase 2: Code Deployment
1. Deploy new admin service with workflow code
2. Worker picks up new workflows automatically
3. HTTP endpoints immediately available

### Phase 3: Testing
1. Test with non-production chains first
2. Monitor Temporal UI for workflow execution
3. Verify cleanup schedules complete successfully

### Phase 4: Cleanup Old Chains
1. Use new hard delete on old test chains
2. Verify no REPLICA_ALREADY_EXISTS errors
3. Re-add chains to confirm clean state

## Known Limitations

1. **Deletion time unpredictable**: Could be seconds or hours depending on data size
2. **Manual intervention needed if**: Schedule gets deleted before completion
3. **Zombie schedules**: If workflow fails to delete schedule, requires manual cleanup
4. **No progress percentage**: Only know "exists" or "doesn't exist", not % complete

## Future Enhancements

1. **Progress estimation**: Query table row counts to estimate progress
2. **Cancellation support**: Allow user to cancel hard delete mid-flight
3. **Batch hard delete**: Delete multiple chains in parallel
4. **Notification on completion**: Webhook or email when delete finishes
5. **Automatic retry on failure**: Auto-retry failed cleanup steps

## References

- [ClickHouse SYSTEM DROP REPLICA](https://clickhouse.com/docs/sql-reference/statements/system)
- [Temporal Schedules Documentation](https://docs.temporal.io/develop/go/schedules)
- [ClickHouse clusterAllReplicas](https://kb.altinity.com/altinity-kb-setup-and-maintenance/sysall/)
- [Temporal Schedule Delete](https://docs.temporal.io/develop/go/schedules)

## Implementation Checklist

- [ ] Update chain model with `deleted` field
- [ ] Create `InitiateHardDeleteWorkflow`
- [ ] Create `CheckDeleteProgressWorkflow`
- [ ] Create all 12 activities
- [ ] Update `HandleChainDelete` to async pattern
- [ ] Add `HandleChainDeleteStatus` endpoint
- [ ] Update `HandleChainCreate` validation
- [ ] Write unit tests
- [ ] Write integration tests
- [ ] Update API documentation
- [ ] Test with mock chains
- [ ] Deploy to staging
- [ ] Monitor production rollout

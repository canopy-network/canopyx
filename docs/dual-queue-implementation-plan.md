# Dual-Queue Architecture Implementation Plan
**Date**: October 17, 2025
**Goal**: Separate live block indexing from historical backfill to ensure predictable low-latency for chain head

---

## Executive Summary

### Current Problem
- Single queue `index:canopy_local` handles BOTH historical (500k+ workflows) AND live blocks
- Even with priority 5, live blocks compete with massive historical backlog
- 12 indexers mostly busy with historical blocks, leaving only 1-2 for live blocks
- Result: 70+ block gap at head with unpredictable latency

### Proposed Solution
**Three workers within same indexer process:**
1. **OpsWorker** - Operations queue (scheduler, headscan, gapscan) - UNCHANGED
2. **LiveIndexWorker** - Live blocks queue (last 200 blocks from head)
3. **HistoricalIndexWorker** - Historical blocks queue (everything else)

### Benefits
- **Guaranteed live sync** - Live queue never waits behind historical backlog
- **Predictable latency** - Live blocks indexed within seconds
- **Independent scaling** - Scale historical workers without affecting live performance
- **Same deployment** - No controller changes, same pods, same image

---

## Queue Naming Convention

### Current
```
index:canopy_local        → All IndexBlock workflows (live + historical)
admin:canopy_local        → Operations (HeadScan, GapScan, SchedulerWorkflow)
```

### Proposed
```
index:canopy_local:live       → Live blocks (within 200 of head)
index:canopy_local:historical → Historical blocks (>200 behind head)
admin:canopy_local            → Operations (UNCHANGED)
```

---

## Live Block Threshold: 200 blocks

**Rationale:**
- Canopy block time: ~20 seconds per block
- 200 blocks = ~67 minutes of chain history
- Provides buffer for temporary indexing slowdowns
- Avoids flapping between queues during normal operations

**Logic:**
```go
func IsLiveBlock(latest, height uint64) bool {
    return latest - height <= 200
}
```

---

## Implementation Phases

### Phase 1: Core Infrastructure Changes

#### 1.1 Update Temporal Client Queue Functions
**File**: `pkg/temporal/client.go`

**Changes:**
```go
type Client struct {
    // ... existing fields ...

    // Task Queues - UPDATED
    ManagerQueue         string // manager
    ReportsQueue         string // reports
    IndexerQueue         string // index:<chainID>         [DEPRECATED - remove after migration]
    IndexerLiveQueue     string // index:<chainID>:live    [NEW]
    IndexerHistoricalQueue string // index:<chainID>:historical [NEW]
    IndexerOpsQueue      string // admin:<chainID>

    // ... rest unchanged ...
}

func NewClient(...) (*Client, error) {
    return &Client{
        // ... existing ...
        IndexerQueue:           "index:%s",         // Keep for backwards compat during migration
        IndexerLiveQueue:       "index:%s:live",    // NEW
        IndexerHistoricalQueue: "index:%s:historical", // NEW
        // ... rest unchanged ...
    }, nil
}

// NEW: GetIndexerLiveQueue returns the live indexer queue for the given chain
func (c *Client) GetIndexerLiveQueue(chainID string) string {
    return fmt.Sprintf(c.IndexerLiveQueue, chainID)
}

// NEW: GetIndexerHistoricalQueue returns the historical indexer queue for the given chain
func (c *Client) GetIndexerHistoricalQueue(chainID string) string {
    return fmt.Sprintf(c.IndexerHistoricalQueue, chainID)
}

// KEEP for backwards compatibility during migration
func (c *Client) GetIndexerQueue(chainID string) string {
    return fmt.Sprintf(c.IndexerQueue, chainID)
}
```

#### 1.2 Add Live Block Detection Function
**File**: `pkg/indexer/workflow/ops.go`

**Changes:**
```go
const (
    // ... existing constants ...

    // --- Live Block Threshold (determines queue routing)
    LiveBlockThreshold = 200 // Blocks within 200 of head are considered "live"
)

// IsLiveBlock determines if a block should be routed to the live queue.
// Blocks within LiveBlockThreshold of the chain head are considered live.
func IsLiveBlock(latest, height uint64) bool {
    if height > latest {
        return true // Future blocks are live
    }
    return latest - height <= LiveBlockThreshold
}
```

---

### Phase 2: Worker Configuration

#### 2.1 Create Three Workers in Indexer Process
**File**: `app/indexer/app.go`

**Changes:**
```go
type App struct {
    LiveWorker       worker.Worker  // NEW: Live block indexing
    HistoricalWorker worker.Worker  // NEW: Historical block indexing
    OpsWorker        worker.Worker  // UNCHANGED: Operations (headscan, gapscan, scheduler)
    TemporalClient   *temporal.Client
    Logger           *zap.Logger
}

func (a *App) Start(ctx context.Context) {
    if err := a.LiveWorker.Start(); err != nil {
        a.Logger.Fatal("Unable to start live worker", zap.Error(err))
    }
    if err := a.HistoricalWorker.Start(); err != nil {
        a.Logger.Fatal("Unable to start historical worker", zap.Error(err))
    }
    if err := a.OpsWorker.Start(); err != nil {
        a.Logger.Fatal("Unable to start operations worker", zap.Error(err))
    }
    <-ctx.Done()
    a.Stop()
}

func (a *App) Stop() {
    a.LiveWorker.Stop()
    a.HistoricalWorker.Stop()
    a.OpsWorker.Stop()
    time.Sleep(200 * time.Millisecond)
    a.Logger.Info("さようなら!")
}

func Initialize(ctx context.Context) *App {
    // ... existing initialization code ...

    // UPDATED: Live Worker - Optimized for low-latency, high-priority blocks
    liveWorker := worker.New(
        temporalClient.TClient,
        temporalClient.GetIndexerLiveQueue(chainID),
        worker.Options{
            // Lower poller count - live queue should have small backlog
            MaxConcurrentWorkflowTaskPollers: 10,
            MaxConcurrentActivityTaskPollers: 10,
            // Lower execution limits - focus on throughput per block
            MaxConcurrentWorkflowTaskExecutionSize: 100,
            MaxConcurrentActivityExecutionSize:     200,
            WorkerStopTimeout:                      1 * time.Minute,
        },
    )

    // UPDATED: Historical Worker - Optimized for high-throughput batch processing
    historicalWorker := worker.New(
        temporalClient.TClient,
        temporalClient.GetIndexerHistoricalQueue(chainID),
        worker.Options{
            // Higher poller count for large backlogs
            MaxConcurrentWorkflowTaskPollers: 50,
            MaxConcurrentActivityTaskPollers: 50,
            // High execution limits for parallel processing
            MaxConcurrentWorkflowTaskExecutionSize: 2000,
            MaxConcurrentActivityExecutionSize:     5000,
            WorkerStopTimeout:                      1 * time.Minute,
        },
    )

    // Register IndexBlockWorkflow on BOTH workers (same workflow, different queues)
    liveWorker.RegisterWorkflowWithOptions(
        workflowContext.IndexBlockWorkflow,
        temporalworkflow.RegisterOptions{Name: workflow.IndexBlockWorkflowName},
    )
    historicalWorker.RegisterWorkflowWithOptions(
        workflowContext.IndexBlockWorkflow,
        temporalworkflow.RegisterOptions{Name: workflow.IndexBlockWorkflowName},
    )

    // Register activities on BOTH workers
    for _, w := range []worker.Worker{liveWorker, historicalWorker} {
        w.RegisterActivity(activityContext.PrepareIndexBlock)
        w.RegisterActivity(activityContext.FetchBlockFromRPC)
        w.RegisterActivity(activityContext.SaveBlock)
        w.RegisterActivity(activityContext.IndexBlock)
        w.RegisterActivity(activityContext.IndexTransactions)
        w.RegisterActivity(activityContext.SaveBlockSummary)
        w.RegisterActivity(activityContext.RecordIndexed)
    }

    // OpsWorker - UNCHANGED
    opsWorker := worker.New(
        temporalClient.TClient,
        temporalClient.GetIndexerOpsQueue(chainID),
        worker.Options{
            MaxConcurrentWorkflowTaskPollers: 5,
            MaxConcurrentActivityTaskPollers: 5,
            WorkerStopTimeout:                1 * time.Minute,
        },
    )

    // ... register ops workflows/activities (UNCHANGED) ...

    return &App{
        LiveWorker:       liveWorker,
        HistoricalWorker: historicalWorker,
        OpsWorker:        opsWorker,
        TemporalClient:   temporalClient,
        Logger:           logger,
    }
}
```

---

### Phase 3: Workflow Scheduling Logic

#### 3.1 Update StartIndexWorkflow Activity to Route by Queue
**File**: `pkg/indexer/activity/ops.go`

**Current behavior**: Always schedules to `index:canopy_local`

**New behavior**: Route based on block height vs chain head

**Changes:**
```go
// StartIndexWorkflow activity - UPDATED to route to correct queue
func (c *Context) StartIndexWorkflow(ctx context.Context, in types.IndexBlockInput) error {
    logger := c.Logger.With(
        zap.String("chain_id", in.ChainID),
        zap.Uint64("height", in.Height),
        zap.Int("priority", in.PriorityKey),
    )

    wfID := c.TemporalClient.GetIndexBlockWorkflowId(in.ChainID, in.Height)

    // NEW: Get latest height to determine queue routing
    latest, err := c.GetLatestHead(ctx, &types.ChainIdInput{ChainID: in.ChainID})
    if err != nil {
        logger.Warn("Failed to get latest head for queue routing, using historical queue",
            zap.Error(err),
        )
        latest = 0 // Default to historical queue on error
    }

    // NEW: Determine target queue based on block age
    var taskQueue string
    if workflow.IsLiveBlock(latest, in.Height) {
        taskQueue = c.TemporalClient.GetIndexerLiveQueue(in.ChainID)
        logger.Debug("Routing to live queue",
            zap.String("task_queue", taskQueue),
            zap.Uint64("latest", latest),
            zap.Uint64("height", in.Height),
        )
    } else {
        taskQueue = c.TemporalClient.GetIndexerHistoricalQueue(in.ChainID)
        logger.Debug("Routing to historical queue",
            zap.String("task_queue", taskQueue),
            zap.Uint64("latest", latest),
            zap.Uint64("height", in.Height),
        )
    }

    options := client.StartWorkflowOptions{
        ID:        wfID,
        TaskQueue: taskQueue, // UPDATED: Dynamic queue selection
        RetryPolicy: &sdktemporal.RetryPolicy{
            InitialInterval:    time.Second,
            BackoffCoefficient: 1.2,
            MaximumInterval:    5 * time.Second,
            MaximumAttempts:    0,
        },
    }

    // Set priority if provided (still useful for ordering within queue)
    if in.PriorityKey > 0 {
        options.Priority = sdktemporal.Priority{PriorityKey: in.PriorityKey}
    }

    // ... rest of function unchanged ...
}
```

#### 3.2 Update StartIndexWorkflowBatch Activity
**File**: `pkg/indexer/activity/ops.go`

**Changes**: Similar routing logic as above, but for batch operations

```go
// StartIndexWorkflowBatch activity - UPDATED to route to correct queue
func (c *Context) StartIndexWorkflowBatch(ctx context.Context, in types.BatchScheduleInput) (interface{}, error) {
    // ... existing validation ...

    // NEW: Get latest height for queue routing
    latest, err := c.GetLatestHead(ctx, &types.ChainIdInput{ChainID: in.ChainID})
    if err != nil {
        logger.Warn("Failed to get latest head for batch routing, using historical queue",
            zap.Error(err),
        )
        latest = 0
    }

    // NEW: Determine if this batch is entirely live, entirely historical, or mixed
    batchIsLive := workflow.IsLiveBlock(latest, in.StartHeight) && workflow.IsLiveBlock(latest, in.EndHeight)
    batchIsHistorical := !workflow.IsLiveBlock(latest, in.StartHeight) && !workflow.IsLiveBlock(latest, in.EndHeight)

    var taskQueue string
    if batchIsLive {
        taskQueue = c.TemporalClient.GetIndexerLiveQueue(in.ChainID)
        logger.Info("Batch routing to live queue",
            zap.String("task_queue", taskQueue),
            zap.Uint64("start", in.StartHeight),
            zap.Uint64("end", in.EndHeight),
        )
    } else if batchIsHistorical {
        taskQueue = c.TemporalClient.GetIndexerHistoricalQueue(in.ChainID)
        logger.Info("Batch routing to historical queue",
            zap.String("task_queue", taskQueue),
            zap.Uint64("start", in.StartHeight),
            zap.Uint64("end", in.EndHeight),
        )
    } else {
        // Mixed batch - split it
        logger.Info("Mixed batch detected, splitting by queue",
            zap.Uint64("latest", latest),
            zap.Uint64("start", in.StartHeight),
            zap.Uint64("end", in.EndHeight),
        )

        // Find the boundary: latest - 200
        boundary := latest - workflow.LiveBlockThreshold

        // Schedule live portion (boundary+1 to end)
        if in.EndHeight > boundary {
            liveStart := boundary + 1
            if liveStart < in.StartHeight {
                liveStart = in.StartHeight
            }
            liveBatch := types.BatchScheduleInput{
                ChainID:     in.ChainID,
                StartHeight: liveStart,
                EndHeight:   in.EndHeight,
                PriorityKey: in.PriorityKey,
            }
            if err := c.scheduleBatchToQueue(ctx, liveBatch, c.TemporalClient.GetIndexerLiveQueue(in.ChainID)); err != nil {
                logger.Error("Failed to schedule live portion", zap.Error(err))
            }
        }

        // Schedule historical portion (start to boundary)
        if in.StartHeight <= boundary {
            histEnd := boundary
            if histEnd > in.EndHeight {
                histEnd = in.EndHeight
            }
            histBatch := types.BatchScheduleInput{
                ChainID:     in.ChainID,
                StartHeight: in.StartHeight,
                EndHeight:   histEnd,
                PriorityKey: in.PriorityKey,
            }
            if err := c.scheduleBatchToQueue(ctx, histBatch, c.TemporalClient.GetIndexerHistoricalQueue(in.ChainID)); err != nil {
                logger.Error("Failed to schedule historical portion", zap.Error(err))
            }
        }

        return map[string]interface{}{
            "split": true,
            "live_count": in.EndHeight - (latest - workflow.LiveBlockThreshold),
            "historical_count": (latest - workflow.LiveBlockThreshold) - in.StartHeight + 1,
        }, nil
    }

    // Schedule single-queue batch
    return c.scheduleBatchToQueue(ctx, in, taskQueue)
}

// NEW: Helper function to schedule a batch to a specific queue
func (c *Context) scheduleBatchToQueue(ctx context.Context, in types.BatchScheduleInput, taskQueue string) (interface{}, error) {
    // ... existing batch scheduling logic, but with taskQueue parameter ...
}
```

---

### Phase 4: Controller Updates

#### 4.1 Update Controller to Monitor Both Queues
**File**: `app/controller/app.go`

**Changes**: Controller needs to aggregate stats from both queues

```go
// ensureChain - UPDATED to monitor both queues
func (a *App) ensureChain(ctx context.Context, c db.Chain) error {
    // ... existing code ...

    // NEW: Get stats from BOTH queues and aggregate
    liveQueue := a.TemporalClient.GetIndexerLiveQueue(c.ChainID)
    historicalQueue := a.TemporalClient.GetIndexerHistoricalQueue(c.ChainID)

    livePendingWF, livePendingAct, livePollers, liveBacklogAge, liveErr := a.TemporalClient.GetQueueStats(ctx, liveQueue)
    histPendingWF, histPendingAct, histPollers, histBacklogAge, histErr := a.TemporalClient.GetQueueStats(ctx, historicalQueue)

    // Aggregate stats
    totalPendingWF := livePendingWF + histPendingWF
    totalPendingAct := livePendingAct + histPendingAct
    totalPollers := livePollers + histPollers
    maxBacklogAge := liveBacklogAge
    if histBacklogAge > maxBacklogAge {
        maxBacklogAge = histBacklogAge
    }

    // Log both queues separately for visibility
    a.Logger.Info("Queue stats",
        "chain_id", c.ChainID,
        "live_queue_wf", livePendingWF,
        "live_queue_act", livePendingAct,
        "historical_queue_wf", histPendingWF,
        "historical_queue_act", histPendingAct,
        "total_pollers", totalPollers,
    )

    // Use aggregated stats for scaling decisions
    // ... rest of scaling logic uses totalPendingWF, totalPendingAct, etc ...
}
```

#### 4.2 Update Health Check Messages
**File**: `app/controller/app.go`

**Changes**: Report both queue depths separately

```go
// buildQueueHealthStatus - UPDATED to show both queues
func (a *App) buildQueueHealthStatus(c db.Chain, ...) (string, string) {
    // ... existing code ...

    msg := fmt.Sprintf(
        "Live queue: %d tasks (age: %.0fs), Historical queue: %d tasks (age: %.0fs)",
        livePendingTotal,
        liveBacklogAge,
        histPendingTotal,
        histBacklogAge,
    )

    // ... threshold logic based on total ...
}
```

---

### Phase 5: Admin API Updates

#### 5.1 Add Queue Selection to Trigger Endpoints
**File**: `app/admin/controller/chain.go`

**Impact**: Minimal - endpoints can stay the same, routing handled by activities

**Optional Enhancement**: Add explicit queue parameter for manual triggers
```go
// POST /api/chains/:chainId/trigger-index
type TriggerIndexRequest struct {
    StartHeight uint64 `json:"start_height"`
    EndHeight   uint64 `json:"end_height"`
    Queue       string `json:"queue,omitempty"` // NEW: Optional - "live", "historical", "auto" (default)
}
```

---

### Phase 6: UI Updates (Admin Web)

**Priority**: High - Addresses Task 0.3 UI requirements from performance-optimization-plan.md

**Goals**:
1. Fix misleading progress display (currently shows 0.007% when actually live-synced)
2. Separate "Live Sync Status" from "Historical Progress"
3. Display dual-queue health metrics after implementation

---

#### 6.1 Update API Response Schema (Backend First)
**File**: `app/admin/controller/chain.go` (GET /api/chains/:chainId)

**Changes**: Add fields for live sync status and gap information

```go
type ChainResponse struct {
    // ... existing fields ...

    // PHASE 1: Task 0.3 Fields (Already implemented in backend)
    LastIndexed          uint64  `json:"last_indexed"`
    Head                 uint64  `json:"head"`
    MissingBlocksCount   uint64  `json:"missing_blocks_count"`
    GapRangesCount       int     `json:"gap_ranges_count"`
    LargestGapStart      uint64  `json:"largest_gap_start"`
    LargestGapEnd        uint64  `json:"largest_gap_end"`
    IsLiveSync           bool    `json:"is_live_sync"`

    // PHASE 2: Dual-Queue Fields (Will be added during dual-queue implementation)
    LiveQueueDepth          int64   `json:"live_queue_depth"`
    LiveQueueBacklogAge     float64 `json:"live_queue_backlog_age"`
    HistoricalQueueDepth    int64   `json:"historical_queue_depth"`
    HistoricalQueueBacklogAge float64 `json:"historical_queue_backlog_age"`

    // Keep aggregated for backwards compat
    QueueHealthMessage      string `json:"queue_health_message"`
}
```

---

#### 6.2 Update TypeScript Types
**File**: `web/admin/app/lib/types.ts` (or wherever chain types are defined)

**Changes**: Add new fields to Chain interface

```typescript
export interface Chain {
  // ... existing fields ...

  // Task 0.3 fields
  last_indexed: number;
  head: number;
  missing_blocks_count: number;
  gap_ranges_count: number;
  largest_gap_start: number;
  largest_gap_end: number;
  is_live_sync: boolean;

  // Dual-queue fields (Phase 2)
  live_queue_depth?: number;
  live_queue_backlog_age?: number;
  historical_queue_depth?: number;
  historical_queue_backlog_age?: number;

  queue_health_message: string;
}
```

---

#### 6.3 Create Live Sync Status Component
**File**: `web/admin/app/components/LiveSyncStatus.tsx` (NEW)

**Purpose**: Display live sync status separately from historical progress

```tsx
import { Chain } from '@/lib/types';

interface LiveSyncStatusProps {
  chain: Chain;
}

export function LiveSyncStatus({ chain }: LiveSyncStatusProps) {
  const { is_live_sync, last_indexed, head, missing_blocks_count } = chain;

  return (
    <div className="live-sync-status">
      {/* Live Sync Badge */}
      <div className="flex items-center gap-2">
        {is_live_sync ? (
          <span className="badge badge-success">
            ✓ Live Sync
          </span>
        ) : (
          <span className="badge badge-warning">
            ⏳ Catching Up
          </span>
        )}
        <span className="text-sm text-gray-600">
          At block {last_indexed.toLocaleString()} / {head.toLocaleString()}
        </span>
      </div>

      {/* Historical Backlog Info */}
      {missing_blocks_count > 0 && (
        <div className="historical-backlog mt-2">
          <div className="text-sm text-amber-600">
            ⚠ Historical Backlog: {missing_blocks_count.toLocaleString()} blocks missing
          </div>
          <div className="progress-bar mt-1">
            <div className="progress-fill"
                 style={{ width: `${((last_indexed / head) * 100).toFixed(2)}%` }}>
            </div>
          </div>
          <div className="text-xs text-gray-500 mt-1">
            {((last_indexed / head) * 100).toFixed(2)}% total indexed
          </div>
        </div>
      )}
    </div>
  );
}
```

---

#### 6.4 Create Gap Ranges Display Component
**File**: `web/admin/app/components/GapRangesDisplay.tsx` (NEW)

**Purpose**: Show gap information clearly

```tsx
import { Chain } from '@/lib/types';

interface GapRangesDisplayProps {
  chain: Chain;
}

export function GapRangesDisplay({ chain }: GapRangesDisplayProps) {
  const { gap_ranges_count, largest_gap_start, largest_gap_end, missing_blocks_count } = chain;

  if (gap_ranges_count === 0) {
    return (
      <div className="text-sm text-green-600">
        ✓ No gaps - fully indexed
      </div>
    );
  }

  const largestGapSize = largest_gap_end - largest_gap_start + 1;

  return (
    <div className="gap-ranges-info">
      <div className="text-sm font-medium">Gap Summary</div>
      <div className="grid grid-cols-2 gap-2 mt-2 text-sm">
        <div>
          <div className="text-gray-600">Total Missing:</div>
          <div className="font-semibold">{missing_blocks_count.toLocaleString()} blocks</div>
        </div>
        <div>
          <div className="text-gray-600">Gap Ranges:</div>
          <div className="font-semibold">{gap_ranges_count}</div>
        </div>
      </div>

      {gap_ranges_count === 1 && (
        <div className="mt-2 text-xs text-gray-500">
          Single gap: blocks {largest_gap_start.toLocaleString()} - {largest_gap_end.toLocaleString()}
        </div>
      )}

      {gap_ranges_count > 1 && (
        <div className="mt-2 text-xs text-gray-500">
          Largest gap: {largestGapSize.toLocaleString()} blocks
          ({largest_gap_start.toLocaleString()} - {largest_gap_end.toLocaleString()})
        </div>
      )}
    </div>
  );
}
```

---

#### 6.5 Create Dual-Queue Health Component (Phase 2)
**File**: `web/admin/app/components/DualQueueHealth.tsx` (NEW)

**Purpose**: Display live and historical queue stats separately

```tsx
import { Chain } from '@/lib/types';

interface DualQueueHealthProps {
  chain: Chain;
}

export function DualQueueHealth({ chain }: DualQueueHealthProps) {
  const {
    live_queue_depth = 0,
    live_queue_backlog_age = 0,
    historical_queue_depth = 0,
    historical_queue_backlog_age = 0,
  } = chain;

  const getLiveQueueStatus = () => {
    if (live_queue_depth === 0) return 'idle';
    if (live_queue_depth < 50) return 'healthy';
    if (live_queue_depth < 100) return 'warning';
    return 'critical';
  };

  const getHistoricalQueueStatus = () => {
    if (historical_queue_depth === 0) return 'idle';
    if (historical_queue_depth < 1000) return 'healthy';
    if (historical_queue_depth < 10000) return 'warning';
    return 'active';
  };

  const liveStatus = getLiveQueueStatus();
  const historicalStatus = getHistoricalQueueStatus();

  return (
    <div className="dual-queue-health grid grid-cols-2 gap-4">
      {/* Live Queue */}
      <div className={`queue-card live-queue status-${liveStatus}`}>
        <div className="queue-header">
          <h4 className="text-sm font-medium">Live Queue</h4>
          <span className={`badge badge-${liveStatus}`}>
            {liveStatus.toUpperCase()}
          </span>
        </div>
        <div className="queue-metrics mt-2">
          <div className="metric">
            <span className="metric-label">Pending:</span>
            <span className="metric-value">{live_queue_depth.toLocaleString()}</span>
          </div>
          <div className="metric">
            <span className="metric-label">Age:</span>
            <span className="metric-value">{live_queue_backlog_age.toFixed(1)}s</span>
          </div>
        </div>
        {liveStatus === 'critical' && (
          <div className="alert alert-danger mt-2 text-xs">
            ⚠ Live queue backlog high - investigate worker health
          </div>
        )}
      </div>

      {/* Historical Queue */}
      <div className={`queue-card historical-queue status-${historicalStatus}`}>
        <div className="queue-header">
          <h4 className="text-sm font-medium">Historical Queue</h4>
          <span className={`badge badge-${historicalStatus}`}>
            {historicalStatus.toUpperCase()}
          </span>
        </div>
        <div className="queue-metrics mt-2">
          <div className="metric">
            <span className="metric-label">Pending:</span>
            <span className="metric-value">{historical_queue_depth.toLocaleString()}</span>
          </div>
          <div className="metric">
            <span className="metric-label">Age:</span>
            <span className="metric-value">{historical_queue_backlog_age.toFixed(1)}s</span>
          </div>
        </div>
      </div>
    </div>
  );
}
```

---

#### 6.6 Update Main Chain Dashboard/Detail Page
**File**: `web/admin/app/chains/[chainId]/page.tsx` (or main dashboard)

**Changes**: Integrate new components

```tsx
import { LiveSyncStatus } from '@/components/LiveSyncStatus';
import { GapRangesDisplay } from '@/components/GapRangesDisplay';
import { DualQueueHealth } from '@/components/DualQueueHealth';

export default function ChainDetailPage({ params }) {
  const { chain, isLoading } = useChain(params.chainId);

  if (isLoading) return <Loading />;

  return (
    <div className="chain-detail-page">
      <h1>{chain.chain_id}</h1>

      {/* SECTION 1: Live Sync Status (Task 0.3) */}
      <section className="sync-status-section">
        <h2>Sync Status</h2>
        <LiveSyncStatus chain={chain} />
      </section>

      {/* SECTION 2: Gap Information (Task 0.3) */}
      {chain.gap_ranges_count > 0 && (
        <section className="gap-info-section">
          <h2>Missing Blocks</h2>
          <GapRangesDisplay chain={chain} />
        </section>
      )}

      {/* SECTION 3: Queue Health (Dual-Queue Implementation) */}
      {chain.live_queue_depth !== undefined && (
        <section className="queue-health-section">
          <h2>Queue Health</h2>
          <DualQueueHealth chain={chain} />
        </section>
      )}

      {/* ... rest of existing page content ... */}
    </div>
  );
}
```

---

#### 6.7 Update Chain List/Table View
**File**: `web/admin/app/chains/page.tsx` (or chain list component)

**Changes**: Show live sync status in table

```tsx
export default function ChainsListPage() {
  const { chains, isLoading } = useChains();

  return (
    <table className="chains-table">
      <thead>
        <tr>
          <th>Chain ID</th>
          <th>Sync Status</th>
          <th>Progress</th>
          <th>Missing Blocks</th>
          <th>Queue Health</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        {chains.map(chain => (
          <tr key={chain.chain_id}>
            <td>{chain.chain_id}</td>

            {/* Live Sync Status */}
            <td>
              {chain.is_live_sync ? (
                <span className="badge badge-success">Live</span>
              ) : (
                <span className="badge badge-warning">Catching Up</span>
              )}
            </td>

            {/* Progress */}
            <td>
              <div className="text-sm">
                {chain.last_indexed.toLocaleString()} / {chain.head.toLocaleString()}
              </div>
              <div className="text-xs text-gray-500">
                {((chain.last_indexed / chain.head) * 100).toFixed(2)}%
              </div>
            </td>

            {/* Missing Blocks */}
            <td>
              {chain.missing_blocks_count > 0 ? (
                <span className="text-amber-600">
                  {chain.missing_blocks_count.toLocaleString()}
                </span>
              ) : (
                <span className="text-green-600">✓</span>
              )}
            </td>

            {/* Queue Health */}
            <td>
              {chain.live_queue_depth !== undefined ? (
                <div className="text-xs">
                  <div>L: {chain.live_queue_depth}</div>
                  <div>H: {chain.historical_queue_depth}</div>
                </div>
              ) : (
                <span className="text-gray-400">-</span>
              )}
            </td>

            <td>
              {/* ... actions ... */}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
```

---

#### 6.8 Add Styling
**File**: `web/admin/app/styles/components.css` (or component-specific CSS)

**Changes**: Add styles for new components

```css
/* Live Sync Status */
.live-sync-status {
  padding: 1rem;
  border: 1px solid #e5e7eb;
  border-radius: 0.5rem;
  background: #f9fafb;
}

.badge {
  display: inline-block;
  padding: 0.25rem 0.75rem;
  border-radius: 9999px;
  font-size: 0.75rem;
  font-weight: 600;
}

.badge-success {
  background: #d1fae5;
  color: #065f46;
}

.badge-warning {
  background: #fef3c7;
  color: #92400e;
}

.badge-danger {
  background: #fee2e2;
  color: #991b1b;
}

/* Progress Bar */
.progress-bar {
  width: 100%;
  height: 0.5rem;
  background: #e5e7eb;
  border-radius: 9999px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, #3b82f6, #8b5cf6);
  transition: width 0.3s ease;
}

/* Dual Queue Health */
.dual-queue-health {
  gap: 1rem;
}

.queue-card {
  padding: 1rem;
  border: 2px solid #e5e7eb;
  border-radius: 0.5rem;
  transition: all 0.2s;
}

.queue-card.status-healthy {
  border-color: #10b981;
  background: #ecfdf5;
}

.queue-card.status-warning {
  border-color: #f59e0b;
  background: #fffbeb;
}

.queue-card.status-critical {
  border-color: #ef4444;
  background: #fef2f2;
}

.queue-card.status-idle {
  border-color: #d1d5db;
  background: #f9fafb;
}

.queue-metrics .metric {
  display: flex;
  justify-content: space-between;
  margin-top: 0.5rem;
}

.metric-label {
  font-size: 0.875rem;
  color: #6b7280;
}

.metric-value {
  font-size: 0.875rem;
  font-weight: 600;
}
```

---

#### 6.9 Update API Fetching Logic
**File**: `web/admin/app/hooks/useChain.ts` (or API client)

**Changes**: Ensure new fields are fetched

```typescript
export function useChain(chainId: string) {
  return useSWR<Chain>(`/api/chains/${chainId}`, fetcher, {
    refreshInterval: 5000, // Refresh every 5 seconds for real-time updates
  });
}

export function useChains() {
  return useSWR<Chain[]>('/api/chains', fetcher, {
    refreshInterval: 10000, // Refresh every 10 seconds
  });
}
```

---

#### 6.10 Implementation Checklist - UI Updates

**Phase 1: Task 0.3 UI (Immediate)**
- [ ] Update TypeScript types with Task 0.3 fields
- [ ] Create LiveSyncStatus component
- [ ] Create GapRangesDisplay component
- [ ] Update chain detail page to use new components
- [ ] Update chain list table to show live sync status
- [ ] Add styling for new components
- [ ] Test with chains that have gaps vs no gaps
- [ ] Test with live-synced chains vs catching-up chains

**Phase 2: Dual-Queue UI (After Backend Implementation)**
- [ ] Add dual-queue fields to TypeScript types
- [ ] Create DualQueueHealth component
- [ ] Integrate DualQueueHealth into chain detail page
- [ ] Update chain list table to show queue metrics
- [ ] Add queue health styling
- [ ] Test with different queue states (idle, healthy, warning, critical)
- [ ] Verify real-time updates work correctly

---

### Phase 7: Testing Updates

#### 7.1 Unit Tests - Workflow Routing
**File**: `tests/unit/indexer/headscan_test.go` (NEW or update existing)

**Tests to Add:**
```go
func TestIsLiveBlock(t *testing.T) {
    tests := []struct {
        name     string
        latest   uint64
        height   uint64
        expected bool
    }{
        {"exact head", 1000, 1000, true},
        {"within threshold", 1000, 801, true},
        {"at boundary", 1000, 800, true},
        {"just outside threshold", 1000, 799, false},
        {"far historical", 1000, 100, false},
        {"future block", 1000, 1001, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := workflow.IsLiveBlock(tt.latest, tt.height)
            assert.Equal(t, tt.expected, result)
        })
    }
}

func TestHeadScanLiveQueueRouting(t *testing.T) {
    // Test that HeadScan routes blocks within 200 to live queue
    // Mock GetLatestHead to return 1000
    // Schedule blocks 801-1000
    // Verify all went to GetIndexerLiveQueue()
}

func TestSchedulerHistoricalQueueRouting(t *testing.T) {
    // Test that SchedulerWorkflow routes old blocks to historical queue
    // Mock GetLatestHead to return 10000
    // Schedule blocks 1-1000
    // Verify all went to GetIndexerHistoricalQueue()
}
```

#### 7.2 Integration Tests - Queue Isolation
**File**: `tests/integration/indexer/dual_queue_test.go` (NEW)

**Tests to Add:**
```go
func TestLiveQueueNotBlockedByHistorical(t *testing.T) {
    // 1. Start historical backfill (blocks 1-50000)
    // 2. Wait for historical queue to fill up
    // 3. Trigger live block indexing (blocks 50001-50010)
    // 4. Verify live blocks complete within 30 seconds
    // 5. Verify historical blocks still processing
}

func TestQueueBoundarySplitting(t *testing.T) {
    // Schedule range that crosses live/historical boundary (e.g., 700-900 when latest=1000)
    // Verify blocks 801-900 go to live queue
    // Verify blocks 700-800 go to historical queue
}
```

#### 7.3 Update Existing Tests
**Files**: All existing indexer tests

**Changes Needed**:
- Mock `GetIndexerLiveQueue()` and `GetIndexerHistoricalQueue()`
- Update assertions to check correct queue routing
- Update worker initialization in test fixtures

**Example**:
```go
// Before
mockClient.EXPECT().GetIndexerQueue("test_chain").Return("index:test_chain")

// After
mockClient.EXPECT().GetIndexerLiveQueue("test_chain").Return("index:test_chain:live")
mockClient.EXPECT().GetIndexerHistoricalQueue("test_chain").Return("index:test_chain:historical")
```

---

## Migration Strategy

### Zero-Downtime Migration Plan

#### Step 1: Deploy Code with Dual Workers
1. Deploy indexer with 3 workers (ops, live, historical)
2. Workers start polling both new queues
3. Old workflows on `index:canopy_local` continue processing (backwards compat)

#### Step 2: Gradual Queue Migration
1. New workflows route to live/historical queues
2. Old workflows drain from legacy queue
3. Monitor both queues during transition

#### Step 3: Cleanup (After 24 hours)
1. Verify legacy queue is empty
2. Remove `GetIndexerQueue()` function
3. Remove legacy queue monitoring

---

## Monitoring & Observability

### New Metrics to Track

#### Grafana Dashboard Updates
**Panel 1: Queue Depth Over Time**
```promql
# Live queue depth
temporal_task_queue_depth{task_queue="index:canopy_local:live"}

# Historical queue depth
temporal_task_queue_depth{task_queue="index:canopy_local:historical"}
```

**Panel 2: Live Block Latency**
```promql
# Time from block production to indexing completion (live blocks only)
histogram_quantile(0.95,
    rate(indexer_block_latency_seconds{queue="live"}[5m])
)
```

**Panel 3: Worker Distribution**
```promql
# Pollers per queue
temporal_task_queue_pollers{task_queue=~"index:.*"}
```

### Alerting Rules

**Critical: Live Queue Backlog**
```yaml
- alert: LiveQueueBacklog
  expr: temporal_task_queue_depth{task_queue="index:canopy_local:live"} > 100
  for: 5m
  annotations:
    summary: "Live queue has {{ $value }} pending workflows"
    description: "Live blocks should process quickly - investigate worker health"
```

**Warning: Historical Queue Stalled**
```yaml
- alert: HistoricalQueueStalled
  expr: rate(temporal_workflow_completed_total{task_queue="index:canopy_local:historical"}[10m]) == 0
  for: 15m
  annotations:
    summary: "Historical queue has not completed workflows in 15 minutes"
```

---

## Rollback Plan

### If Things Go Wrong

#### Immediate Rollback (< 1 hour)
1. Revert indexer deployment to previous version
2. Old version uses single `index:canopy_local` queue
3. New workflows on live/historical queues will fail (workers not listening)
4. Cancel those workflows and re-trigger on legacy queue

#### Graceful Rollback (> 1 hour)
1. Deploy "bridge" version that polls all 3 queues
2. Migrate live/historical workflows back to legacy queue
3. Drain live/historical queues
4. Deploy full rollback

---

## Performance Expectations

### Before (Single Queue)
- Live block latency: 25+ minutes (unpredictable)
- Head gap: 50-80 blocks (unstable)
- Historical throughput: ~3,500 wf/sec

### After (Dual Queue)
- Live block latency: <30 seconds (predictable)
- Head gap: 0-5 blocks (stable)
- Historical throughput: ~3,500 wf/sec (unchanged)

### Resource Impact
- **CPU**: +10% (3 workers instead of 2)
- **Memory**: +15% (additional worker goroutines)
- **Network**: Unchanged (same workflow volume)

---

## Implementation Checklist

### Backend (Go)
- [ ] Update `pkg/temporal/client.go` - Add live/historical queue functions
- [ ] Update `pkg/indexer/workflow/ops.go` - Add IsLiveBlock() function
- [ ] Update `pkg/indexer/activity/ops.go` - Update StartIndexWorkflow routing
- [ ] Update `pkg/indexer/activity/ops.go` - Update StartIndexWorkflowBatch routing
- [ ] Update `app/indexer/app.go` - Create 3 workers (live, historical, ops)
- [ ] Update `app/controller/app.go` - Monitor both queues
- [ ] Update `app/controller/app.go` - Update health check messages
- [ ] Update `app/admin/controller/chain.go` - Add queue fields to API response

### Frontend (Next.js)
- [ ] Update chain status component - Display both queue depths
- [ ] Update API types - Add live/historical queue fields
- [ ] Update dashboard - Add queue health indicators

### Testing
- [ ] Unit tests - TestIsLiveBlock()
- [ ] Unit tests - TestHeadScanLiveQueueRouting()
- [ ] Unit tests - TestSchedulerHistoricalQueueRouting()
- [ ] Integration tests - TestLiveQueueNotBlockedByHistorical()
- [ ] Integration tests - TestQueueBoundarySplitting()
- [ ] Update all existing tests - Mock new queue functions

### Documentation
- [ ] Update architecture docs
- [ ] Update deployment guide
- [ ] Update troubleshooting guide
- [ ] Add queue routing decision flowchart

### Deployment
- [ ] Build and tag new indexer image
- [ ] Deploy to staging environment
- [ ] Run integration tests
- [ ] Deploy to production
- [ ] Monitor for 24 hours
- [ ] Cleanup legacy queue code

---

## Open Questions / Decisions Needed

1. **Live threshold of 200 blocks - confirm?**
   - Alternative: Make it configurable via env var `LIVE_BLOCK_THRESHOLD`

2. **Worker resource allocation - confirm split?**
   - Current proposal: Same limits for both workers
   - Alternative: 30% resources for live, 70% for historical

3. **Priority still needed within queues?**
   - Proposal: Keep priority for ordering within queue
   - Alternative: Remove priority entirely (queue separation is enough)

4. **Should GapScan route to historical queue by default?**
   - Proposal: Yes, gaps are by definition historical
   - Alternative: Check each gap range individually

5. **Backwards compatibility period?**
   - Proposal: Keep legacy queue monitoring for 24 hours
   - Alternative: Remove immediately

---

## Estimated Timeline

### Development
- Phase 1-3 (Core changes): **2-3 days**
- Phase 4-6 (Controller, API, UI): **1-2 days**
- Phase 7 (Testing): **2-3 days**
- **Total: 5-8 days**

### Testing & Deployment
- Staging deployment: **1 day**
- Integration testing: **1 day**
- Production deployment: **1 day**
- Monitoring period: **2-3 days**
- **Total: 5-6 days**

### Overall: **10-14 days** from start to production-stable

---

## Success Criteria

### Must Have (Go/No-Go)
- ✅ Live queue processes blocks within 30 seconds
- ✅ Historical queue does not block live queue
- ✅ All existing tests pass
- ✅ Zero errors during 24-hour monitoring period

### Should Have
- ✅ Head gap stays under 10 blocks
- ✅ Historical throughput maintained at 3,500+ wf/sec
- ✅ Grafana dashboard shows both queues

### Nice to Have
- ✅ Live block latency under 10 seconds
- ✅ Automated queue health alerting
- ✅ Historical queue auto-scaling based on backlog

---

**End of Implementation Plan**

**Next Steps**: Review this plan, confirm decisions on open questions, then begin Phase 1 implementation.
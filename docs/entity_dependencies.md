c# Entity Dependencies Analysis

This document analyzes the indexing activities and identifies which entities depend on others versus which only depend on height-1 RPC data.

## Entities Dependent **ONLY on Height-1** (No Database Entity Dependencies)

These activities only query RPC at height H and H-1, without querying other indexed entities from the database:

### 1. Block (`app/indexer/activity/block.go:33`)
- Fetches block from RPC using height-aware endpoint selection
- Saves to staging table
- No dependencies on other entities

### 2. Transactions (`app/indexer/activity/transactions.go:18`)
- Fetches transactions from RPC(H)
- Converts to indexer models
- No dependencies on other entities

### 3. Events (`app/indexer/activity/events.go:18`)
- Fetches events from RPC(H)
- Converts to indexer models
- No dependencies on other entities
- **Critical**: Many other entities depend on this being indexed first

### 4. Params (`app/indexer/activity/params.go:22`)
- Compares RPC(H) vs RPC(H-1)
- Snapshot-on-change pattern
- No dependencies on other entities

### 5. Supply (`app/indexer/activity/supply.go:27`)
- Compares RPC(H) vs RPC(H-1)
- Snapshot-on-change pattern
- No dependencies on other entities

### 6. Committees (`app/indexer/activity/committees.go:21`)
- Compares RPC(H) vs RPC(H-1)
- Snapshot-on-change pattern
- Also fetches subsidized and retired committees
- No dependencies on other entities

### 7. DexPrices (`app/indexer/activity/dexprices.go:20`)
- Compares RPC(H) vs RPC(H-1)
- Calculates price deltas, local pool deltas, remote pool deltas
- No dependencies on other entities

## Entities Dependent on **Events** (Must Run After Events)

These activities query events from the staging table using `GetEventsByTypeAndHeight`, so they depend on Events being indexed first:

### 1. Accounts (`app/indexer/activity/accounts.go:36`)
- **RPC Pattern**: Parallel fetch of RPC(H) and RPC(H-1)
- **Event Dependencies**:
  - Queries `EventReward`, `EventSlash` from staging (lines 115-121)
  - Uses events for balance change correlation
- **Logic**: Compares current vs previous balances, creates snapshots on change
- **Why Events?**: To correlate account balance changes with reward/slash events

### 2. Validators (`app/indexer/activity/validators.go:31`)
- **RPC Pattern**: Parallel fetch of RPC(H) and RPC(H-1) for validators, non-signers, and double-signers (6 workers)
- **Event Dependencies**:
  - Queries `EventReward`, `EventSlash`, `EventAutoPause`, `EventAutoBeginUnstaking`, `EventAutoFinishUnstaking` from staging (lines 185-195)
  - Uses events for validator lifecycle state transitions
- **Logic**: Compares validator state, creates snapshots on change, correlates with lifecycle events
- **Why Events?**: To identify when validators change state (pause, unstaking, slashing)

### 3. Orders (`app/indexer/activity/orders.go:38`)
- **RPC Pattern**: Parallel fetch of RPC(H) and RPC(H-1)
- **Event Dependencies**:
  - Queries `EventOrderBookSwap` from staging (lines 111-118)
  - Uses events to determine order completion status
- **Logic**: Compares order state, detects filled/cancelled orders
- **Why Events?**: To identify when orders are filled via swap events

### 4. Pools (`app/indexer/activity/pools.go:22`)
- **RPC Pattern**: Parallel fetch of RPC(H) and RPC(H-1), plus pool point holders
- **Event Dependencies**:
  - Queries `EventDexLiquidityDeposit`, `EventDexLiquidityWithdraw`, `EventDexSwap` from staging (lines 81-86)
  - Uses events for pool state change correlation
- **Logic**: Calculates H-1 deltas, tracks pool holders with snapshot-on-change
- **Why Events?**: To correlate pool balance changes with deposit/withdraw/swap events

### 5. DexBatch (`app/indexer/activity/dex_batch.go:47`)
- **RPC Pattern**: Parallel fetch of DexBatch(H), NextDexBatch(H), and **DexBatch(H-1)**
- **Event Dependencies**:
  - Queries `EventDexSwap`, `EventDexLiquidityDeposit`, `EventDexLiquidityWithdraw` from staging (lines 127-135)
  - Uses events-first architecture
- **Logic**:
  - DexBatch(H) orders → state="locked"
  - NextDexBatch(H) orders → state="pending"
  - DexBatch(H-1) + Events(H) → state="complete"
- **Why Events?**: Events at height H describe entities from height H-1, requires correlation
- **Special Note**: Most complex dependency - needs both Events and RPC(H-1) data

## Must Run **Last**

### BlockSummary (`app/indexer/activity/block_summary.go:14`)
- Aggregates entity counts from ALL entity indexing activities
- Must run after all entities have been indexed
- Saves summary to staging table
- Used by the `RecordIndexed` activity to publish block.indexed events

## **Independent** (Special Case)

### Poll (`app/indexer/activity/poll.go:23`)
- **Not height-based**: Uses time-based snapshots (scheduled every 20 seconds)
- Fetches current poll state from RPC (no height parameter support)
- Inserts directly to production table (no staging)
- ReplacingMergeTree deduplicates by (proposal_hash, snapshot_time)
- No dependencies on any entities

## Dependency Graph

```
┌─────────────────────────────────────────────────────┐
│ Height-1 Only (Can Run in Parallel)                │
├─────────────────────────────────────────────────────┤
│  ├─ Block                                           │
│  ├─ Transactions                                    │
│  ├─ Events  ◄───────────────────┐                  │
│  ├─ Params                       │                  │
│  ├─ Supply                       │                  │
│  ├─ Committees                   │                  │
│  └─ DexPrices                    │                  │
└─────────────────────────────────────────────────────┘
                                   │
                                   │ (depends on)
                                   │
┌─────────────────────────────────────────────────────┐
│ Events-Dependent (Must Run After Events)           │
├─────────────────────────────────────────────────────┤
│  ├─ Accounts ──────────────────┘                   │
│  ├─ Validators ─────────────────┘                  │
│  ├─ Orders ─────────────────────┘                  │
│  ├─ Pools ──────────────────────┘                  │
│  └─ DexBatch ───────────────────┘ (also RPC(H-1))  │
└─────────────────────────────────────────────────────┘
                                   │
                                   │ (depends on ALL)
                                   │
┌─────────────────────────────────────────────────────┐
│ Final Stage                                         │
├─────────────────────────────────────────────────────┤
│  └─ BlockSummary                                    │
└─────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────┐
│ Independent (Time-Based)                            │
├─────────────────────────────────────────────────────┤
│  └─ Poll (every 20 seconds, not height-based)      │
└─────────────────────────────────────────────────────┘
```

## Execution Order

### Phase 1: Independent RPC Fetching (Parallel)
All of these can run concurrently:
- Block
- Transactions
- Events
- Params
- Supply
- Committees
- DexPrices

### Phase 2: Event-Dependent Processing (Parallel)
After Events completes, all of these can run concurrently:
- Accounts
- Validators
- Orders
- Pools
- DexBatch

### Phase 3: Aggregation (Sequential)
After all Phase 2 activities complete:
- BlockSummary

## Key Insights

1. **Events is Critical**: The Events activity is a critical dependency for most complex entity types (Accounts, Validators, Orders, Pools, DexBatch) because they use the "events-first" architecture to correlate state changes with events.

2. **Snapshot-on-Change Pattern**: Most activities use the RPC(H) vs RPC(H-1) comparison pattern to detect changes and only insert snapshots when data changes. This provides significant storage savings.

3. **Parallel Optimization**: Within each phase, activities can run in parallel for maximum throughput:
   - Phase 1: 7 activities in parallel
   - Phase 2: 5 activities in parallel

4. **DexBatch Complexity**: The DexBatch activity has the most complex dependency pattern:
   - Queries Events(H) from staging
   - Queries RPC(H-1) to match events with entities
   - Uses events-first architecture where events at H describe entities from H-1

5. **No Cross-Entity DB Dependencies**: Importantly, no activity queries other indexed entities from the database (besides Events). All state comparison is done via RPC(H) vs RPC(H-1), which keeps the indexing pipeline simple and parallelizable.

6. **Poll Independence**: The Poll activity is completely independent and runs on a time-based schedule rather than per-height, making it suitable for governance data that doesn't align with block heights.
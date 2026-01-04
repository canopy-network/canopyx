# Follow best practices for ClickHouse query optimization

https://clickhouse.com/resources/engineering/clickhouse-query-optimisation-definitive-guide

# ClickHouse Engine Decision: ReplacingMergeTree

## Decision

**Use `ReplacingMergeTree` for ALL tables (production and staging).**

---

## Why ReplacingMergeTree?

### The Problem
ClickHouse has NO unique constraints or transactions. Without protection, duplicate rows can occur from:
- **Workflow retries** - Temporal retries failed activities
- **Human error** - Manual workflow retrigger without reindex flag
- **Race conditions** - Concurrent operations

### The Solution
ReplacingMergeTree automatically deduplicates rows with identical `ORDER BY` keys during background merges and `FINAL` queries.

### Why Not MergeTree?
While our workflow logic (`index_progress` watermark) should prevent duplicates, it relies on application-level enforcement:
- ✅ Works 99.9% of the time
- ❌ Cannot prevent manual errors or edge cases
- ❌ Duplicates require manual cleanup

**Philosophy:** Defensive programming - better safe than sorry.

---

## Key Characteristics

### For Immutable Entities (blocks, transactions, etc)
```sql
ENGINE = ReplacingMergeTree()
ORDER BY height
```
- Deduplicates by `ORDER BY` key
- Keeps latest inserted row (by internal block number)
- Different heights = different rows (NOT duplicates)

### For Versioned Entities (accounts, validators, etc)
```sql
ENGINE = ReplacingMergeTree(height)
ORDER BY (address, height)
```
- `height` is both data field AND version column
- Stores multiple versions (different heights) per address
- Deduplicates retries of same height
- Keeps highest version if multiple rows have same `ORDER BY` key

**Critical:** Historical versions at different heights are NEVER merged - only true duplicates (same `ORDER BY` key).

---

## Performance Trade-off

### Cost
`FINAL` queries are **2× slower** than regular queries (with optimizations).

### Benefit
Automatic deduplication - no manual cleanup, no corrupted data.

### Mitigation
1. Setting: `do_not_merge_across_partitions_select_final = 1` (reduces 6× → 2×)
2. Partitioning: Monthly partitions (`PARTITION BY toYYYYMM(time)`)
3. Maintenance: Monthly `OPTIMIZE TABLE` via Temporal scheduled workflow
4. **Compression:** ZSTD level 3 for 3-5x compression ratio
5. **Storage Tiering:** Hot (NVMe, 30d) → Warm (SSD, 180d) → Cold (HDD, permanent)

**Verdict:** 2× overhead is acceptable for data integrity guarantees.

---

## Storage Optimization

### Compression Strategy

ClickHouse supports codec-level compression for even better space savings:

```sql
CREATE TABLE accounts (
    address String CODEC(ZSTD(1)),              -- ZSTD level 1 for strings
    amount UInt64 CODEC(Delta, ZSTD(3)),        -- Delta + ZSTD for numeric values
    height UInt64 CODEC(DoubleDelta, LZ4),      -- DoubleDelta for sequential heights
    height_time DateTime64(6) CODEC(DoubleDelta, LZ4)  -- DoubleDelta for timestamps
) ENGINE = ReplacingMergeTree(height)
ORDER BY (address, height)
SETTINGS storage_policy = 'tiered_storage';
```

**Codec Guide:**
- **ZSTD(1-9):** Better compression than default LZ4
  - Level 1: Fast compression (~2-3x)
  - Level 3: Balanced (3-5x compression, recommended)
  - Level 9: Max compression (5-10x, slower writes)
- **Delta:** For numeric sequences that change gradually (balances, amounts)
- **DoubleDelta:** For sequential/monotonic data (heights, timestamps)

**Trade-off:** ~10-20% more CPU during writes for 2-3x better compression.

**Recommendation:** Use ZSTD(3) + Delta/DoubleDelta for blockchain data - excellent compression with minimal CPU cost.

### Storage Tiering (Hot/Warm/Cold)

Blockchain data has clear access patterns - recent data is queried frequently, old data rarely. ClickHouse supports automatic tiering based on TTL:

**Configuration:** `deploy/k8s/clickhouse/storage-policy.xml`

```xml
<storage_configuration>
    <disks>
        <hot><path>/var/lib/clickhouse/hot/</path></hot>      <!-- NVMe -->
        <warm><path>/var/lib/clickhouse/warm/</path></warm>   <!-- SSD -->
        <cold><path>/var/lib/clickhouse/cold/</path></cold>   <!-- HDD -->
    </disks>

    <policies>
        <tiered_storage>
            <volumes>
                <hot><disk>hot</disk></hot>       <!-- Last 30 days -->
                <warm><disk>warm</disk></warm>    <!-- 30-180 days -->
                <cold><disk>cold</disk></cold>    <!-- 180+ days -->
            </volumes>
        </tiered_storage>
    </policies>
</storage_configuration>
```

**Table Definition:**
```sql
CREATE TABLE accounts (
    ...
) ENGINE = ReplacingMergeTree(height)
ORDER BY (address, height)
SETTINGS storage_policy = 'tiered_storage'
TTL
    height_time + INTERVAL 30 DAY TO VOLUME 'hot',   -- NVMe for 30 days
    height_time + INTERVAL 180 DAY TO VOLUME 'warm', -- SSD for 180 days
    height_time + INTERVAL 365 DAY TO VOLUME 'cold'; -- HDD forever
```

**Cost Savings:**
- **NVMe:** $1000/TB - only 30 days of data (~210GB for 200k accounts/block)
- **SSD:** $150/TB - 150 days of historical data (~1TB)
- **HDD:** $20/TB - everything older (10TB+)

**Result:** 95% of data on cheap HDD, 5% on fast NVMe. **10x cost reduction** with zero performance impact on recent queries.

**Deployment:** Automatically configured via Tilt - see `Tiltfile:clickhouse-storage-config`.

---

## Indexing Strategy for Versioned Entities

### The Problem

For versioned entities (accounts, validators, pools), we have two indexing approaches:

**Option A: Index ALL entities every block**
- Fetch all 200k accounts from RPC
- Insert all 200k rows to ClickHouse
- Simple, fast (~700ms/block)

**Option B: Index ONLY changed entities**
- Fetch all 200k accounts from RPC
- Query previous state from ClickHouse
- Compare in-memory, insert only changed (~500 changed)
- Complex, slower (~1000-1500ms/block)

### Storage Comparison

| Metric              | Option A (All) | Option B (Changed)                      | Savings           |
|---------------------|----------------|-----------------------------------------|-------------------|
| **Storage/year**    | 5.8 TB         | 290 GB                                  | **20x less**      |
| **Rows/year**       | 73 billion     | 3.65 billion                            | **20x less**      |
| **Cost/year/chain** | $225           | $11.25                                  | **$213.75 saved** |
| **Indexing speed**  | 700ms/block    | 1000-1500ms                             | **30-50% slower** |
| **Code complexity** | Simple         | Complex (chunking, workers, comparison) | Much higher       |

### Decision: Option B (Changed Only) ✅

**Why:**
- 10 chains = **$2,137.50/year savings**
- Storage can spiral out of control quickly
- Slower indexing is an acceptable trade-off

---

## Numeric Type Strategy

### The Problem

Blockchain amounts can be large. Which numeric type should we use?

**Available types:**
- `UInt64`: Max 18.4 quintillion (what blockchain uses)
- `UInt128`: Max 340 undecillion (overkill for individuals)
- `UInt256`: Max stupidly large (Ethereum wei compatible)

### Analysis

**Individual values:**
- Blockchain enforces uint64 limits
- Individual balances **CANNOT** overflow uint64 (blockchain validation)
- Billions of rows per year
- Query performance critical

**Aggregate values:**
- Sum of 1M accounts × 10M CNPY each = 10^19 uCNPY
- **CAN** exceed uint64 max (1.8 × 10^19)
- Only thousands of rows per year
- No complex queries (just INSERT and SELECT)

### Decision: Hybrid Approach ✅

**Use UInt64 for individual entity tables (high volume, queried):**
```go
type Account struct {
    Amount uint64  `ch:"amount,codec:Delta,ZSTD(3)"`  // Individual balance
}

type Transaction struct {
    Amount uint64  `ch:"amount"`  // Individual tx amount
    Fee    uint64  `ch:"fee"`
}

type Validator struct {
    StakedAmount uint64  `ch:"staked_amount"`  // Individual stake
}
```

**Use UInt256 for aggregate/summary tables (low volume, computed):**
```go
type Supply struct {
    Total      *big.Int `ch:"total,type:UInt256"`          // SUM can overflow
    Staked     *big.Int `ch:"staked,type:UInt256"`         // SUM can overflow
    Delegated  *big.Int `ch:"delegated_only,type:UInt256"` // SUM can overflow
}

type ChainMetrics struct {
    CumulativeFees  *big.Int `ch:"cumulative_fees,type:UInt256"`  // SUM over time
    CumulativeBurned *big.Int `ch:"cumulative_burned,type:UInt256"` // SUM over time
}
```

**Why this works:**
- ✅ Individual values match blockchain's uint64 type
- ✅ Billions of rows use efficient 8-byte UInt64
- ✅ Aggregations protected from overflow with UInt256
- ✅ Aggregate tables only ~365k rows/year (11 MB storage)
- ✅ No overflow risk on either side
- ✅ Optimal storage and query performance

**Storage impact:** Negligible (~11 MB/year for all aggregates)

---

## DevOps Requirements

### 1. ClickHouse Configuration

#### User Profile Settings

**File:** `/etc/clickhouse-server/users.d/performance.xml`

```xml
<clickhouse>
    <profiles>
        <default>
            <!-- Critical: Reduces FINAL overhead from 6× to 2× -->
            <do_not_merge_across_partitions_select_final>1</do_not_merge_across_partitions_select_final>

            <!-- Memory limits for FINAL queries -->
            <max_memory_usage>10000000000</max_memory_usage>  <!-- 10GB -->

            <!-- Use more threads for FINAL queries -->
            <max_threads>8</max_threads>

            <!-- Optimize aggregations -->
            <optimize_aggregation_in_order>1</optimize_aggregation_in_order>
        </default>
    </profiles>
</clickhouse>
```

#### Background Merge Settings

**File:** `/etc/clickhouse-server/config.xml`

```xml
<clickhouse>
    <merge_tree>
        <!-- Merge smaller parts more aggressively -->
        <max_bytes_to_merge_at_min_space_in_pool>1073741824</max_bytes_to_merge_at_min_space_in_pool>

        <!-- Allow more concurrent merges -->
        <max_replicated_merges_in_queue>16</max_replicated_merges_in_queue>

        <!-- Merge parts faster -->
        <number_of_free_entries_in_pool_to_lower_max_size_of_merge>8</number_of_free_entries_in_pool_to_lower_max_size_of_merge>
    </merge_tree>
</clickhouse>
```

**Why these settings:**
- Faster background merges → fewer unmerged parts → better FINAL performance
- More concurrent merges → old partitions merge quicker
- Aggressive merging → reduces part count

### 2. Table Schema Requirements

All tables MUST use:
- `ENGINE = ReplacingMergeTree()`  (immutable) or `ReplacingMergeTree(height)` (versioned)
- `PARTITION BY toYYYYMM(time_column)`  (monthly partitioning)
- `ORDER BY` appropriate to entity type

**Example:**
```sql
CREATE TABLE blocks (
    height UInt64,
    hash String,
    time DateTime64(6),
    ...
) ENGINE = ReplacingMergeTree()
ORDER BY height
PARTITION BY toYYYYMM(time);
```

### 3. Temporal Scheduled Workflow (Automated Maintenance)

**Each indexer schedules its own maintenance on the OPS queue.**

**Workflow:** `OptimizePartitionsWorkflow`
- **Schedule:** Monthly (1st day of month at 2 AM)
- **Queue:** `ops:{chainId}` (separate from indexing)
- **Action:** Optimizes previous month's partitions

**Implementation:**

```go
// pkg/indexer/workflow/optimize_partitions.go

// OptimizePartitionsWorkflow optimizes old ClickHouse partitions.
// Runs monthly to merge old data into single parts, making FINAL queries instant.
func (wc *Context) OptimizePartitionsWorkflow(ctx workflow.Context, in types.OptimizePartitionsInput) error {
    ao := workflow.ActivityOptions{
        StartToCloseTimeout: 30 * time.Minute,
        TaskQueue:           fmt.Sprintf("ops:%s", in.ChainID),
    }
    ctx = workflow.WithActivityOptions(ctx, ao)

    // Get last month partition (e.g., "202401")
    lastMonth := time.Now().AddDate(0, -1, 0).Format("200601")

    // Optimize each table
    tables := []string{"blocks", "txs", "accounts", "validators", "pools"}

    for _, table := range tables {
        optimizeInput := types.OptimizeTableInput{
            ChainID:   in.ChainID,
            Table:     table,
            Partition: lastMonth,
        }

        var out types.OptimizeTableOutput
        if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.OptimizeTable, optimizeInput).Get(ctx, &out); err != nil {
            // Log but don't fail entire workflow
            workflow.GetLogger(ctx).Warn("Failed to optimize table", "table", table, "error", err)
            continue
        }

        workflow.GetLogger(ctx).Info("Optimized table",
            "table", table,
            "partition", lastMonth,
            "duration_ms", out.DurationMs)
    }

    return nil
}
```

**Activity:**

```go
// pkg/indexer/activity/optimize.go

func (c *Context) OptimizeTable(ctx context.Context, in types.OptimizeTableInput) (types.OptimizeTableOutput, error) {
    start := time.Now()

    chainDb, err := c.NewChainDb(ctx, in.ChainID)
    if err != nil {
        return types.OptimizeTableOutput{}, err
    }

    c.Logger.Info("Optimizing table partition",
        zap.String("table", in.Table),
        zap.String("partition", in.Partition))

    // Execute OPTIMIZE TABLE
    query := fmt.Sprintf("OPTIMIZE TABLE %s PARTITION '%s' FINAL", in.Table, in.Partition)
    if _, err := chainDb.ClickHouse().ExecContext(ctx, query); err != nil {
        return types.OptimizeTableOutput{}, err
    }

    durationMs := float64(time.Since(start).Milliseconds())
    return types.OptimizeTableOutput{
        Table:      in.Table,
        Partition:  in.Partition,
        DurationMs: durationMs,
    }, nil
}
```

**Schedule registration (per chain):**

```go
// pkg/indexer/scheduler.go

func (s *Scheduler) RegisterMaintenanceSchedules(chainID string) error {
    // Schedule monthly partition optimization
    // Runs on 1st day of month at 2 AM
    scheduleID := fmt.Sprintf("optimize-partitions-%s", chainID)

    err := s.temporalClient.ScheduleClient().Create(ctx, client.ScheduleOptions{
        ID:   scheduleID,
        Spec: client.ScheduleSpec{
            CronExpressions: []string{"0 2 1 * *"},  // 2 AM on 1st of month
        },
        Action: &client.ScheduleWorkflowAction{
            ID:        fmt.Sprintf("optimize-partitions-%s-{timestamp}", chainID),
            Workflow:  wc.OptimizePartitionsWorkflow,
            TaskQueue: fmt.Sprintf("ops:%s", chainID),
            Args: []interface{}{
                types.OptimizePartitionsInput{ChainID: chainID},
            },
        },
    })

    return err
}
```

**Why Temporal vs Cron/Bash:**
- ✅ Visible in Temporal UI (history, logs, metrics)
- ✅ Automatic retries on failure
- ✅ Per-chain isolation (each indexer manages own)
- ✅ Version controlled in codebase
- ✅ No external dependencies (cron, bash scripts)
- ✅ Can pause/resume schedules dynamically
- ✅ Centralized monitoring

### 4. Monitoring (admin prometheus metrics in future)

**Check partition count:**
```sql
SELECT
    table,
    COUNT(DISTINCT partition) as partition_count
FROM system.parts
WHERE active = 1
GROUP BY table;
```

**Guideline:** Keep under 100 partitions per table.

**Check unmerged parts (alert if high):**
```sql
SELECT
    partition,
    COUNT(*) as part_count
FROM system.parts
WHERE table = 'blocks' AND active = 1
GROUP BY partition
HAVING part_count > 5;
```

**Temporal UI:** Monitor `OptimizePartitionsWorkflow` executions monthly.

---

## Developer Requirements

### 1. ALWAYS Use FINAL in Queries

**Rule:** Every `SELECT` query MUST include `FINAL` modifier.

**Why:** Without `FINAL`, queries may return duplicate rows.

#### ❌ WRONG
```go
func GetBlock(ctx, height) (*Block, error) {
    var block Block
    err := db.NewSelect().
        Model(&block).
        Where("height = ?", height).
        Scan(ctx)  // ❌ No FINAL - may return duplicates!
    return &block, err
}
```

#### ✅ CORRECT
```go
func GetBlock(ctx, height) (*Block, error) {
    var block Block
    err := db.NewSelect().
        Model(&block).
        Final().  // ✅ Required
        Where("height = ?", height).
        Scan(ctx)
    return &block, err
}
```

### 2. Use Query Helper (Recommended)

Create a helper to enforce `FINAL` usage and apply performance settings:

```go
// pkg/db/query_helpers.go

// QueryWithFinal executes a query with FINAL and optimization settings.
func (db *chainDB) QueryWithFinal(ctx context.Context) *ch.SelectQuery {
    return db.clickhouse.NewSelect().
        Final().  // Apply FINAL for deduplication
        Apply(func(q *ch.SelectQuery) *ch.SelectQuery {
            // Add performance settings
            q = q.Setting("do_not_merge_across_partitions_select_final", 1)
            q = q.Setting("optimize_aggregation_in_order", 1)
            return q
        })
}

// Usage
func GetBlock(ctx context.Context, db *chainDB, height uint64) (*Block, error) {
    var block Block

    err := db.QueryWithFinal(ctx).
        Model(&block).
        Where("height = ?", height).
        Scan(ctx)

    return &block, err
}
```

**Why Apply():**
- Applies settings to every query automatically
- No need to remember to add settings manually
- Consistent performance across all queries

### 3. Integration Tests

Every query endpoint MUST have integration tests verifying:

```go
func TestGetBlock_Deduplication(t *testing.T) {
    // Insert duplicate
    db.Insert(&Block{Height: 1000, Hash: "abc"})
    db.Insert(&Block{Height: 1000, Hash: "abc"})  // Duplicate

    // Query
    block, err := GetBlock(ctx, db, 1000)

    // Must return single row
    assert.NoError(err)
    assert.NotNil(block)
}
```

### 4. Common Query Patterns

#### Get Latest Version (Versioned Entities)
```go
func GetAccountLatest(ctx context.Context, db *chainDB, address string) (*Account, error) {
    var account Account

    err := db.QueryWithFinal(ctx).
        Model(&account).
        Where("address = ?", address).
        OrderExpr("height DESC").
        Limit(1).
        Scan(ctx)

    return &account, err
}
```

#### Get Historical Version
```go
func GetAccountAtHeight(ctx context.Context, db *chainDB, address string, height uint64) (*Account, error) {
    var account Account

    err := db.QueryWithFinal(ctx).
        Model(&account).
        Where("address = ? AND height <= ?", address, height).
        OrderExpr("height DESC").
        Limit(1).
        Scan(ctx)

    return &account, err
}
```

#### List Query (Paginated)
```go
func ListBlocks(ctx context.Context, db *chainDB, page, perPage int) ([]*Block, error) {
    var blocks []*Block

    err := db.QueryWithFinal(ctx).
        Model(&blocks).
        OrderExpr("height DESC").
        Limit(perPage).
        Offset((page - 1) * perPage).
        Scan(ctx)

    return blocks, err
}
```

---

## Summary Checklist

### DevOps
- [ ] Set `do_not_merge_across_partitions_select_final = 1` globally
- [ ] Configure memory limits (10GB+)
- [ ] All tables use ReplacingMergeTree + monthly partitions
- [ ] Monitor partition count (keep under 100)

### Application
- [ ] Implement `OptimizePartitionsWorkflow` and `OptimizeTable` activity
- [ ] Register a monthly schedule for each chain on the OPS queue
- [ ] Monitor scheduled workflow executions in Temporal UI

### Developers
- [ ] Use `FINAL` in ALL SELECT queries
- [ ] Use `QueryWithFinal()` helper function
- [ ] Add deduplication integration tests
- [ ] Validate queries return a single row for unique keys
- [ ] Never bypass watermark validation (`index_progress`)

---

## Performance Expectations

| Query Type                   | Slowdown | Acceptable?            |
|------------------------------|----------|------------------------|
| Point lookup (WHERE key = X) | 2×       | ✅ Yes (~16ms vs 8ms)   |
| Range query (small)          | 2×       | ✅ Yes (~100ms vs 50ms) |
| Range query (large)          | 2.5×     | ✅ Yes (~2.5s vs 1s)    |
| Full table scan              | 3×       | ⚠️ Avoid if possible   |

**Optimization:** Old partitions (merged to single part) have near-zero FINAL overhead.

---

## Trade-off Accepted

**Cost:** 2× query overhead
**Benefit:** Data integrity, automatic deduplication, no manual cleanup
**Decision:** Acceptable for production use ✅

r# Cross-Chain Database Architecture

## Overview

CanopyX uses a three-layer ClickHouse database architecture to manage blockchain data across multiple chains while enabling efficient cross-chain queries.

## 1. Three-Layer Structure

```
┌─────────────────────────────────────────────────────────────────┐
│                     ADMIN DATABASE (canopyx)                    │
│  - chains (chain registry)                                      │
│  - index_progress, index_progress_agg (indexing tracking)      │
│  - reindex_requests (reindex audit log)                        │
│  - rpc_endpoints (RPC health tracking)                         │
└─────────────────────────────────────────────────────────────────┘
                                  ▲
                                  │ manages/tracks
                                  ▼
┌─────────────────────────────────────────────────────────────────┐
│              PER-CHAIN DATABASES (chain_1, chain_2, ...)        │
│                                                                  │
│  Each chain has its own ClickHouse database with:               │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ Production Tables (ReplacingMergeTree)                    │  │
│  │  - accounts                                               │  │
│  │  - validators                                             │  │
│  │  - validator_signing_info                                 │  │
│  │  - validator_double_signing_info                          │  │
│  │  - pools                                                  │  │
│  │  - pool_points_by_holder                                  │  │
│  │  - orders                                                 │  │
│  │  - dex_orders                                             │  │
│  │  - dex_deposits                                           │  │
│  │  - dex_withdrawals                                        │  │
│  │  - block_summaries                                        │  │
│  │  - committee_payments                                     │  │
│  │  + blocks, transactions, events, committees, params, etc. │  │
│  └──────────────────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ Staging Tables (same schema, for atomic indexing)        │  │
│  │  - accounts_staging                                       │  │
│  │  - validators_staging                                     │  │
│  │  - ... (same 12 entities)                                │  │
│  └──────────────────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ Materialized Views (sync to cross-chain)                 │  │
│  │  - accounts_sync_chain_1         ──────┐                 │  │
│  │  - validators_sync_chain_1       ──────┤                 │  │
│  │  - pools_sync_chain_1            ──────┤                 │  │
│  │  - ... (12 MVs per chain)        ──────┤                 │  │
│  └──────────────────────────────────────────┼───────────────┘  │
└──────────────────────────────────────────────┼──────────────────┘
                                               │
                            Materialized Views │ auto-sync NEW data
                                               ▼
┌─────────────────────────────────────────────────────────────────┐
│            CROSS-CHAIN DATABASE (canopyx_global)                │
│                                                                  │
│  Global tables aggregating data from ALL chains:                │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ Global Tables (ReplacingMergeTree with height version)   │  │
│  │  - accounts_global                                        │  │
│  │    Columns: chain_id + [account cols] + updated_at       │  │
│  │    ORDER BY: (chain_id, address)                          │  │
│  │                                                           │  │
│  │  - validators_global                                      │  │
│  │    Columns: chain_id + [validator cols] + updated_at     │  │
│  │    ORDER BY: (chain_id, address)                          │  │
│  │                                                           │  │
│  │  - pools_global                                           │  │
│  │    Columns: chain_id + [pool cols (pool_chain_id)] + ... │  │
│  │    ORDER BY: (chain_id, pool_id)                          │  │
│  │                                                           │  │
│  │  - block_summaries_global                                 │  │
│  │  - orders_global                                          │  │
│  │  - dex_orders_global                                      │  │
│  │  - dex_deposits_global                                    │  │
│  │  - dex_withdrawals_global                                 │  │
│  │  - pool_points_by_holder_global                           │  │
│  │  - validator_signing_info_global                          │  │
│  │  - validator_double_signing_info_global                   │  │
│  │  - committee_payments_global                              │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

## 2. Materialized View Sync Mechanism

### Overview

Materialized views in each chain database automatically sync data to the cross-chain global database in real-time.

**Location:** Chain database (e.g., `chain_1`)
**Name Pattern:** `{table}_sync_chain_{chainID}`
**Example:** `accounts_sync_chain_1`

### SQL Structure

```sql
CREATE MATERIALIZED VIEW "chain_1"."accounts_sync_chain_1"
TO "canopyx_global"."accounts_global"
AS SELECT
    1 AS chain_id,          -- Hard-coded chain ID
    address,                 -- Source columns
    height,
    balance,
    ... (all account fields),
    now64(6) AS updated_at  -- Sync timestamp
FROM "chain_1"."accounts"
```

### How It Works

1. **Automatic trigger:** When a row is inserted into `chain_1.accounts`
2. **Immediate sync:** Materialized view automatically inserts to `accounts_global`
3. **Adds context:** Injects `chain_id` and `updated_at` columns
4. **Deduplication:** ReplacingMergeTree uses `height` as version column

## 3. Schema Relationships

### Chain Table Schema

Example: `accounts` table in `chain_1` database

```sql
CREATE TABLE chain_1.accounts (
    address String,
    height UInt64,          -- Version column for deduplication
    balance UInt64,
    staked_balance UInt64,
    height_time DateTime64(6),
    ...
) ENGINE = ReplacingMergeTree(height)
ORDER BY (address, height)
```

### Global Table Schema

Example: `accounts_global` table in `canopyx_global` database

```sql
CREATE TABLE canopyx_global.accounts_global (
    chain_id UInt64,        -- NEW: identifies source chain
    address String,
    height UInt64,          -- Version column (preserved from source)
    balance UInt64,
    staked_balance UInt64,
    height_time DateTime64(6),
    ...,
    updated_at DateTime64(6) -- NEW: sync timestamp
) ENGINE = ReplacingMergeTree(height)
ORDER BY (chain_id, address)  -- Different: chain_id prefix
```

### Key Differences

| Aspect | Chain Table | Global Table |
|--------|-------------|--------------|
| **Scope** | Single chain data | All chains aggregated |
| **chain_id** | Not present | Added as first column |
| **updated_at** | Not present | Sync timestamp |
| **ORDER BY** | Entity-specific | `(chain_id, ...)` prefix |
| **Primary Key** | `(entity_id, height)` | `(chain_id, entity_id)` |

## 4. The 12 Synced Entities

These tables are automatically synced from each chain database to the global database:

| Table Name | Primary Key (Global) | Has Address Bloom Filter |
|------------|---------------------|--------------------------|
| `accounts` | `(chain_id, address)` | ✓ |
| `validators` | `(chain_id, address)` | ✓ |
| `validator_signing_info` | `(chain_id, address)` | ✓ |
| `validator_double_signing_info` | `(chain_id, address)` | ✓ |
| `pools` | `(chain_id, pool_id)` | ✗ |
| `pool_points_by_holder` | `(chain_id, address, pool_id)` | ✓ |
| `orders` | `(chain_id, order_id)` | ✗ |
| `dex_orders` | `(chain_id, order_id)` | ✗ |
| `dex_deposits` | `(chain_id, order_id)` | ✗ |
| `dex_withdrawals` | `(chain_id, order_id)` | ✗ |
| `block_summaries` | `(chain_id, height)` | ✗ |
| `committee_payments` | `(chain_id, committee_id, address, height)` | ✓ |

### Tables NOT Synced to Global

The following tables remain chain-specific only:

- `blocks` - Too large, rarely queried cross-chain
- `transactions` - Too large, rarely queried cross-chain
- `events` - Too large, event-specific queries
- `committees` - Chain-specific governance
- `committee_validators` - Junction table
- `params` - Chain-specific parameters
- `poll_snapshots` - Chain-specific polls
- `supply` - Chain-specific supply tracking

## 5. Data Flow

```
┌──────────────┐
│   Indexer    │
│  (Temporal)  │
└──────┬───────┘
       │ 1. Index blocks
       ▼
┌─────────────────────────────┐
│  chain_X.{table}_staging    │ ◄── Staging for atomic writes
└──────┬──────────────────────┘
       │ 2. Promote to production
       ▼
┌─────────────────────────────┐
│  chain_X.{table}            │ ◄── Production table
└──────┬──────────────────────┘
       │ 3. Materialized view auto-triggers
       ▼
┌─────────────────────────────┐
│  {table}_sync_chain_X       │ ◄── MV in chain database
│  (Materialized View)        │
└──────┬──────────────────────┘
       │ 4. INSERT INTO global table
       ▼
┌─────────────────────────────┐
│  canopyx_global.            │
│  {table}_global             │ ◄── Aggregated cross-chain data
└─────────────────────────────┘
```

### Step-by-Step Process

1. **Index** - Temporal workflow indexes blocks and writes to `{table}_staging`
2. **Promote** - After successful indexing, data is promoted to production table
3. **Auto-sync** - Materialized view automatically triggers on INSERT
4. **Aggregate** - Data appears in global table with `chain_id` and `updated_at`

## 6. Special Column Handling

### Pools Table Exception

The `pools` table has a special case to avoid column name collision:

**Chain table:**
```sql
CREATE TABLE chain_1.pools (
    chain_id UInt64,  -- Refers to pool's source chain
    pool_id UInt64,
    ...
)
```

**Global table:**
```sql
CREATE TABLE canopyx_global.pools_global (
    chain_id UInt64,       -- Refers to the INDEXER's chain (e.g., chain_1)
    pool_chain_id UInt64,  -- Renamed from pools.chain_id (pool's source chain)
    pool_id UInt64,
    ...
)
```

**Materialized view handles renaming:**
```sql
CREATE MATERIALIZED VIEW chain_1.pools_sync_chain_1
TO canopyx_global.pools_global
AS SELECT
    1 AS chain_id,                    -- Indexer's chain ID
    chain_id AS pool_chain_id,        -- Rename pool's chain_id
    pool_id,
    ...
FROM chain_1.pools
```

This allows distinguishing between:
- **chain_id**: Which indexer/chain database this record came from
- **pool_chain_id**: Which chain the pool itself belongs to (for cross-chain pools)

## 7. Chain Lifecycle Management

### Adding a New Chain

When a new chain is added via `/api/chains` (POST):

1. Chain registered in `canopyx.chains` table
2. Chain database created: `chain_{id}`
3. Production and staging tables created in chain DB
4. Cross-chain sync setup:
   - 12 materialized views created in chain DB
   - MVs start auto-syncing NEW data
5. **Backfill** (separate step): `ResyncChain` copies existing data

### Deleting a Chain

When a chain is deleted via `/api/chains/{id}` (DELETE):

```
HandleChainDelete Sequence:

Step 1-2: Stop indexing
  ├── Delete Temporal head scan schedule
  └── Delete Temporal gap scan schedule

Step 3: Mark as deleted
  └── UPDATE chains SET deleted = 1 WHERE chain_id = X

Step 4: Remove cross-chain sync (RemoveChainSync)
  ├── Drop chain_X.accounts_sync_chain_X
  ├── Drop chain_X.validators_sync_chain_X
  ├── ... (all 12 materialized views)
  ├── DELETE FROM accounts_global WHERE chain_id = X
  ├── DELETE FROM validators_global WHERE chain_id = X
  └── ... (all 12 global tables)

Step 5-6: Clear in-memory caches
  ├── Remove from ChainsDB map
  └── Clear QueueStatsCache

Step 7-9: Clean admin tracking data
  ├── DELETE FROM rpc_endpoints WHERE chain_id = X
  ├── DELETE FROM index_progress WHERE chain_id = X (base + agg)
  └── DELETE FROM reindex_requests WHERE chain_id = X

Step 10: Drop chain database
  └── DROP DATABASE IF EXISTS chain_X
```

**Important:** Materialized views must be dropped BEFORE dropping the chain database, otherwise ClickHouse will error.

## 8. Query Patterns

### Single Chain Query

Query data from a specific chain:

```sql
-- Direct chain query
SELECT * FROM chain_1.accounts
WHERE address = '0x1234...'
```

### Cross-Chain Query

Query data across all chains:

```sql
-- Find account across all chains
SELECT chain_id, address, balance, updated_at
FROM canopyx_global.accounts_global
WHERE address = '0x1234...'
ORDER BY chain_id
```

### Using FINAL for Consistency

ReplacingMergeTree tables may have duplicate rows until background merges complete. Use `FINAL` to force deduplication:

```sql
-- Ensure deduplicated results
SELECT * FROM accounts_global FINAL
WHERE chain_id = 1 AND address = '0x1234...'
```

**Performance Note:** `FINAL` is expensive. Use it only when consistency is critical (e.g., admin endpoints, reports).

### Cross-Chain Aggregation

```sql
-- Total staked across all chains
SELECT
    SUM(staked_balance) as total_staked,
    COUNT(DISTINCT chain_id) as num_chains
FROM accounts_global FINAL
WHERE staked_balance > 0
```

### Per-Chain Aggregation

```sql
-- Staked per chain
SELECT
    chain_id,
    SUM(staked_balance) as total_staked,
    COUNT(*) as num_stakers
FROM accounts_global FINAL
GROUP BY chain_id
ORDER BY total_staked DESC
```

## 9. Architecture Benefits

This design provides:

✅ **Per-chain isolation** - Each chain has its own database, preventing cross-contamination
✅ **Cross-chain aggregation** - Global tables enable querying across all chains
✅ **Real-time sync** - Materialized views provide automatic, immediate data propagation
✅ **Deduplication** - ReplacingMergeTree with height versioning handles concurrent writes
✅ **Clean deletion** - Cascading cleanup: MVs → global data → admin tracking → chain DB
✅ **Scalability** - Adding new chains doesn't impact existing chain query performance
✅ **Bloom filters** - Address-indexed tables use bloom filters for fast lookups
✅ **Historical accuracy** - Height-based versioning ensures correct state at any block height

## 10. Implementation Details

### Key Files

- **`pkg/db/crosschain/store.go`** - Cross-chain database management
  - `InitializeSchema()` - Create 12 global tables
  - `SetupChainSync()` - Create 12 materialized views for a chain
  - `RemoveChainSync()` - Drop MVs and delete global data
  - `ResyncChain()` - Backfill historical data

- **`pkg/db/chain/db.go`** - Per-chain database management
  - `InitializeDB()` - Create production and staging tables

- **`pkg/db/admin/db.go`** - Admin database management
  - `DropChainDatabase()` - Drop chain database

- **`app/admin/controller/chain.go`** - Chain CRUD operations
  - `HandleChainPost()` - Create chain + setup sync
  - `HandleChainDelete()` - Complete chain cleanup

### Column Definitions

Schema definitions are centralized in `pkg/db/models/indexer/`:
- Each entity has a `ColumnDef` array (e.g., `AccountColumns`)
- `ColumnsToSchemaSQL()` - Generate CREATE TABLE schema
- `GetCrossChainColumnNames()` - Get column list for global tables
- `FilterCrossChainColumns()` - Exclude chain-specific columns

## 11. Maintenance Operations

### Resync Chain Data

After materialized view creation, backfill historical data:

```go
// POST /api/crosschain/resync/{chainID}
func (c *Controller) HandleCrossChainResyncChain(w http.ResponseWriter, r *http.Request)
```

This is necessary because materialized views only sync NEW data inserted after their creation.

### Resync Specific Table

Resync a single table for a chain:

```go
// POST /api/crosschain/resync/{chainID}/{table}
func (c *Controller) HandleCrossChainResyncTable(w http.ResponseWriter, r *http.Request)
```

### Optimize Global Tables

Periodically run OPTIMIZE to trigger final merges and remove old versions:

```go
store.OptimizeTables(ctx)
```

This should be run weekly or monthly depending on data volume.

### Health Monitoring

Check sync status and lag:

```go
// GET /api/crosschain/health
health := store.GetHealthStatus(ctx)

// GET /api/crosschain/sync-status/{chainID}/{table}
status := store.GetSyncStatus(ctx, chainID, tableName)
```

Lag metrics help identify sync issues or performance problems.

## 12. Troubleshooting

### Materialized View Not Syncing

1. Check if MV exists: `SHOW TABLES FROM chain_1 LIKE '%sync%'`
2. Check MV definition: `SHOW CREATE TABLE chain_1.accounts_sync_chain_1`
3. Check for errors: Query ClickHouse logs
4. Recreate MV: Drop and call `SetupChainSync()` again

### Missing Historical Data

Materialized views only sync NEW inserts. If data was already present:
1. Call `ResyncChain(chainID)` to backfill
2. Or call `ResyncTable(chainID, tableName)` for specific table

### High Lag in Global Tables

Check sync status:
```go
status := store.GetSyncStatus(ctx, chainID, "accounts")
fmt.Printf("Lag: %d rows\n", status.Lag)
```

Causes:
- High write volume overwhelming sync
- ClickHouse server resource constraints
- Need to run `OptimizeTables()` to merge parts

### Column Mismatch Errors

If schema changes, the materialized view SELECT may not match global table columns:
1. Drop affected MVs
2. Update column definitions in `pkg/db/models/indexer/`
3. Recreate global tables (if needed)
4. Recreate MVs with `SetupChainSync()`
5. Backfill with `ResyncChain()`

## 13. Future Considerations

### Sharding

For very large deployments, consider:
- Sharding global tables by `chain_id` ranges
- Distributed tables across multiple ClickHouse nodes
- Separate global databases per region

### Compression

Global tables can benefit from:
- Compression codecs (e.g., `ZSTD`, `LZ4`)
- TTL policies for old data
- Partitioning by time ranges

### Async Deletes

Current `DELETE FROM global_tables` is synchronous. For better performance:
- Use `ALTER TABLE DELETE` (async mutation)
- Or use `updated_at` filtering + periodic cleanup

This architecture document will be updated as the system evolves.

---

## 14. Common Scenarios & How-To Guides

### Scenario 1: Adding a Completely New Entity to Cross-Chain Sync

**Example:** You want to add a new `potatoes` entity that should be synced across all chains.

#### Step 1: Define the Entity Model

Create the entity struct and column definitions in `pkg/db/models/indexer/`:

**File:** `pkg/db/models/indexer/potato.go`

```go
package indexer

import "time"

const PotatoesProductionTableName = "potatoes"
const PotatoesStagingTableName = PotatoesProductionTableName + entities.StagingSuffix

// PotatoColumns defines the schema for the potatoes table.
// This is the single source of truth - used by both chain tables and cross-chain tables.
var PotatoColumns = []ColumnDef{
	{Name: "potato_id", Type: "String", Codec: "ZSTD(1)"},
	{Name: "height", Type: "UInt64", Codec: "DoubleDelta, LZ4"},
	{Name: "variety", Type: "String", Codec: "ZSTD(1)"},
	{Name: "weight", Type: "UInt64", Codec: "Delta, ZSTD(3)"},
	{Name: "farmer_address", Type: "String", Codec: "ZSTD(1)"},
	{Name: "is_organic", Type: "Bool"},
	{Name: "height_time", Type: "DateTime64(6)", Codec: "DoubleDelta, LZ4"},

	// Example: Skip large fields from cross-chain sync
	{Name: "growing_notes", Type: "String", Codec: "ZSTD(3)", CrossChainSkip: true},
}

// Potato represents a potato entity (obviously a fake example).
type Potato struct {
	PotatoID      string    `ch:"potato_id" json:"potato_id"`
	Height        uint64    `ch:"height" json:"height"`
	Variety       string    `ch:"variety" json:"variety"`
	Weight        uint64    `ch:"weight" json:"weight"`
	FarmerAddress string    `ch:"farmer_address" json:"farmer_address"`
	IsOrganic     bool      `ch:"is_organic" json:"is_organic"`
	HeightTime    time.Time `ch:"height_time" json:"height_time"`
	GrowingNotes  string    `ch:"growing_notes" json:"growing_notes"` // Excluded from cross-chain
}
```

**Key Points:**
- Use `CrossChainSkip: true` for columns too large or granular for cross-chain queries
- Use `CrossChainRename: "new_name"` to avoid naming conflicts (see pools example)
- Always include `height` and `height_time` for versioning and time-range queries

#### Step 2: Add Chain Database Methods

Add database operations in `pkg/db/chain/`:

**File:** `pkg/db/chain/potato.go`

```go
package chain

import (
	"context"
	"fmt"
	"strings"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// InsertPotatoesStaging inserts potatoes to the staging table
func (db *DB) InsertPotatoesStaging(ctx context.Context, potatoes []*indexer.Potato) error {
	if len(potatoes) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		%s
	)`, db.DatabaseName(), indexer.PotatoesStagingTableName,
		strings.Join(indexer.ColumnsToNameList(indexer.PotatoColumns), ", "))

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}

	for _, p := range potatoes {
		if err := batch.Append(
			p.PotatoID,
			p.Height,
			p.Variety,
			p.Weight,
			p.FarmerAddress,
			p.IsOrganic,
			p.HeightTime,
			p.GrowingNotes,
		); err != nil {
			return fmt.Errorf("append to batch: %w", err)
		}
	}

	return batch.Send()
}
```

#### Step 3: Create Schema Initialization

Add table creation in `pkg/db/chain/init.go`:

```go
func (db *DB) createPotatoesTable(ctx context.Context) error {
	// Production table
	prodQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s"
		(
			%s
		)
		ENGINE = ReplacingMergeTree(height)
		ORDER BY (potato_id, height)
		SETTINGS index_granularity = 8192
	`, db.DatabaseName(), indexer.PotatoesProductionTableName,
		indexer.ColumnsToSchemaSQL(indexer.PotatoColumns))

	if err := db.Exec(ctx, prodQuery); err != nil {
		return fmt.Errorf("create potatoes table: %w", err)
	}

	// Staging table (same schema)
	stagingQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s"
		(
			%s
		)
		ENGINE = ReplacingMergeTree(height)
		ORDER BY (potato_id, height)
		SETTINGS index_granularity = 8192
	`, db.DatabaseName(), indexer.PotatoesStagingTableName,
		indexer.ColumnsToSchemaSQL(indexer.PotatoColumns))

	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create potatoes_staging table: %w", err)
	}

	return nil
}

// Add to InitializeDB():
func (db *DB) InitializeDB(ctx context.Context) error {
	// ... existing tables ...

	if err := db.createPotatoesTable(ctx); err != nil {
		return err
	}

	// ... rest of tables ...
}
```

#### Step 4: Add to Cross-Chain Configuration

Update `pkg/db/crosschain/store.go`:

**In `GetTableConfigs()`:**

```go
func GetTableConfigs() []TableConfig {
	return []TableConfig{
		// ... existing configs ...

		{
			TableName:        indexer.PotatoesProductionTableName,
			PrimaryKey:       []string{"chain_id", "potato_id"},
			HasAddressColumn: true, // Set true for bloom filter on farmer_address
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.PotatoColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.PotatoColumns),
		},
	}
}
```

**In `getColumnsForTable()`:**

```go
func getColumnsForTable(tableName string) []indexer.ColumnDef {
	switch tableName {
	// ... existing cases ...

	case indexer.PotatoesProductionTableName:
		return indexer.PotatoColumns

	default:
		return nil
	}
}
```

#### Step 5: Add Indexing Activity

Create activity in `app/indexer/activity/potato.go`:

```go
package activity

import (
	"context"
	"time"
	"github.com/canopy-network/canopyx/app/indexer/types"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// IndexPotatoes indexes potatoes for a given block.
func (ac *Context) IndexPotatoes(ctx context.Context, in types.ActivityIndexAtHeight) (types.ActivityIndexPotatoesOutput, error) {
	start := time.Now()

	cli, err := ac.rpcClientForHeight(ctx, in.Height)
	if err != nil {
		return types.ActivityIndexPotatoesOutput{}, err
	}

	chainDb, err := ac.GetChainDb(ctx, ac.ChainID)
	if err != nil {
		return types.ActivityIndexPotatoesOutput{}, err
	}

	// Fetch potatoes from RPC
	rpcPotatoes, err := cli.PotatoesByHeight(ctx, in.Height)
	if err != nil {
		return types.ActivityIndexPotatoesOutput{}, err
	}

	// Convert to indexer models
	potatoes := make([]*indexer.Potato, 0, len(rpcPotatoes))
	for _, rpcPotato := range rpcPotatoes {
		potato := &indexer.Potato{
			PotatoID:      rpcPotato.ID,
			Height:        in.Height,
			Variety:       rpcPotato.Variety,
			Weight:        rpcPotato.Weight,
			FarmerAddress: rpcPotato.FarmerAddress,
			IsOrganic:     rpcPotato.IsOrganic,
			HeightTime:    in.BlockTime,
			GrowingNotes:  rpcPotato.Notes,
		}
		potatoes = append(potatoes, potato)
	}

	// Insert to staging
	if err := chainDb.InsertPotatoesStaging(ctx, potatoes); err != nil {
		return types.ActivityIndexPotatoesOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.ActivityIndexPotatoesOutput{
		NumPotatoes: uint32(len(potatoes)),
		DurationMs:  durationMs,
	}, nil
}
```

#### Step 6: Deploy Schema Changes

**The system is idempotent - schema creation happens automatically on startup!**

##### Option A: Automatic Schema Creation (Recommended)

1. **Deploy updated admin image:**
   - Admin starts
   - `InitializeSchema()` runs → sees `potatoes_global` doesn't exist → **creates it automatically**
   - `SetupChainSync()` runs for all chains → sees materialized views missing → **creates them automatically**

2. **Deploy updated indexer images:**
   - Each indexer starts
   - `createPotatoesTable()` runs → sees `potatoes` and `potatoes_staging` don't exist → **creates them automatically**
   - Indexers start indexing → new data flows through materialized views to global table

3. **(Optional) Backfill historical data:**
```bash
# Via API endpoint (only needed if you have old data to sync)
curl -X POST http://localhost:8080/api/crosschain/resync/1/potatoes
```

**That's it!** No manual SQL needed. The system uses `CREATE TABLE IF NOT EXISTS` and `CREATE MATERIALIZED VIEW IF NOT EXISTS` everywhere.

##### Option B: Manual Schema Creation (If You Want Tables Before Code Deploy)

If you prefer to create tables before deploying code:

1. **Create tables in existing chain databases:**
```sql
-- Run for each chain_X database
CREATE TABLE IF NOT EXISTS "chain_1"."potatoes" (
	potato_id String CODEC(ZSTD(1)),
	height UInt64 CODEC(DoubleDelta, LZ4),
	variety String CODEC(ZSTD(1)),
	weight UInt64 CODEC(Delta, ZSTD(3)),
	farmer_address String CODEC(ZSTD(1)),
	is_organic Bool,
	height_time DateTime64(6) CODEC(DoubleDelta, LZ4),
	growing_notes String CODEC(ZSTD(3))
)
ENGINE = ReplacingMergeTree(height)
ORDER BY (potato_id, height);

-- Same for potatoes_staging
```

2. **Create global table in cross-chain database:**
```sql
CREATE TABLE IF NOT EXISTS "canopyx_global"."potatoes_global" (
	chain_id UInt64,
	potato_id String CODEC(ZSTD(1)),
	height UInt64 CODEC(DoubleDelta, LZ4),
	variety String CODEC(ZSTD(1)),
	weight UInt64 CODEC(Delta, ZSTD(3)),
	farmer_address String CODEC(ZSTD(1)),
	is_organic Bool,
	height_time DateTime64(6) CODEC(DoubleDelta, LZ4),
	-- Note: growing_notes excluded (CrossChainSkip)
	updated_at DateTime64(6),
	INDEX farmer_address_bloom farmer_address TYPE bloom_filter GRANULARITY 4
)
ENGINE = ReplacingMergeTree(height)
ORDER BY (chain_id, potato_id);
```

3. **Create materialized views for each chain:**
```sql
-- For chain_1
CREATE MATERIALIZED VIEW IF NOT EXISTS "chain_1"."potatoes_sync_chain_1"
TO "canopyx_global"."potatoes_global"
AS SELECT
	1 AS chain_id,
	potato_id,
	height,
	variety,
	weight,
	farmer_address,
	is_organic,
	height_time,
	-- growing_notes excluded
	now64(6) AS updated_at
FROM "chain_1"."potatoes";
```

4. **Then deploy code and restart services**

**Note:** This manual approach is optional. The automatic approach (Option A) is simpler and recommended.

#### Step 7: Update Promote Activity

Add potatoes to the promote workflow in `app/indexer/activity/promote.go`:

```go
// PromoteTables promotes staging tables to production atomically
func (ac *Context) PromoteTables(ctx context.Context, in types.ActivityPromoteInput) error {
	tables := []string{
		indexer.BlocksProductionTableName,
		indexer.PotatoesProductionTableName, // ADD THIS
		// ... rest of tables ...
	}

	// Promote all tables atomically
	// ...
}
```

---

### Scenario 2: Adding a New Field to Existing Cross-Chain Entity

**Example:** You want to add `rewards` and `slashes` fields to the existing `accounts` entity.

#### Step 1: Update Column Definitions

**File:** `pkg/db/models/indexer/account.go`

```go
// AccountColumns defines the schema for the accounts table.
var AccountColumns = []ColumnDef{
	{Name: "address", Type: "String", Codec: "ZSTD(1)"},
	{Name: "amount", Type: "UInt64", Codec: "Delta, ZSTD(3)"},

	// NEW FIELDS - Add here
	{Name: "rewards", Type: "UInt64", Codec: "Delta, ZSTD(3)"},
	{Name: "slashes", Type: "UInt64", Codec: "Delta, ZSTD(3)"},

	{Name: "height", Type: "UInt64", Codec: "DoubleDelta, LZ4"},
	{Name: "height_time", Type: "DateTime64(6)", Codec: "DoubleDelta, LZ4"},
}

// Update struct to include new fields
type Account struct {
	Address    string    `ch:"address" json:"address"`
	Amount     uint64    `ch:"amount" json:"amount"`

	// NEW FIELDS
	Rewards    uint64    `ch:"rewards" json:"rewards"`
	Slashes    uint64    `ch:"slashes" json:"slashes"`

	Height     uint64    `ch:"height" json:"height"`
	HeightTime time.Time `ch:"height_time" json:"height_time"`
}
```

#### Step 2: Update Database Methods

**File:** `pkg/db/chain/account.go`

Update `InsertAccountsStaging` to include new fields:

```go
func (db *DB) InsertAccountsStaging(ctx context.Context, accounts []*indexer.Account) error {
	// ... batch setup ...

	for _, acc := range accounts {
		if err := batch.Append(
			acc.Address,
			acc.Amount,
			acc.Rewards,  // ADD
			acc.Slashes,  // ADD
			acc.Height,
			acc.HeightTime,
		); err != nil {
			return fmt.Errorf("append to batch: %w", err)
		}
	}

	return batch.Send()
}
```

Update query methods to SELECT new fields:

```go
func (db *DB) GetAccount(ctx context.Context, address string, staging bool) (*indexer.Account, error) {
	// ... query setup ...

	var acc indexer.Account
	if err := row.Scan(
		&acc.Address,
		&acc.Amount,
		&acc.Rewards,   // ADD
		&acc.Slashes,   // ADD
		&acc.Height,
		&acc.HeightTime,
	); err != nil {
		return nil, err
	}

	return &acc, nil
}
```

#### Step 3: Update Indexing Activity

**File:** `app/indexer/activity/accounts.go`

Update the activity to populate new fields:

```go
func (ac *Context) IndexAccounts(ctx context.Context, input types.ActivityIndexAtHeight) (types.ActivityIndexAccountsOutput, error) {
	// ... existing code to fetch RPC(H) and RPC(H-1) ...

	// Query reward and slash events
	accountEvents, err := chainDb.GetEventsByTypeAndHeight(ctx, input.Height, true,
		rpc.EventTypeAsStr(rpc.EventTypeReward),
		rpc.EventTypeAsStr(rpc.EventTypeSlash),
	)
	if err != nil {
		return types.ActivityIndexAccountsOutput{}, err
	}

	// Build event maps
	rewardEvents := make(map[string]*indexer.Event)
	slashEvents := make(map[string]*indexer.Event)
	for _, event := range accountEvents {
		switch event.EventType {
		case "EventReward":
			rewardEvents[event.Address] = event
		case "EventSlash":
			slashEvents[event.Address] = event
		}
	}

	// Create snapshots with new fields
	for _, curr := range currentAccounts {
		prevAmount, existed := prevMap[curr.Address]

		if curr.Amount != prevAmount {
			account := &indexer.Account{
				Address:    curr.Address,
				Amount:     curr.Amount,
				Height:     input.Height,
				HeightTime: input.BlockTime,
			}

			// Populate new fields from events
			if rewardEvent, hasReward := rewardEvents[curr.Address]; hasReward {
				if rewardEvent.Amount != nil {
					account.Rewards = *rewardEvent.Amount
				}
			}
			if slashEvent, hasSlash := slashEvents[curr.Address]; hasSlash {
				if slashEvent.Amount != nil {
					account.Slashes = *slashEvent.Amount
				}
			}

			changedAccounts = append(changedAccounts, account)
		}
	}

	// ... insert to staging ...
}
```

#### Step 4: Deploy Schema Changes

**GOOD NEWS:** ClickHouse fully supports `ALTER TABLE ADD COLUMN` for ReplacingMergeTree! This is a **metadata-only operation** with no downtime.

Reference: [ClickHouse Column Manipulations](https://clickhouse.com/docs/sql-reference/statements/alter/column) and [How ALTERs work in ClickHouse](https://kb.altinity.com/altinity-kb-setup-and-maintenance/alters/)

##### The Simple Approach (Production-Safe, No Downtime)

**How it works:**
- ALTER TABLE ADD COLUMN only updates table metadata (takes milliseconds)
- Locks the table for ~1 millisecond to change metadata
- Old data parts: ClickHouse synthesizes default values on-the-fly during reads
- New data parts: Application writes actual values
- No data copying needed = No race condition!

**Step-by-step:**

1. **Add columns to production tables (for each chain):**

```sql
-- For chain_1 (repeat for all chains)
-- This is INSTANT - metadata-only change
ALTER TABLE "chain_1"."accounts"
ADD COLUMN rewards UInt64 DEFAULT 0,
ADD COLUMN slashes UInt64 DEFAULT 0;
```

2. **Add columns to staging tables:**

```sql
-- For chain_1 (repeat for all chains)
ALTER TABLE "chain_1"."accounts_staging"
ADD COLUMN rewards UInt64 DEFAULT 0,
ADD COLUMN slashes UInt64 DEFAULT 0;
```

3. **Update global table:**

```sql
-- Drop materialized views first (IMPORTANT!)
DROP TABLE IF EXISTS "chain_1"."accounts_sync_chain_1";
DROP TABLE IF EXISTS "chain_2"."accounts_sync_chain_2";
-- ... for all chains

-- Drop and recreate global table with new schema
DROP TABLE IF EXISTS "canopyx_global"."accounts_global";

CREATE TABLE "canopyx_global"."accounts_global" (
	chain_id UInt64,
	address String CODEC(ZSTD(1)),
	amount UInt64 CODEC(Delta, ZSTD(3)),
	rewards UInt64 CODEC(Delta, ZSTD(3)),    -- NEW
	slashes UInt64 CODEC(Delta, ZSTD(3)),    -- NEW
	height UInt64 CODEC(DoubleDelta, LZ4),
	height_time DateTime64(6) CODEC(DoubleDelta, LZ4),
	updated_at DateTime64(6),
	INDEX address_bloom address TYPE bloom_filter GRANULARITY 4
)
ENGINE = ReplacingMergeTree(height)
ORDER BY (chain_id, address);
```

4. **Recreate materialized views:**

```sql
-- For chain_1
CREATE MATERIALIZED VIEW "chain_1"."accounts_sync_chain_1"
TO "canopyx_global"."accounts_global"
AS SELECT
	1 AS chain_id,
	address,
	amount,
	rewards,   -- NEW
	slashes,   -- NEW
	height,
	height_time,
	now64(6) AS updated_at
FROM "chain_1"."accounts";

-- Repeat for all chains (chain_2, chain_3, etc.)
```

5. **Deploy updated application code:**

Update your indexer code to populate the new fields (see Step 2 and Step 3).

6. **Backfill global table:**

```bash
# Resync all chains to populate global table
curl -X POST http://localhost:8080/api/crosschain/resync/1
curl -X POST http://localhost:8080/api/crosschain/resync/2
# ... for each chain
```

**What happens to existing data?**
- Queries on old data: ClickHouse returns `0` (the DEFAULT value)
- Queries on new data: Returns actual values written by updated application
- Background merges: When old parts merge, the new column gets materialized with default values
- Optional: If you need to backfill historical values from events, write a separate migration script

**Migration Timing:**
- ALTER TABLE: ~1-10ms (metadata only)
- Drop/Create materialized views: ~100ms per chain
- Resync: Depends on data volume (can run in background)
- **Total downtime: None!** The indexer keeps running throughout

##### Alternative: Recreate Tables (Development/Testing Only)

If you're in development and want a clean slate:

```sql
-- For each chain database
DROP TABLE IF EXISTS "chain_1"."accounts";
DROP TABLE IF EXISTS "chain_1"."accounts_staging";
DROP TABLE IF EXISTS "chain_1"."accounts_sync_chain_1";

-- Drop global table
DROP TABLE IF EXISTS "canopyx_global"."accounts_global";

-- Then rerun initialization code to recreate everything
-- Restart indexer to reindex from scratch
```

**Use this only in development - causes complete data loss!**

#### Step 5: Validation

After deployment, verify:

```sql
-- 1. Check schema matches
DESCRIBE "chain_1"."accounts";
DESCRIBE "canopyx_global"."accounts_global";

-- 2. Check row counts match
SELECT chain_id, COUNT(*)
FROM "canopyx_global"."accounts_global"
GROUP BY chain_id;

SELECT COUNT(*) FROM "chain_1"."accounts";

-- 3. Verify new fields exist and are populated
SELECT address, amount, rewards, slashes
FROM "chain_1"."accounts"
WHERE rewards > 0 OR slashes > 0
LIMIT 10;
```

---

### Common Pitfalls & Best Practices

#### Pitfall 1: Forgetting to Update Both Staging and Production

Always update both tables when adding fields:
- `{table}` (production)
- `{table}_staging` (staging)

#### Pitfall 2: Not Dropping Materialized Views Before Schema Changes

**ALWAYS** drop materialized views BEFORE changing global table schema:

```sql
-- CORRECT ORDER:
DROP TABLE "chain_1"."{table}_sync_chain_1";  -- 1. Drop MV first
DROP TABLE "canopyx_global"."{table}_global";  -- 2. Drop global table
-- 3. Recreate global table with new schema
-- 4. Recreate MV with new columns
```

If you don't follow this order, ClickHouse may throw errors or have inconsistent state.

#### Pitfall 3: Forgetting to Backfill After Schema Changes

Materialized views only sync **NEW** data inserted after their creation. After recreating MVs, you MUST backfill:

```bash
curl -X POST http://localhost:8080/api/crosschain/resync/{chainID}
```

#### Pitfall 4: Using CrossChainSkip and CrossChainRename Together

These flags are mutually exclusive. Validation will fail:

```go
// WRONG - will fail validation
{Name: "field", Type: "String",
	CrossChainSkip: true,
	CrossChainRename: "new_field"}  // ERROR!

// CORRECT - use one or the other
{Name: "field", Type: "String", CrossChainSkip: true}
{Name: "field", Type: "String", CrossChainRename: "new_field"}
```

#### Pitfall 5: Not Updating `getColumnsForTable()`

When adding a new entity, you MUST add it to the switch statement in `pkg/db/crosschain/store.go`:

```go
func getColumnsForTable(tableName string) []indexer.ColumnDef {
	switch tableName {
	case indexer.PotatoesProductionTableName:
		return indexer.PotatoColumns  // ADD THIS for new entities
	// ... rest of cases ...
	default:
		return nil
	}
}
```

---

### Rollback Procedures

If something goes wrong during migration:

#### For New Fields (Scenario 2)

```sql
-- 1. Stop indexing workflows

-- 2. Restore old table
RENAME TABLE "chain_1"."accounts" TO "chain_1"."accounts_failed";
RENAME TABLE "chain_1"."accounts_old" TO "chain_1"."accounts";

-- 3. Drop failed materialized views
DROP TABLE IF EXISTS "chain_1"."accounts_sync_chain_1";

-- 4. Recreate old global table and materialized views
-- (Use backup of old CREATE TABLE and CREATE MATERIALIZED VIEW statements)

-- 5. Restart indexing workflows
```

#### For New Entities (Scenario 1)

```sql
-- 1. Stop indexing workflows

-- 2. Drop materialized views
DROP TABLE IF EXISTS "chain_1"."potatoes_sync_chain_1";

-- 3. Drop global table
DROP TABLE IF EXISTS "canopyx_global"."potatoes_global";

-- 4. Drop chain tables
DROP TABLE IF EXISTS "chain_1"."potatoes";
DROP TABLE IF EXISTS "chain_1"."potatoes_staging";

-- 5. Remove from GetTableConfigs() and getColumnsForTable() in code

-- 6. Restart indexing workflows
```
# Database Models & Structure

Guide to CanopyX database code organization.

## Package Structure

```
pkg/db/
├── interfaces.go           # AdminStore and ChainStore interfaces
├── clickhouse/
│   └── client.go          # ClickHouse connection setup
├── admin/
│   ├── db.go              # AdminStore implementation
│   └── store.go           # Chain/progress management methods
├── chain/
│   ├── db.go              # ChainStore implementation
│   ├── store.go           # Core insert/promote/clear methods
│   ├── block.go           # Block table schema + insert
│   ├── transaction.go     # Transaction table schema + insert
│   ├── account.go         # Account table schema + insert
│   ├── validator.go       # Validator table schema + insert
│   ├── committee.go       # Committee table schema + insert
│   ├── pool.go            # Pool table schema + insert
│   ├── order.go           # Order table schema + insert
│   └── ...                # One file per entity
├── models/
│   ├── admin/             # Admin database models
│   │   ├── chain.go
│   │   ├── index_progress.go
│   │   └── reindex_request.go
│   └── indexer/           # Per-chain database models
│       ├── block.go
│       ├── transaction.go
│       ├── account.go
│       ├── validator.go
│       ├── committee.go
│       ├── pool.go
│       ├── order.go
│       └── ...            # One file per entity
└── entities/              # Type-safe query builders (advanced)
    ├── account.go
    ├── validator.go
    └── docs/              # Entity system documentation
```

## Two Database Types

### Admin Database (`pkg/db/admin/`)

**Purpose**: Manages chains and indexing progress across all chains.

**Interface**: `AdminStore` in `pkg/db/interfaces.go`

**Models**: `pkg/db/models/admin/`
- `Chain` - Chain metadata and RPC endpoints
- `IndexProgress` - Last indexed height per chain
- `ReindexRequest` - Re-indexing job tracking

**Key Methods**:
- `GetChain(chainID)` - Get chain configuration
- `ListChains()` - Get all chains
- `UpdateIndexProgress(chainID, height)` - Update last indexed height
- `CreateReindexRequest()` - Queue re-indexing job

### Chain Database (`pkg/db/chain/`)

**Purpose**: Stores blockchain data for a specific chain (blocks, txs, state).

**Interface**: `ChainStore` in `pkg/db/interfaces.go`

**Models**: `pkg/db/models/indexer/`
- Core: `Block`, `Transaction`, `Event`
- State: `Account`, `Validator`, `Committee`, `Pool`, `Order`, `Param`
- Derived: `BlockSummary`, `DexPrice`, `ValidatorSigningInfo`

**Key Methods**:
- `Insert*Staging([]*Model)` - Insert to staging table
- `PromoteAllStagingTables()` - Move staging → production
- `ClearAllStagingTables()` - Clean up after promotion

**Pattern**: Each entity gets one file in `pkg/db/chain/` with:
- Schema definition (CREATE TABLE)
- Insert method (batch insert to staging)

## How Models Work

Models are Go structs with ClickHouse tags:

```go
// See: pkg/db/models/indexer/validator.go
type Validator struct {
    Address      string   `ch:"address"`
    StakedAmount uint64   `ch:"staked_amount"`
    Height       uint64   `ch:"height"`
    // ... see file for all fields
}
```

**All models** follow this pattern:
- Struct fields map to ClickHouse columns via `ch:` tags
- Business logic methods (like `DeriveStatus()`) live in model files
- No SQL in model files - schemas are in `pkg/db/chain/`

## How Schemas Work

Each entity's schema is defined in `pkg/db/chain/`:

```go
// See: pkg/db/chain/validator.go
func (db *DB) initValidators(ctx) error {
    query := `
        CREATE TABLE IF NOT EXISTS validators (
            address String,
            staked_amount UInt64,
            ...
        ) ENGINE = ReplacingMergeTree(height)
        ORDER BY (address, height)
    `
    // Create production + staging tables
}
```

**Schema files** contain:
- `init*()` - CREATE TABLE for production and staging
- `Insert*Staging()` - Batch insert to staging table
- Table uses proper ClickHouse types and compression codecs

## Two-Phase Commit Pattern

All writes go through staging tables:

1. Activity calls `Insert*Staging()` for each entity type
2. Activity completes successfully
3. Workflow calls `PromoteAllStagingTables()` - atomic move
4. Workflow calls `ClearAllStagingTables()` - cleanup

**Why**: Ensures all-or-nothing writes per block (atomicity).

## Snapshot-on-Change

Models marked "snapshot-on-change" only insert when state changes:

**Applies to**: Account, Validator, Committee, Pool, Order, Param

**How it works**: Activities compare RPC(H) vs RPC(H-1) and only insert if different.

**See implementation**: `app/indexer/activity/validators.go` (reference example)

## Entity System (Advanced)

Type-safe query builders for complex queries.

**Location**: `pkg/db/entities/`

**Documentation**: `pkg/db/entities/README.md`

**Purpose**: Simplifies temporal queries (get state at specific height, history, etc.)

**When to use**:
- Complex temporal queries
- Multi-table joins
- Type-safe query building

**When NOT to use**:
- Simple inserts (use ChainStore methods)
- Bulk operations (use raw ClickHouse)

## Working with the Database

### Creating a new entity

1. Add model to `pkg/db/models/indexer/entity_name.go`
2. Add schema to `pkg/db/chain/entity_name.go` with `init*()` and `Insert*Staging()`
3. Add method to `ChainStore` interface in `pkg/db/interfaces.go`
4. Implement in `pkg/db/chain/store.go` (register init, promote, clear)
5. Create activity in `app/indexer/activity/entity_name.go`

### Reading models

- **Models**: Start in `pkg/db/models/indexer/` to see struct fields
- **Schemas**: Check `pkg/db/chain/` for table structure and ClickHouse types
- **Usage**: Look at `app/indexer/activity/` for how models are populated from RPC

### Understanding interfaces

- **AdminStore**: Chain management operations → `pkg/db/interfaces.go`
- **ChainStore**: Per-chain data operations → `pkg/db/interfaces.go`
- **Implementation**: Both interfaces are in `pkg/db/admin/db.go` and `pkg/db/chain/db.go`

## Key Files to Read

| What                 | File                                 | Why                               |
|----------------------|--------------------------------------|-----------------------------------|
| Database interfaces  | `pkg/db/interfaces.go`               | All available methods             |
| Admin implementation | `pkg/db/admin/db.go`                 | Chain management                  |
| Chain implementation | `pkg/db/chain/db.go`                 | Per-chain data                    |
| Store orchestration  | `pkg/db/chain/store.go`              | Init, promote, clear coordination |
| Model examples       | `pkg/db/models/indexer/validator.go` | Complete model with methods       |
| Schema examples      | `pkg/db/chain/validator.go`          | CREATE TABLE + insert             |
| Activity examples    | `app/indexer/activity/validators.go` | How to use models + store         |

## Further Reading

- ClickHouse ReplacingMergeTree: Official ClickHouse docs
- Entity system: `pkg/db/entities/README.md`
- Snapshot-on-change: `docs/rpc.md` (explains RPC(H) vs RPC(H-1))

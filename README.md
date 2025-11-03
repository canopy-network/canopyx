# CanopyX

Blockchain indexer for Canopy protocol. Indexes blocks, transactions, and chain state into ClickHouse for fast querying.

## What It Is

CanopyX is a distributed indexer that processes Canopy blockchain data using Temporal workflows. It captures blocks, transactions, events, and state changes (accounts, validators, committees, pools, orders, parameters) into ClickHouse for efficient temporal and analytical queries.

The system automatically tracks multiple chains, detects gaps, and maintains data consistency through atomic two-phase commits.

## What It Does

- **Indexes blockchain data** - Blocks, transactions, events, and all state changes
- **Manages multiple chains** - Each chain gets isolated Temporal queue and indexer deployment
- **Optimizes storage** - Snapshot-on-change pattern reduces storage by 60-80%
- **Ensures consistency** - Two-phase commit guarantees atomic writes per block
- **Fills gaps automatically** - Detects and re-indexes missing blocks
- **Supports temporal queries** - Query any state at any historical height

## How It Works

### Components

**Admin Service** - Manages chains and provides admin UI (Next.js). Runs Temporal workflows for head scanning and gap detection.

**Indexer Worker** - Temporal worker that processes blocks for a specific chain. Each chain gets dedicated worker deployment.

**Controller** - Manages indexer deployments in Kubernetes (one deployment per chain).

### Data Flow

```
Canopy Node (RPC)
    ↓ Fetch block + state at height H and H-1
Temporal Workflow (per block)
    ↓ Compare H vs H-1, detect changes
ClickHouse (staging tables)
    ↓ Promote on success
ClickHouse (production tables)
```

### Indexing Process

1. Admin schedule workflow scans chain head for new blocks
2. Index workflow triggered for each block height
3. Fetch block data and current state (H) from RPC
4. Fetch previous state (H-1) from RPC
5. Compare states and detect changes (snapshot-on-change)
6. Insert changed entities to staging tables
7. Promote staging to production (atomic commit)
8. Clear staging tables

### Key Patterns

**Snapshot-on-Change** - Only insert database rows when entity state changes between heights. Applies to accounts, validators, committees, pools, orders, and parameters.

**RPC(H) vs RPC(H-1)** - Always fetch current and previous state from RPC, never from database. Enables stateless execution and gap filling.

**Two-Phase Commit** - Write to staging tables, then atomically promote to production. Ensures all-or-nothing per block.

**ReplacingMergeTree** - ClickHouse engine deduplicates by (entity_id, height), enabling efficient temporal queries.

## Quick Start

See **[DEV.md](docs/dev.md)** for complete setup instructions, prerequisites, and development workflows.

## Documentation

- **[docs/dev.md](docs/dev.md)** - Development setup, Make/Tilt/Kind guide, workflows
- **[docs/models.md](docs/models.md)** - Database models, entities, schema organization
- **[docs/rpc.md](docs/rpc.md)** - RPC client, height parameters, patterns

## Project Structure

```
cmd/                # Service entrypoints
  admin/            # Admin service + UI
  indexer/          # Indexer worker
  controller/       # Kubernetes controller

app/                # Service composition
  admin/            # Admin routes, workflows, activities
  indexer/          # Indexer workflows, activities
  controller/       # Controller logic

pkg/
  db/               # Database layer
    admin/          # Chain management database
    chain/          # Per-chain data database
    models/         # Go structs with ClickHouse tags
    entities/       # Type-safe query builders
  rpc/              # Canopy RPC client
  indexer/          # Indexer types and interfaces
  temporal/         # Temporal client wrapper

web/admin/          # Next.js admin UI
```

## Key Concepts

**Temporal Queries** - Query state at any height by selecting rows where `height <= target_height` ordered by height descending.

**Staging Tables** - All writes go to `*_staging` tables first, then atomically promoted to production on success.

**Snapshot-on-Change** - State entities only create new rows when values change, drastically reducing storage.

**Height Parameter** - All RPC methods accept `height` parameter for historical queries and gap filling.

**Entity System** - Type-safe query builders in `pkg/db/entities/` for complex temporal queries.

## Architecture Notes

### Why Temporal?

- Reliable execution with automatic retries
- Workflow durability across restarts
- Per-chain isolation via task queues
- Built-in observability

### Why ClickHouse?

- Excellent compression (10x+ vs PostgreSQL)
- Fast temporal queries (ORDER BY height)
- ReplacingMergeTree for automatic deduplication
- Column-oriented storage for analytics

### Why Snapshot-on-Change?

- 60-80% storage reduction
- Faster queries (fewer rows)
- Clear change history
- State changes are rare (most blocks don't change most entities)

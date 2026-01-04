# CanopyX Admin API Reference

<hr/>

## API Routes Reference

### System
- [GET /api/health](#health) - Health check endpoint

### Authentication
- [POST /api/auth/login](#admin-login) - Admin login
- [POST /api/auth/logout](#admin-logout) - Admin logout

### Chains Management
- [GET /api/chains](#list-chains) - List all registered chains
- [POST /api/chains](#register-chain) - Register a new chain
- [GET /api/chains/{id}](#get-chain) - Get chain details
- [PATCH /api/chains/{id}](#update-chain) - Partial update chain configuration
- [DELETE /api/chains/{id}](#delete-chain) - Delete a chain
- [GET /api/chains/{id}/schema](#get-chain-schema) - Get chain database schema
- [GET /api/chains/{id}/progress](#get-chain-progress) - Get indexing progress for chain
- [GET /api/chains/{id}/progress-history](#get-progress-history) - Get historical progress data
- [GET /api/chains/{id}/gaps](#get-chain-gaps) - List missing block ranges
- [POST /api/chains/{id}/reindex](#reindex-chain) - Trigger reindexing
- [POST /api/chains/{id}/headscan](#trigger-headscan) - Manually trigger head scan
- [POST /api/chains/{id}/gapscan](#trigger-gapscan) - Manually trigger gap scan
- [GET /api/chains/status](#get-all-chains-status) - Get status for all chains
- [PATCH /api/chains/status](#bulk-update-chains-status) - Bulk update chain status fields

### Chain Entities
- [GET /api/entities](#list-entities) - List available entity types
- [GET /api/chains/{id}/entity/{entity}](#query-chain-entity) - Query entity data for a chain
- [GET /api/chains/{id}/entity/{entity}/lookup](#lookup-entity) - Lookup entity by ID
- [GET /api/chains/{id}/entity/{entity}/schema](#get-entity-schema) - Get entity schema

### WebSocket
- [GET /api/ws](#websocket) - Real-time events via WebSocket

### Cross-Chain Queries
- [GET /api/crosschain/health](#crosschain-health) - Cross-chain database health
- [GET /api/crosschain/entities](#list-crosschain-entities) - List cross-chain entity types
- [GET /api/crosschain/entities/{entity}](#query-crosschain-entity) - Query cross-chain entity data
- [GET /api/crosschain/entities/{entity}/schema](#get-crosschain-entity-schema) - Get cross-chain entity schema
- [POST /api/crosschain/resync/{chainID}](#resync-chain-crosschain) - Resync all tables for a chain
- [POST /api/crosschain/resync/{chainID}/{table}](#resync-table-crosschain) - Resync specific table
- [GET /api/crosschain/sync-status/{chainID}/{table}](#get-sync-status) - Get sync status for table

### LP Position Snapshots
- [POST /api/chains/{id}/lp-schedule](#create-lp-snapshot-schedule) - Create LP snapshot schedule
- [POST /api/chains/{id}/lp-schedule/pause](#pause-lp-snapshot-schedule) - Pause LP snapshot schedule
- [POST /api/chains/{id}/lp-schedule/unpause](#unpause-lp-snapshot-schedule) - Unpause LP snapshot schedule
- [DELETE /api/chains/{id}/lp-schedule](#delete-lp-snapshot-schedule) - Delete LP snapshot schedule
- [POST /api/chains/{id}/lp-snapshots/backfill](#trigger-lp-snapshot-backfill) - Trigger LP snapshot backfill
- [GET /api/lp-snapshots](#query-lp-snapshots) - Query LP position snapshots

<hr/>

# System Endpoints

## Health

**Route**: `GET /api/health`

**Description**: Health check endpoint to verify the admin service is running

**Authentication**: None required

**Request**: None

**Response**:
```json
{
  "status": "ok",
  "timestamp": "2025-11-07T12:34:56Z"
}
```

**Example**:
```bash
curl http://localhost:3000/api/health
```

<hr/>

# Authentication

## Admin Login

**Route**: `POST /api/auth/login`

**Description**: Authenticate an admin user and establish a session

**Authentication**: None required

**Request**:
```json
{
  "username": "admin",
  "password": "admin"
}
```

**Response** (200 OK):
```json
{
  "ok": "1"
}
```

Sets a `cx_session` cookie with JWT token for subsequent authenticated requests.

**Error Response** (401 Unauthorized):
```json
{
  "error": "invalid credentials"
}
```

**Example**:
```bash
curl -X POST http://localhost:3000/api/auth/login \
  -H "Content-Type: application/json" \
  -c cookies.txt \
  -d '{"username": "admin", "password": "admin"}'
```

## Admin Logout

**Route**: `POST /api/auth/logout`

**Description**: Clear admin session and log out

**Authentication**: None required (clears existing session)

**Request**: None

**Response**: 204 No Content

**Example**:
```bash
curl -X POST http://localhost:3000/api/auth/logout \
  -b cookies.txt
```

<hr/>

# Chains Management

## List Chains

**Route**: `GET /api/chains`

**Description**: Returns a list of all registered blockchain chains

**Authentication**: Required (Bearer token)

**Request**: None

**Response**: Array of chain objects
```json
[
  {
    "chain_id": 1,
    "name": "Canopy Mainnet",
    "rpc_url": "http://node-1:50002",
    "created_at": "2025-11-07T12:00:00Z",
    "updated_at": "2025-11-07T12:00:00Z"
  }
]
```

**Example**:
```bash
curl -H "Authorization: Bearer devtoken" \
  http://localhost:3000/api/chains
```

## Register Chain

**Route**: `POST /api/chains`

**Description**: Register a new blockchain chain for indexing

**Authentication**: Required (Bearer token)

**Request**:
- **chain_id**: `uint64` - Unique chain identifier
- **name**: `string` - Human-readable chain name
- **rpc_url**: `string` - RPC endpoint URL

```json
{
  "chain_id": 2,
  "name": "Canopy Testnet",
  "rpc_url": "http://testnet:50002"
}
```

**Response**:
```json
{
  "chain_id": 2,
  "name": "Canopy Testnet",
  "rpc_url": "http://testnet:50002",
  "created_at": "2025-11-07T12:34:56Z",
  "updated_at": "2025-11-07T12:34:56Z"
}
```

**Example**:
```bash
curl -X POST http://localhost:3000/api/chains \
  -H "Authorization: Bearer devtoken" \
  -H "Content-Type: application/json" \
  -d '{
    "chain_id": 2,
    "name": "Canopy Testnet",
    "rpc_url": "http://testnet:50002"
  }'
```

## Get Chain

**Route**: `GET /api/chains/{id}`

**Description**: Get details for a specific chain

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID

**Response**:
```json
{
  "chain_id": 1,
  "name": "Canopy Mainnet",
  "rpc_url": "http://node-1:50002",
  "created_at": "2025-11-07T12:00:00Z",
  "updated_at": "2025-11-07T12:00:00Z"
}
```

**Example**:
```bash
curl -H "Authorization: Bearer devtoken" \
  http://localhost:3000/api/chains/1
```

## Update Chain

**Route**: `PATCH /api/chains/{id}`

**Description**: Partial update chain configuration

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID

**Request**:
```json
{
  "name": "Canopy Mainnet Updated",
  "rpc_url": "http://new-node:50002"
}
```

**Response**: Updated chain object

**Example**:
```bash
curl -X PATCH http://localhost:3000/api/chains/1 \
  -H "Authorization: Bearer devtoken" \
  -H "Content-Type: application/json" \
  -d '{"chain_name": "Canopy Mainnet Updated"}'
```

## Delete Chain

**Route**: `DELETE /api/chains/{id}`

**Description**: Delete a chain. Supports two modes:
- **Soft delete** (default): Marks chain as deleted, pauses schedules, terminates running workflows. Data is preserved for recovery.
- **Hard delete** (`?hard=true`): Permanently removes all chain data via async workflow. Cannot be undone.

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID

**Query Parameters**:
- **hard**: Set to `true` for permanent deletion (default: `false`)

### Soft Delete (default)

Performs the following operations synchronously:
1. Pauses all Temporal schedules (headscan, gapscan, snapshots, etc.)
2. Terminates all running workflows (batch operation)
3. Marks chain as `deleted=1` in database
4. Clears health status to "unknown"
5. Clears in-memory caches

**Response** (Soft Delete):
```json
{
  "ok": "1"
}
```

### Hard Delete (?hard=true)

Performs the following operations:
1. Deletes the chain's Temporal namespace (removes all workflows and schedules instantly)
2. Clears in-memory caches
3. Starts async `DeleteChainWorkflow` in admin namespace to clean all data

The workflow runs 3 parallel activities:
- **CleanCrossChainData**: Removes data from global cross-chain tables
- **DropChainDatabase**: Drops the `chain_X` database
- **CleanAdminTables**: Removes records from admin tables (endpoints, index_progress, reindex_requests, chain record)

**Response** (Hard Delete):
```json
{
  "status": "deletion_started",
  "workflow_id": "delete-chain:1"
}
```

The workflow can be tracked in the Temporal UI under the `canopyx` namespace.

**Error Response** (500 Internal Server Error):
```json
{
  "error": "failed to start delete workflow"
}
```

**Examples**:
```bash
# Soft delete (recoverable)
curl -X DELETE http://localhost:3000/api/chains/1 \
  -H "Authorization: Bearer devtoken"

# Hard delete (permanent)
curl -X DELETE "http://localhost:3000/api/chains/1?hard=true" \
  -H "Authorization: Bearer devtoken"
```

## Recover Chain

**Route**: `POST /api/chains/{id}/recover`

**Description**: Restore a soft-deleted chain. Unpauses schedules and re-enables indexing.

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID

**Response**:
```json
{
  "ok": "1"
}
```

**Note**: Only works for soft-deleted chains. Hard-deleted chains cannot be recovered.

**Example**:
```bash
curl -X POST http://localhost:3000/api/chains/1/recover \
  -H "Authorization: Bearer devtoken"
```

## Get Chain Schema

**Route**: `GET /api/chains/{id}/schema`

**Description**: Get the ClickHouse database schema for a chain

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID

**Response**:
```json
{
  "database": "canopyx_chain_1",
  "tables": [
    {
      "name": "accounts",
      "columns": ["address", "amount", "height", "height_time"],
      "engine": "ReplacingMergeTree",
      "order_by": ["address", "height"]
    }
  ]
}
```

**Example**:
```bash
curl -H "Authorization: Bearer devtoken" \
  http://localhost:3000/api/chains/1/schema
```

## Get Chain Progress

**Route**: `GET /api/chains/{id}/progress`

**Description**: Get current indexing progress for a chain

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID

**Response**:
```json
{
  "chain_id": 1,
  "latest_indexed_height": 12345,
  "chain_height": 12350,
  "behind": 5,
  "progress_percentage": 99.96,
  "indexing_rate": 10.5,
  "estimated_catchup_time": "30s"
}
```

**Example**:
```bash
curl -H "Authorization: Bearer devtoken" \
  http://localhost:3000/api/chains/1/progress
```

## Get Progress History

**Route**: `GET /api/chains/{id}/progress-history`

**Description**: Get historical indexing progress data

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID

**Query Parameters**:
- **hours**: Number of hours of history (default: 24)

**Response**:
```json
{
  "chain_id": 1,
  "data_points": [
    {
      "timestamp": "2025-11-07T12:00:00Z",
      "indexed_height": 12000,
      "chain_height": 12005,
      "rate": 10.2
    }
  ]
}
```

**Example**:
```bash
curl -H "Authorization: Bearer devtoken" \
  "http://localhost:3000/api/chains/1/progress-history?hours=48"
```

## Get Chain Gaps

**Route**: `GET /api/chains/{id}/gaps`

**Description**: List missing block ranges (gaps) in indexed data

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID

**Response**:
```json
{
  "chain_id": 1,
  "gaps": [
    {
      "start_height": 100,
      "end_height": 150,
      "size": 51
    }
  ],
  "total_gaps": 1,
  "total_missing_blocks": 51
}
```

**Example**:
```bash
curl -H "Authorization: Bearer devtoken" \
  http://localhost:3000/api/chains/1/gaps
```

## Reindex Chain

**Route**: `POST /api/chains/{id}/reindex`

**Description**: Trigger full reindexing of a chain from genesis

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID

**Request**:
```json
{
  "start_height": 0,
  "end_height": 10000
}
```

**Response**:
```json
{
  "message": "Reindexing started",
  "workflow_id": "reindex-chain-1-abc123"
}
```

**Example**:
```bash
curl -X POST http://localhost:3000/api/chains/1/reindex \
  -H "Authorization: Bearer devtoken" \
  -H "Content-Type: application/json" \
  -d '{
    "start_height": 0,
    "end_height": 10000
  }'
```

## Trigger Headscan

**Route**: `POST /api/chains/{id}/headscan`

**Description**: Manually trigger a head scan workflow for the chain

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID

**Response**:
```json
{
  "message": "Head scan triggered",
  "workflow_id": "headscan-chain-1-def456"
}
```

**Example**:
```bash
curl -X POST http://localhost:3000/api/chains/1/headscan \
  -H "Authorization: Bearer devtoken"
```

## Trigger Gapscan

**Route**: `POST /api/chains/{id}/gapscan`

**Description**: Manually trigger a gap scan workflow to find missing blocks

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID

**Response**:
```json
{
  "message": "Gap scan triggered",
  "workflow_id": "gapscan-chain-1-ghi789"
}
```

**Example**:
```bash
curl -X POST http://localhost:3000/api/chains/1/gapscan \
  -H "Authorization: Bearer devtoken"
```

## Get All Chains Status

**Route**: `GET /api/chains/status`

**Description**: Get status summary for all registered chains including per-endpoint health data

**Authentication**: Required (Bearer token)

**Response**: Map of chain IDs to chain status objects
```json
{
  "1": {
    "chain_id": 1,
    "chain_name": "Canopy Mainnet",
    "image": "ghcr.io/canopy-network/canopyx-indexer:latest",
    "notes": "",
    "paused": false,
    "deleted": false,
    "min_replicas": 1,
    "max_replicas": 3,
    "last_indexed": 12345,
    "head": 12350,
    "queue": {
      "pending_workflow": 2,
      "pending_activity": 5,
      "backlog_age_secs": 0.5,
      "pollers": 4
    },
    "ops_queue": {
      "pending_workflow": 0,
      "pending_activity": 1,
      "backlog_age_secs": 0.0,
      "pollers": 2
    },
    "indexer_queue": {
      "pending_workflow": 2,
      "pending_activity": 5,
      "backlog_age_secs": 0.5,
      "pollers": 4
    },
    "missing_blocks_count": 0,
    "gap_ranges_count": 0,
    "largest_gap_start": 0,
    "largest_gap_end": 0,
    "is_live_sync": true,
    "live_queue_depth": 2,
    "live_queue_backlog_age": 0.3,
    "historical_queue_depth": 5,
    "historical_queue_backlog_age": 1.2,
    "health": {
      "status": "healthy",
      "message": "",
      "updated_at": "2025-11-07T12:34:56Z"
    },
    "rpc_health": {
      "status": "healthy",
      "message": "3/3 endpoints healthy, max height 12350",
      "updated_at": "2025-11-07T12:34:56Z"
    },
    "queue_health": {
      "status": "healthy",
      "message": "backlog within threshold",
      "updated_at": "2025-11-07T12:34:56Z"
    },
    "deployment_health": {
      "status": "healthy",
      "message": "2/2 replicas ready",
      "updated_at": "2025-11-07T12:34:56Z"
    },
    "endpoints": [
      {
        "endpoint": "http://node-1:50002",
        "status": "healthy",
        "height": 12350,
        "latency_ms": 45.2,
        "updated_at": "2025-11-07T12:34:56Z"
      },
      {
        "endpoint": "http://node-2:50002",
        "status": "healthy",
        "height": 12348,
        "latency_ms": 62.1,
        "updated_at": "2025-11-07T12:34:56Z"
      },
      {
        "endpoint": "http://node-3:50002",
        "status": "unreachable",
        "height": 0,
        "latency_ms": 0,
        "error": "connection refused",
        "updated_at": "2025-11-07T12:34:56Z"
      }
    ],
    "reindex_history": []
  }
}
```

**Response Fields**:
- **chain_id**: Unique chain identifier
- **chain_name**: Human-readable chain name
- **image**: Docker image used for indexer workers
- **paused**: Whether indexing is paused
- **deleted**: Whether chain is soft-deleted
- **min_replicas/max_replicas**: Scaling configuration
- **last_indexed**: Last successfully indexed block height
- **head**: Current chain head from RPC
- **queue/ops_queue/indexer_queue**: Temporal queue metrics
- **missing_blocks_count**: Total missing blocks across all gaps
- **gap_ranges_count**: Number of contiguous gap ranges
- **is_live_sync**: True if within 2 blocks of chain head
- **health/rpc_health/queue_health/deployment_health**: Health status for subsystems
- **endpoints**: Per-endpoint health data (see below)
- **reindex_history**: Recent reindex requests

**Endpoint Health Object**:
- **endpoint**: RPC endpoint URL
- **status**: `healthy`, `unreachable`, or `degraded`
- **height**: Last known block height from this endpoint
- **latency_ms**: Response time in milliseconds
- **error**: Error message if unhealthy (omitted if healthy)
- **updated_at**: When this endpoint was last checked

**Example**:
```bash
curl -H "Authorization: Bearer devtoken" \
  http://localhost:3000/api/chains/status
```

## Bulk Update Chains Status

**Route**: `PATCH /api/chains/status`

**Description**: Bulk update status fields for multiple chains in a single request

**Authentication**: Required (Bearer token or session cookie)

**Request**: Array of chain objects with partial fields to update
```json
[
  {
    "chain_id": 1,
    "paused": 1,
    "rpc_endpoints": ["http://node-1:50002", "http://node-2:50002"]
  },
  {
    "chain_id": 2,
    "min_replicas": 2,
    "max_replicas": 4
  }
]
```

**Response**: 204 No Content

**Error Response** (400 Bad Request):
```
chain_id is required
```

**Example**:
```bash
curl -X PATCH http://localhost:3000/api/chains/status \
  -H "Authorization: Bearer devtoken" \
  -H "Content-Type: application/json" \
  -d '[
    {"chain_id": 1, "paused": 1},
    {"chain_id": 2, "paused": 0}
  ]'
```

<hr/>

# Chain Entities

## List Entities

**Route**: `GET /api/entities`

**Description**: List all available entity types that can be queried

**Authentication**: Required (Bearer token)

**Response**:
```json
{
  "entities": [
    "accounts",
    "validators",
    "pools",
    "orders",
    "dex_orders",
    "block_summaries"
  ]
}
```

**Example**:
```bash
curl -H "Authorization: Bearer devtoken" \
  http://localhost:3000/api/entities
```

## Query Chain Entity

**Route**: `GET /api/chains/{id}/entity/{entity}`

**Description**: Query entity data for a specific chain with filtering and pagination

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID
- **entity**: Entity type (e.g., "accounts", "validators")

**Query Parameters**:
- **page**: Page number (default: 1)
- **limit**: Items per page (default: 10, max: 1000)
- **order_by**: Field to sort by
- **order_dir**: Sort direction ("asc" or "desc")
- **filters**: JSON object with field filters

**Response**:
```json
{
  "entity": "accounts",
  "chain_id": 1,
  "data": [
    {
      "address": "851e90eaef1fa27debaee2c2591503bdeec1d123",
      "amount": 1000000,
      "height": 12345,
      "height_time": "2025-11-07T12:34:56Z"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 10,
    "total": 150,
    "total_pages": 15
  }
}
```

**Example**:
```bash
# Basic query
curl -H "Authorization: Bearer devtoken" \
  "http://localhost:3000/api/chains/1/entity/accounts?limit=20"

# With filters
curl -H "Authorization: Bearer devtoken" \
  "http://localhost:3000/api/chains/1/entity/accounts?filters={\"amount\":{\"gt\":1000000}}"
```

## Lookup Entity

**Route**: `GET /api/chains/{id}/entity/{entity}/lookup`

**Description**: Lookup a specific entity by its unique identifier

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID
- **entity**: Entity type

**Query Parameters**:
- Varies by entity type (e.g., **address** for accounts, **pool_id** for pools)

**Response**: Single entity object

**Example**:
```bash
curl -H "Authorization: Bearer devtoken" \
  "http://localhost:3000/api/chains/1/entity/accounts/lookup?address=851e90eaef1fa27debaee2c2591503bdeec1d123"
```

## Get Entity Schema

**Route**: `GET /api/chains/{id}/entity/{entity}/schema`

**Description**: Get the schema definition for an entity type

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID
- **entity**: Entity type

**Response**:
```json
{
  "entity": "accounts",
  "chain_id": 1,
  "fields": [
    {
      "name": "address",
      "type": "String",
      "description": "Account address (hex)",
      "filterable": true,
      "sortable": false
    },
    {
      "name": "amount",
      "type": "UInt64",
      "description": "Account balance in micro denomination",
      "filterable": true,
      "sortable": true
    }
  ]
}
```

**Example**:
```bash
curl -H "Authorization: Bearer devtoken" \
  http://localhost:3000/api/chains/1/entity/accounts/schema
```

<hr/>

# WebSocket

## Real-Time Events

**Route**: `GET /api/ws`

**Description**: WebSocket connection for real-time blockchain indexing events. Requires Redis to be enabled.

**Authentication**: None required

**Protocol**:

Client messages:
```json
// Subscribe to specific chain
{"action": "subscribe", "chainId": "1"}

// Subscribe to ALL chains
{"action": "subscribe", "chainId": "*"}

// Unsubscribe from chain
{"action": "unsubscribe", "chainId": "1"}
```

Server messages:
```json
// Block indexed event
{
  "type": "block.indexed",
  "payload": {
    "chain_id": 1,
    "height": 12345,
    "timestamp": "2025-11-07T12:34:56Z"
  }
}

// Subscription confirmed
{
  "type": "subscribed",
  "payload": {"chainId": "1"}
}

// Unsubscription confirmed
{
  "type": "unsubscribed",
  "payload": {"chainId": "1"}
}

// Keep-alive ping
{
  "type": "ping",
  "payload": {"timestamp": 1699356896}
}

// Error
{
  "type": "error",
  "payload": {"message": "chainId is required"}
}
```

**Error Response** (503 Service Unavailable):
```
Real-time events not available (Redis disabled)
```

**Example** (using wscat):
```bash
wscat -c ws://localhost:3000/api/ws
> {"action": "subscribe", "chainId": "*"}
< {"type": "subscribed", "payload": {"chainId": "*"}}
< {"type": "block.indexed", "payload": {...}}
```

<hr/>

# Cross-Chain Queries

## Crosschain Health

**Route**: `GET /api/crosschain/health`

**Description**: Check cross-chain database health and connectivity

**Authentication**: Required (Bearer token)

**Response**:
```json
{
  "status": "healthy",
  "database": "canopyx_cross_chain",
  "tables": 11,
  "chains_synced": 2,
  "last_updated": "2025-11-07T12:34:56Z"
}
```

**Example**:
```bash
curl -H "Authorization: Bearer devtoken" \
  http://localhost:3000/api/crosschain/health
```

## List Crosschain Entities

**Route**: `GET /api/crosschain/entities`

**Description**: List entity types available for cross-chain queries

**Authentication**: Required (Bearer token)

**Response**:
```json
{
  "entities": [
    "accounts",
    "validators",
    "pools",
    "pool_points_by_holder",
    "orders",
    "dex_orders",
    "dex_deposits",
    "dex_withdrawals",
    "validator_signing_info",
    "validator_double_signing_info",
    "block_summaries"
  ]
}
```

**Example**:
```bash
curl -H "Authorization: Bearer devtoken" \
  http://localhost:3000/api/crosschain/entities
```

## Query Crosschain Entity

**Route**: `GET /api/crosschain/entities/{entity}`

**Description**: Query entity data across all chains with filtering and pagination

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **entity**: Entity type

**Query Parameters**:
- **page**: Page number (default: 1)
- **limit**: Items per page (default: 10, max: 1000)
- **order_by**: Field to sort by
- **order_dir**: Sort direction ("asc" or "desc")
- **filters**: JSON object with field filters (can filter by chain_id)

**Response**:
```json
{
  "entity": "accounts",
  "data": [
    {
      "chain_id": 1,
      "address": "851e90eaef1fa27debaee2c2591503bdeec1d123",
      "amount": 980213903,
      "height": 19,
      "height_time": "2025-11-07T12:34:56Z",
      "updated_at": "2025-11-07T12:35:00Z"
    },
    {
      "chain_id": 2,
      "address": "851e90eaef1fa27debaee2c2591503bdeec1d123",
      "amount": 461833030,
      "height": 18,
      "height_time": "2025-11-07T12:34:50Z",
      "updated_at": "2025-11-07T12:35:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 10,
    "total": 2,
    "total_pages": 1
  }
}
```

**Example**:
```bash
# Query specific address across all chains
curl -H "Authorization: Bearer devtoken" \
  "http://localhost:3000/api/crosschain/entities/accounts?filters={\"address\":\"851e90eaef1fa27debaee2c2591503bdeec1d123\"}"

# Query specific chain
curl -H "Authorization: Bearer devtoken" \
  "http://localhost:3000/api/crosschain/entities/accounts?filters={\"chain_id\":1}"
```

## Get Crosschain Entity Schema

**Route**: `GET /api/crosschain/entities/{entity}/schema`

**Description**: Get the schema definition for a cross-chain entity type

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **entity**: Entity type

**Response**:
```json
{
  "entity": "accounts",
  "fields": [
    {
      "name": "chain_id",
      "type": "UInt64",
      "description": "Chain identifier",
      "filterable": true,
      "sortable": true
    },
    {
      "name": "address",
      "type": "String",
      "description": "Account address (hex)",
      "filterable": true,
      "sortable": false
    },
    {
      "name": "amount",
      "type": "UInt64",
      "description": "Account balance",
      "filterable": true,
      "sortable": true
    },
    {
      "name": "height",
      "type": "UInt64",
      "description": "Block height of latest state",
      "filterable": true,
      "sortable": true
    }
  ]
}
```

**Example**:
```bash
curl -H "Authorization: Bearer devtoken" \
  http://localhost:3000/api/crosschain/entities/accounts/schema
```

## Resync Chain Crosschain

**Route**: `POST /api/crosschain/resync/{chainID}`

**Description**: Trigger resync of all cross-chain tables for a specific chain

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **chainID**: Chain ID

**Response**:
```json
{
  "message": "Resync started for all tables",
  "chain_id": 1,
  "tables": 11,
  "estimated_duration": "5m"
}
```

**Example**:
```bash
curl -X POST http://localhost:3000/api/crosschain/resync/1 \
  -H "Authorization: Bearer devtoken"
```

## Resync Table Crosschain

**Route**: `POST /api/crosschain/resync/{chainID}/{table}`

**Description**: Trigger resync of a specific cross-chain table for a chain

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **chainID**: Chain ID
- **table**: Table name (e.g., "accounts", "validators")

**Response**:
```json
{
  "message": "Resync started",
  "chain_id": 1,
  "table": "accounts",
  "estimated_duration": "30s"
}
```

**Example**:
```bash
curl -X POST http://localhost:3000/api/crosschain/resync/1/accounts \
  -H "Authorization: Bearer devtoken"
```

## Get Sync Status

**Route**: `GET /api/crosschain/sync-status/{chainID}/{table}`

**Description**: Get synchronization status for a specific table

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **chainID**: Chain ID
- **table**: Table name

**Response**:
```json
{
  "chain_id": 1,
  "table": "accounts",
  "status": "synced",
  "last_sync": "2025-11-07T12:34:56Z",
  "records_synced": 150,
  "materialized_view_status": "running"
}
```

**Example**:
```bash
curl -H "Authorization: Bearer devtoken" \
  http://localhost:3000/api/crosschain/sync-status/1/accounts
```

<hr/>

# LP Position Snapshots

LP Position Snapshots capture daily snapshots of liquidity provider positions across all chains. These snapshots are computed via scheduled Temporal workflows that run hourly per chain.

## Create LP Snapshot Schedule

**Route**: `POST /api/chains/{id}/lp-schedule`

**Description**: Create an hourly LP snapshot schedule for a specific chain. Optionally trigger a backfill for historical data.

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID

**Request**:
```json
{
  "backfill": {
    "start": "2024-01-01",
    "end": "2024-12-31"
  }
}
```

**Request Fields**:
- **backfill**: (Optional) Backfill configuration
  - **start**: Start date (UTC, format: YYYY-MM-DD)
  - **end**: End date (UTC, format: YYYY-MM-DD)
  - If omitted, automatically calculates from block 1 to current indexed height

**Response** (200 OK):
```json
{
  "message": "LP snapshot schedule created successfully",
  "chain_id": 1,
  "schedule_id": "chain:1:lpsnapshot",
  "backfill_triggered": true,
  "backfill_start": "2024-01-01T00:00:00Z",
  "backfill_end": "2024-12-31T00:00:00Z"
}
```

**Response** (without backfill):
```json
{
  "message": "LP snapshot schedule created successfully",
  "chain_id": 1,
  "schedule_id": "chain:1:lpsnapshot",
  "backfill_triggered": false
}
```

**Error Response** (400 Bad Request):
```json
{
  "error": "schedule already exists"
}
```

**Example**:
```bash
# Create schedule with automatic backfill range
curl -X POST http://localhost:3000/api/chains/1/lp-schedule \
  -H "Authorization: Bearer devtoken" \
  -H "Content-Type: application/json" \
  -d '{}'

# Create schedule with specific backfill dates
curl -X POST http://localhost:3000/api/chains/1/lp-schedule \
  -H "Authorization: Bearer devtoken" \
  -H "Content-Type: application/json" \
  -d '{
    "backfill": {
      "start": "2024-01-01",
      "end": "2024-12-31"
    }
  }'
```

## Pause LP Snapshot Schedule

**Route**: `POST /api/chains/{id}/lp-schedule/pause`

**Description**: Pause an existing LP snapshot schedule for a chain

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID

**Request**: None

**Response**:
```json
{
  "message": "LP snapshot schedule paused successfully",
  "chain_id": 1,
  "schedule_id": "chain:1:lpsnapshot"
}
```

**Error Response** (404 Not Found):
```json
{
  "error": "schedule not found"
}
```

**Example**:
```bash
curl -X POST http://localhost:3000/api/chains/1/lp-schedule/pause \
  -H "Authorization: Bearer devtoken"
```

## Unpause LP Snapshot Schedule

**Route**: `POST /api/chains/{id}/lp-schedule/unpause`

**Description**: Unpause a paused LP snapshot schedule. Optionally trigger a backfill to catch up on missed snapshots.

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID

**Request**:
```json
{
  "backfill": {
    "start": "2024-06-01",
    "end": "2024-12-31"
  }
}
```

**Request Fields**:
- **backfill**: (Optional) Backfill configuration
  - **start**: Start date (UTC, format: YYYY-MM-DD)
  - **end**: End date (UTC, format: YYYY-MM-DD)
  - If omitted, automatically calculates from block 1 to current indexed height

**Response** (with backfill):
```json
{
  "message": "LP snapshot schedule unpaused successfully",
  "chain_id": 1,
  "schedule_id": "chain:1:lpsnapshot",
  "backfill_triggered": true,
  "backfill_start": "2024-06-01T00:00:00Z",
  "backfill_end": "2024-12-31T00:00:00Z"
}
```

**Response** (without backfill):
```json
{
  "message": "LP snapshot schedule unpaused successfully",
  "chain_id": 1,
  "schedule_id": "chain:1:lpsnapshot",
  "backfill_triggered": false
}
```

**Example**:
```bash
# Unpause without backfill
curl -X POST http://localhost:3000/api/chains/1/lp-schedule/unpause \
  -H "Authorization: Bearer devtoken"

# Unpause with backfill
curl -X POST http://localhost:3000/api/chains/1/lp-schedule/unpause \
  -H "Authorization: Bearer devtoken" \
  -H "Content-Type: application/json" \
  -d '{
    "backfill": {
      "start": "2024-06-01",
      "end": "2024-12-31"
    }
  }'
```

## Delete LP Snapshot Schedule

**Route**: `DELETE /api/chains/{id}/lp-schedule`

**Description**: Delete an LP snapshot schedule for a chain. Stops future snapshot computation.

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID

**Request**: None

**Response**:
```json
{
  "message": "LP snapshot schedule deleted successfully",
  "chain_id": 1,
  "schedule_id": "chain:1:lpsnapshot"
}
```

**Error Response** (404 Not Found):
```json
{
  "error": "schedule not found"
}
```

**Example**:
```bash
curl -X DELETE http://localhost:3000/api/chains/1/lp-schedule \
  -H "Authorization: Bearer devtoken"
```

## Trigger LP Snapshot Backfill

**Route**: `POST /api/chains/{id}/lp-snapshots/backfill`

**Description**: Trigger a backfill for LP position snapshots for a specific date range. This endpoint can be used independently of schedule creation.

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID

**Request**:
```json
{
  "start": "2024-01-01",
  "end": "2024-12-31"
}
```

**Request Fields**:
- **start**: (Optional) Start date (UTC, format: YYYY-MM-DD). If omitted, uses block 1 timestamp
- **end**: (Optional) End date (UTC, format: YYYY-MM-DD). If omitted, uses current indexed height timestamp

**Response**:
```json
{
  "message": "LP snapshot backfill triggered successfully",
  "chain_id": 1,
  "schedule_id": "chain:1:lpsnapshot",
  "start_date": "2024-01-01T00:00:00Z",
  "end_date": "2024-12-31T00:00:00Z",
  "estimated_days": 365
}
```

**Error Response** (400 Bad Request):
```json
{
  "error": "invalid date format, use YYYY-MM-DD"
}
```

**Error Response** (404 Not Found):
```json
{
  "error": "schedule not found, create schedule first"
}
```

**Example**:
```bash
# Backfill with specific dates
curl -X POST http://localhost:3000/api/chains/1/lp-snapshots/backfill \
  -H "Authorization: Bearer devtoken" \
  -H "Content-Type: application/json" \
  -d '{
    "start": "2024-01-01",
    "end": "2024-12-31"
  }'

# Backfill with automatic range calculation
curl -X POST http://localhost:3000/api/chains/1/lp-snapshots/backfill \
  -H "Authorization: Bearer devtoken" \
  -H "Content-Type: application/json" \
  -d '{}'
```

## Query LP Snapshots

**Route**: `GET /api/lp-snapshots`

**Description**: Query LP position snapshots with filtering and pagination. Returns snapshots across all chains.

**Authentication**: Required (Bearer token)

**Query Parameters**:
- **source_chain_id**: Filter by source chain ID
- **address**: Filter by LP holder address
- **pool_id**: Filter by pool ID
- **start_date**: Filter snapshots >= this date (format: YYYY-MM-DD)
- **end_date**: Filter snapshots <= this date (format: YYYY-MM-DD)
- **active_only**: Only return active positions (true/false)
- **limit**: Maximum number of results (default: 100, max: 1000)
- **offset**: Pagination offset (default: 0)

**Response**:
```json
{
  "snapshots": [
    {
      "source_chain_id": 1,
      "address": "851e90eaef1fa27debaee2c2591503bdeec1d123",
      "pool_id": 42,
      "snapshot_date": "2024-12-01",
      "snapshot_height": 123456,
      "snapshot_balance": 15000000,
      "pool_share_percentage": 25464210,
      "position_created_date": "2024-11-15",
      "position_closed_date": null,
      "is_position_active": 1,
      "computed_at": "2024-12-01T01:05:23.456789Z",
      "updated_at": "2024-12-01T01:05:23.456789Z"
    },
    {
      "source_chain_id": 1,
      "address": "851e90eaef1fa27debaee2c2591503bdeec1d123",
      "pool_id": 42,
      "snapshot_date": "2024-11-30",
      "snapshot_height": 123000,
      "snapshot_balance": 14800000,
      "pool_share_percentage": 24832105,
      "position_created_date": "2024-11-15",
      "position_closed_date": null,
      "is_position_active": 1,
      "computed_at": "2024-11-30T01:03:12.123456Z",
      "updated_at": "2024-11-30T01:03:12.123456Z"
    }
  ],
  "total": 2,
  "limit": 100,
  "offset": 0
}
```

**Response Fields**:
- **source_chain_id**: Chain where LP position exists
- **address**: LP holder address
- **pool_id**: Liquidity pool identifier
- **snapshot_date**: Calendar date of snapshot (UTC)
- **snapshot_height**: Block height at snapshot time (highest block <= 23:59:59 UTC)
- **snapshot_balance**: LP position balance at snapshot time (pre-calculated)
- **pool_share_percentage**: Percentage of pool owned (6 decimal precision)
  - Example: 25464210 = 25.464210%
  - Formula: (points / total_points) * 100 * 1000000
- **position_created_date**: Date when position was first created
- **position_closed_date**: Date when position was closed (null if active)
- **is_position_active**: 1 if position has points > 0, 0 if closed
- **computed_at**: When this snapshot was computed
- **updated_at**: When this snapshot was last updated (for deduplication)

**Example**:
```bash
# Query specific address on specific chain
curl -H "Authorization: Bearer devtoken" \
  "http://localhost:3000/api/lp-snapshots?source_chain_id=1&address=851e90eaef1fa27debaee2c2591503bdeec1d123"

# Query specific pool with date range
curl -H "Authorization: Bearer devtoken" \
  "http://localhost:3000/api/lp-snapshots?pool_id=42&start_date=2024-11-01&end_date=2024-12-31"

# Query only active positions
curl -H "Authorization: Bearer devtoken" \
  "http://localhost:3000/api/lp-snapshots?active_only=true&limit=50"

# Query with pagination
curl -H "Authorization: Bearer devtoken" \
  "http://localhost:3000/api/lp-snapshots?limit=100&offset=100"
```

<hr/>

# Authentication Methods

The Admin API supports two authentication methods:

## 1. Bearer Token (Header)
Include a Bearer token in the Authorization header:
```bash
Authorization: Bearer <your-token>
```

For development, the default token is `devtoken`. In production, tokens should be generated securely and rotated regularly.

## 2. Session Cookie (Browser)
Use the `/api/auth/login` endpoint to establish a session. The server sets a `cx_session` HTTP-only cookie that will be automatically sent with subsequent requests.

**Endpoints not requiring authentication:**
- `GET /api/health`
- `POST /api/auth/login`
- `POST /api/auth/logout`
- `GET /api/ws`

# Query Filtering

The entity query endpoints support flexible filtering using JSON filter objects:

## Filter Operators

- **eq**: Equals (default if no operator specified)
- **ne**: Not equals
- **gt**: Greater than
- **gte**: Greater than or equal
- **lt**: Less than
- **lte**: Less than or equal
- **in**: Value in array
- **like**: Pattern matching (use % as wildcard)

## Filter Examples

```json
// Amount greater than 1 million
{"amount": {"gt": 1000000}}

// Specific chain
{"chain_id": 1}

// Address pattern matching
{"address": {"like": "851e%"}}

// Multiple conditions (AND)
{"chain_id": 1, "amount": {"gte": 100000}}

// IN operator
{"chain_id": {"in": [1, 2, 3]}}
```

# Pagination

All list endpoints support pagination:

- **page**: Page number (1-indexed)
- **limit**: Items per page (default: 10, max: 1000)
- **total**: Total number of items
- **total_pages**: Total number of pages

# Sorting

Entity queries support sorting:

- **order_by**: Field name to sort by
- **order_dir**: Sort direction ("asc" or "desc")

Example:
```
?order_by=height&order_dir=desc
```

# Error Responses

All endpoints return standard HTTP status codes and JSON error responses:

```json
{
  "error": "Error message description",
  "code": "ERROR_CODE",
  "details": {}
}
```

Common status codes:
- **200**: Success
- **400**: Bad Request (invalid parameters)
- **401**: Unauthorized (missing/invalid token)
- **404**: Not Found (resource doesn't exist)
- **500**: Internal Server Error
- **503**: Service Unavailable (database connection issues)

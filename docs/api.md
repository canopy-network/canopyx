# CanopyX Admin API Reference

<hr/>

## API Routes Reference

### System
- [GET /api/health](#health) - Health check endpoint

### Chains Management
- [GET /api/chains](#list-chains) - List all registered chains
- [POST /api/chains](#register-chain) - Register a new chain
- [GET /api/chains/{id}](#get-chain) - Get chain details
- [PUT /api/chains/{id}](#update-chain) - Update chain configuration
- [DELETE /api/chains/{id}](#delete-chain) - Delete a chain
- [GET /api/chains/{id}/schema](#get-chain-schema) - Get chain database schema
- [GET /api/chains/{id}/progress](#get-chain-progress) - Get indexing progress for chain
- [GET /api/chains/{id}/progress-history](#get-progress-history) - Get historical progress data
- [GET /api/chains/{id}/gaps](#get-chain-gaps) - List missing block ranges
- [POST /api/chains/{id}/reindex](#reindex-chain) - Trigger reindexing
- [POST /api/chains/{id}/headscan](#trigger-headscan) - Manually trigger head scan
- [POST /api/chains/{id}/gapscan](#trigger-gapscan) - Manually trigger gap scan
- [GET /api/chains/status](#get-all-chains-status) - Get status for all chains
- [POST /api/chains/status](#bulk-get-chains-status) - Bulk status check with filters

### Chain Entities
- [GET /api/entities](#list-entities) - List available entity types
- [GET /api/chains/{id}/entity/{entity}](#query-chain-entity) - Query entity data for a chain
- [GET /api/chains/{id}/entity/{entity}/lookup](#lookup-entity) - Lookup entity by ID
- [GET /api/chains/{id}/entity/{entity}/schema](#get-entity-schema) - Get entity schema

### Cross-Chain Queries
- [GET /api/crosschain/health](#crosschain-health) - Cross-chain database health
- [GET /api/crosschain/entities](#list-crosschain-entities) - List cross-chain entity types
- [GET /api/crosschain/entities/{entity}](#query-crosschain-entity) - Query cross-chain entity data
- [GET /api/crosschain/entities/{entity}/schema](#get-crosschain-entity-schema) - Get cross-chain entity schema
- [POST /api/crosschain/resync/{chainID}](#resync-chain-crosschain) - Resync all tables for a chain
- [POST /api/crosschain/resync/{chainID}/{table}](#resync-table-crosschain) - Resync specific table
- [GET /api/crosschain/sync-status/{chainID}/{table}](#get-sync-status) - Get sync status for table

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

**Route**: `PUT /api/chains/{id}`

**Description**: Update chain configuration

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
curl -X PUT http://localhost:3000/api/chains/1 \
  -H "Authorization: Bearer devtoken" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Canopy Mainnet Updated",
    "rpc_url": "http://new-node:50002"
  }'
```

## Delete Chain

**Route**: `DELETE /api/chains/{id}`

**Description**: Delete a chain and stop indexing it

**Authentication**: Required (Bearer token)

**Path Parameters**:
- **id**: Chain ID

**Response**:
```json
{
  "message": "Chain deleted successfully",
  "chain_id": 1
}
```

**Example**:
```bash
curl -X DELETE http://localhost:3000/api/chains/1 \
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

**Description**: Get status summary for all registered chains

**Authentication**: Required (Bearer token)

**Response**:
```json
[
  {
    "chain_id": 1,
    "name": "Canopy Mainnet",
    "status": "syncing",
    "latest_indexed_height": 12345,
    "chain_height": 12350,
    "behind": 5
  }
]
```

**Example**:
```bash
curl -H "Authorization: Bearer devtoken" \
  http://localhost:3000/api/chains/status
```

## Bulk Get Chains Status

**Route**: `POST /api/chains/status`

**Description**: Get status for multiple chains with optional filters

**Authentication**: Required (Bearer token)

**Request**:
```json
{
  "chain_ids": [1, 2],
  "include_progress": true
}
```

**Response**: Array of detailed chain status objects

**Example**:
```bash
curl -X POST http://localhost:3000/api/chains/status \
  -H "Authorization: Bearer devtoken" \
  -H "Content-Type: application/json" \
  -d '{"chain_ids": [1, 2]}'
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

# Authentication

All API endpoints (except `/api/health`) require authentication using a Bearer token in the Authorization header:

```bash
Authorization: Bearer <your-token>
```

For development, the default token is `devtoken`. In production, tokens should be generated securely and rotated regularly.

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

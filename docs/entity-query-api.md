# Entity Query API

The Entity Query API provides a generic, type-safe mechanism to query any indexed blockchain entity through the admin service. This API supports pagination, staging vs production table selection, and both list and single-entity queries.

## Overview

The API exposes two main endpoints:

1. **List Query**: Paginated queries of entities with cursor-based navigation
2. **Single Entity Lookup**: Retrieve a specific entity by its primary key with optional historical queries

## Architecture

### Key Components

- **Handler**: `/home/overlordyorch/Development/CanopyX/app/admin/controller/entity_query.go`
- **Types**: `/home/overlordyorch/Development/CanopyX/app/admin/controller/types/explorer.go`
- **Tests**: `/home/overlordyorch/Development/CanopyX/app/admin/controller/entity_query_test.go`
- **Routes**: Registered in `/home/overlordyorch/Development/CanopyX/app/admin/controller/controller.go`

### Design Principles

1. **Type Safety**: Uses the `entities` package for compile-time validation of entity names
2. **Testability**: Fully unit-tested with mock dependencies and comprehensive test cases
3. **Flexibility**: Supports querying both production and staging tables
4. **Pagination**: Cursor-based pagination for efficient traversal of large datasets
5. **Production Ready**: Proper error handling, logging, and authentication

## Endpoints

### 1. List Query Endpoint

**Endpoint**: `GET /api/chains/{id}/entity/{entity}`

**Authentication**: Required (uses `RequireAuth` middleware)

#### Path Parameters

- `id`: Chain ID (e.g., "1")
- `entity`: Entity name (e.g., "blocks", "accounts", "txs")

#### Query Parameters

| Parameter     | Type   | Default | Description                               |
|---------------|--------|---------|-------------------------------------------|
| `limit`       | int    | 50      | Number of records to return (max: 1000)   |
| `cursor`      | uint64 | 0       | Pagination cursor (height to start from)  |
| `sort`        | string | "desc"  | Sort order: "asc" or "desc"               |
| `use_staging` | bool   | false   | Query staging table instead of production |

#### Response Format

```json
{
  "data": [
    {
      "height": 100,
      "hash": "abc123",
      ...
    }
  ],
  "limit": 50,
  "next_cursor": 99
}
```

**Response Fields**:
- `data`: Array of entity objects (structure varies by entity type)
- `limit`: Number of records requested
- `next_cursor`: Height value for the next page (null if no more results)

#### Example Requests

**Query blocks in descending order:**
```bash
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/chains/1/entity/blocks?limit=10&sort=desc"
```

**Query accounts with pagination:**
```bash
# First page
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/chains/1/entity/accounts?limit=50"

# Next page using cursor
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/chains/1/entity/accounts?limit=50&cursor=123"
```

**Query staging table:**
```bash
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/chains/1/entity/txs?use_staging=true&limit=100"
```

### 2. Single Entity Lookup Endpoint

**Endpoint**: `GET /api/chains/{id}/entity/{entity}/{id_value}`

**Authentication**: Required (uses `RequireAuth` middleware)

#### Path Parameters

- `id`: Chain ID (e.g., "1")
- `entity`: Entity name (e.g., "blocks", "accounts", "validators")
- `id_value`: Primary key value (height for blocks, address for accounts, etc.)

#### Query Parameters

| Parameter     | Type   | Default | Description                                          |
|---------------|--------|---------|------------------------------------------------------|
| `height`      | uint64 | null    | Optional height for historical point-in-time queries |
| `use_staging` | bool   | false   | Query staging table instead of production            |

#### Response Format

Returns a single entity object or 404 if not found:

```json
{
  "height": 100,
  "hash": "abc123",
  "timestamp": "2024-01-01T00:00:00Z",
  ...
}
```

#### Example Requests

**Get block by height:**
```bash
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/chains/1/entity/blocks/100"
```

**Get account by address:**
```bash
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/chains/1/entity/accounts/canopy1abc123"
```

**Get account at specific height (historical query):**
```bash
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/chains/1/entity/accounts/canopy1abc123?height=500"
```

## Supported Entities

The API supports all entities defined in the `entities` package:

- `blocks` - Blockchain blocks
- `block_summaries` - Block summary aggregations
- `txs` - Transactions
- `accounts` - Account balances
- `events` - Blockchain events
- `orders` - Order book orders
- `pools` - Liquidity pools
- `dex_prices` - DEX price data
- `dex_orders` - DEX limit orders
- `dex_deposits` - DEX liquidity deposits
- `dex_withdrawals` - DEX liquidity withdrawals
- `dex_pool_points_by_holder` - DEX pool points by holder
- `params` - Chain parameters
- `validators` - Validators
- `validator_signing_info` - Validator signing information
- `committees` - Committee data
- `committee_validators` - Committee-validator junction table
- `poll_snapshots` - Governance poll snapshots

To get the full list with metadata, use the entities endpoint:

```bash
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/entities"
```

## Error Handling

### HTTP Status Codes

- `200 OK`: Successful query
- `400 Bad Request`: Invalid parameters or entity name
- `404 Not Found`: Chain or entity not found
- `500 Internal Server Error`: Database or system error

### Error Response Format

```json
{
  "error": "description of the error"
}
```

### Common Errors

**Invalid entity name:**
```json
{
  "error": "invalid entity: unknown entity \"invalid_entity\", valid entities: accounts, blocks, ..."
}
```

**Invalid limit:**
```json
{
  "error": "invalid request parameters: limit must be positive"
}
```

**Chain not found:**
```json
{
  "error": "chain not found"
}
```

## Implementation Details

### Cursor-Based Pagination

The API uses cursor-based pagination for efficient traversal:

1. Request limit+1 records to detect if more data exists
2. If more records exist, trim to requested limit and set `next_cursor`
3. The cursor is the `height` value of the last returned record
4. Client uses this cursor for the next request

**Benefits**:
- Consistent results even when data changes
- Efficient database queries (indexed on height)
- No offset-based performance degradation

### Table Selection Logic

The `use_staging` parameter determines which table to query:

- `use_staging=false` (default): Queries production table (e.g., `blocks`)
- `use_staging=true`: Queries staging table (e.g., `blocks_staging`)

This allows inspection of data before it's promoted to production, useful for:
- Debugging indexing issues
- Validating data before promotion
- Monitoring staging table contents

### Primary Key Detection

The single entity lookup endpoint intelligently detects the primary key:

1. If `id_value` is numeric: Assumes it's a height
2. If `id_value` is non-numeric: Assumes it's an address/identifier
3. If `height` parameter provided: Uses it as a filter for point-in-time queries

**Examples**:
- `GET /entity/blocks/100` → WHERE height = 100
- `GET /entity/accounts/canopy1abc` → WHERE address = 'canopy1abc' ORDER BY height DESC LIMIT 1
- `GET /entity/accounts/canopy1abc?height=500` → WHERE address = 'canopy1abc' AND height = 500

### Query Optimization

- Uses `FINAL` modifier for ReplacingMergeTree tables to get deduplicated results
- Leverages height-based indexes for efficient cursor navigation
- Limits maximum page size to prevent resource exhaustion

## Testing

Comprehensive unit tests cover:

- Request parameter parsing and validation
- Pagination logic (including cursor detection)
- Error handling for invalid inputs
- Mock-based testing without database dependencies
- Both happy path and error scenarios

Run tests:
```bash
go test -v ./app/admin/controller -run "TestParseEntityQueryRequest|TestParseEntityGetRequest|TestHandleEntityQuery|TestHandleEntityGet"
```

## Security Considerations

1. **Authentication**: All endpoints require valid JWT authentication
2. **Rate Limiting**: Consider implementing rate limiting for production use
3. **Input Validation**: All parameters are validated before use
4. **SQL Injection**: Uses parameterized queries to prevent injection
5. **Resource Limits**: Maximum page size enforced to prevent abuse

## Performance Considerations

1. **Database Indexing**: Relies on height-based indexes for efficient queries
2. **Cursor Pagination**: More efficient than offset-based for large datasets
3. **FINAL Modifier**: May impact performance on very large tables; consider caching
4. **Connection Pooling**: Uses ClickHouse connection pool for efficiency

## Future Enhancements

Potential improvements for future iterations:

1. **Field Selection**: Allow clients to specify which fields to return
2. **Advanced Filtering**: Support WHERE clause conditions beyond cursor
3. **Aggregation Queries**: Support COUNT, SUM, AVG operations
4. **Result Caching**: Cache frequently accessed entities
5. **Batch Queries**: Allow fetching multiple entities in one request
6. **Export Formats**: Support CSV, JSON Lines for data export
7. **Compression**: Support compressed responses for large datasets

## Related Documentation

- [ClickHouse Documentation](/home/overlordyorch/Development/CanopyX/docs/clickhouse.md)
- [Entities Package](/home/overlordyorch/Development/CanopyX/pkg/db/entities/entities.go)
- [Admin Controller](/home/overlordyorch/Development/CanopyX/app/admin/controller/controller.go)

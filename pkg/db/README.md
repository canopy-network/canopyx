# Database Backend Configuration

The canopyx indexer supports two database backends:
- **ClickHouse** (default, production-ready)
- **Postgres** (alternative backend, schema complete, query methods in progress)

## Current Status

### ClickHouse (Default)
- **Location**: `pkg/db/admin`, `pkg/db/chain`, `pkg/db/crosschain`
- **Status**: Production-ready, fully implemented
- **Use Case**: High-throughput OLAP workloads, large-scale deployments

### Postgres (Alternative)
- **Location**: `pkg/db/postgres/admin`, `pkg/db/postgres/chain`, `pkg/db/postgres/crosschain`
- **Status**: Schema complete, interface methods partially implemented
- **Use Case**: ACID transactions, simpler deployment, compatibility with existing Postgres infrastructure

## Architecture

### Schema Organization

Both backends implement the same three-database architecture:

1. **Admin DB** (`admin` package)
   - Stores: chains, index_progress, rpc_endpoints, reindex_requests
   - Purpose: Orchestration metadata

2. **Chain DB** (`chain` package)
   - Stores: blocks, txs, events, accounts, validators, pools, orders, etc.
   - Purpose: Per-chain blockchain data

3. **CrossChain DB** (`crosschain` package)
   - Stores: Aggregated multi-chain data with views for latest state
   - Purpose: Global queries across all indexed chains

### Key Differences

| Feature | ClickHouse | Postgres |
|---------|-----------|----------|
| **Staging Pattern** | Two-phase commit with staging tables | Direct writes with ACID transactions |
| **Latest State** | `FINAL` modifier | `DISTINCT ON` views |
| **Schema Definition** | Go code (dynamic table creation) | Go code (SQL table creation) |
| **Transactions** | Manual two-phase commit | Native ACID with `BEGIN/COMMIT` |
| **Type System** | UInt64, DateTime64, LowCardinality | BIGINT, TIMESTAMPTZ, TEXT |

## Using Postgres Backend

### Option 1: Direct Initialization (Recommended for New Code)

```go
import (
    "context"
    "time"

    "github.com/canopy-network/canopyx/pkg/db/postgres"
    postgresadmin "github.com/canopy-network/canopyx/pkg/db/postgres/admin"
    postgreschain "github.com/canopy-network/canopyx/pkg/db/postgres/chain"
    postgrescrosschain "github.com/canopy-network/canopyx/pkg/db/postgres/crosschain"
    "go.uber.org/zap"
)

func InitializePostgresDBs(ctx context.Context, logger *zap.Logger, chainID uint64) error {
    // Configure Postgres connection pools
    adminPoolConfig := postgres.PoolConfig{
        MinConns:        2,
        MaxConns:        50,
        ConnMaxLifetime: 1 * time.Hour,
        ConnMaxIdleTime: 30 * time.Minute,
        Component:       "indexer_admin",
    }

    chainPoolConfig := postgres.PoolConfig{
        MinConns:        10,
        MaxConns:        200,
        ConnMaxLifetime: 1 * time.Hour,
        ConnMaxIdleTime: 30 * time.Minute,
        Component:       "indexer_chain",
    }

    crosschainPoolConfig := postgres.PoolConfig{
        MinConns:        5,
        MaxConns:        100,
        ConnMaxLifetime: 1 * time.Hour,
        ConnMaxIdleTime: 30 * time.Minute,
        Component:       "indexer_crosschain",
    }

    // Initialize databases
    adminDB, err := postgresadmin.NewWithPoolConfig(ctx, logger, "canopyx_indexer", adminPoolConfig)
    if err != nil {
        return fmt.Errorf("failed to initialize admin DB: %w", err)
    }
    defer adminDB.Close()

    chainDB, err := postgreschain.NewWithPoolConfig(ctx, logger, chainID, chainPoolConfig)
    if err != nil {
        return fmt.Errorf("failed to initialize chain DB: %w", err)
    }
    defer chainDB.Close()

    crosschainDB, err := postgrescrosschain.NewWithPoolConfig(ctx, logger, "canopyx_cross_chain", crosschainPoolConfig)
    if err != nil {
        return fmt.Errorf("failed to initialize crosschain DB: %w", err)
    }
    defer crosschainDB.Close()

    // Use databases...
    return nil
}
```

### Option 2: Environment-Based Selection

Add to your application configuration:

```bash
# Database backend selection
DB_BACKEND=postgres  # or "clickhouse" (default)

# Postgres connection settings (used when DB_BACKEND=postgres)
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=canopy
POSTGRES_PASSWORD=your_password
POSTGRES_DATABASE=postgres  # Admin database for creating other databases
POSTGRES_SSL_MODE=disable

# ClickHouse connection settings (used when DB_BACKEND=clickhouse)
CLICKHOUSE_HOST=localhost
CLICKHOUSE_PORT=9000
# ... other ClickHouse settings
```

### Migration from ClickHouse to Postgres

1. **Schema Migration**: Both backends create schemas in Go code, so no SQL migration files needed
2. **Data Migration**: Use the existing ClickHouse data as source:
   ```sql
   -- Export from ClickHouse
   SELECT * FROM accounts INTO OUTFILE 'accounts.csv' FORMAT CSVWithNames;

   -- Import to Postgres
   \COPY accounts FROM 'accounts.csv' WITH CSV HEADER;
   ```
3. **Application Migration**: Update initialization code to use Postgres packages instead of ClickHouse

## Implementation Status

### ✅ Complete
- [x] Postgres admin store schema (chains, index_progress, rpc_endpoints, reindex_requests)
- [x] Postgres chain store schema (22 tables: blocks, txs, events, accounts, validators, etc.)
- [x] Postgres crosschain store schema (22 tables, 11 views, 2 PL/pgSQL functions)
- [x] Store interface structure (staging methods implemented as no-ops or direct writes)

### ⚠️ In Progress
- [ ] Implement query methods (GetBlock, GetEventsByType, etc.)
- [ ] Implement insert methods with proper transaction handling
- [ ] Implement delete methods
- [ ] Add comprehensive integration tests
- [ ] Performance benchmarks comparing ClickHouse vs Postgres

### 📋 Future Enhancements
- [ ] Unified factory pattern for backend selection
- [ ] Backend-agnostic Store interfaces (remove ClickHouse-specific types)
- [ ] Automatic schema migration tooling
- [ ] Support for read replicas
- [ ] Connection pool metrics and monitoring

## Connection Details

### Postgres Connection String

The Postgres client uses environment variables:
- `POSTGRES_HOST`: Database host (default: localhost)
- `POSTGRES_PORT`: Database port (default: 5432)
- `POSTGRES_USER`: Database user
- `POSTGRES_PASSWORD`: Database password
- `POSTGRES_DATABASE`: Initial database name (default: postgres)
- `POSTGRES_SSL_MODE`: SSL mode (default: disable)

### ClickHouse Connection String

The ClickHouse client uses environment variables:
- `CLICKHOUSE_HOST`: Database host
- `CLICKHOUSE_PORT`: Database port (default: 9000)
- `CLICKHOUSE_USERNAME`: Database user
- `CLICKHOUSE_PASSWORD`: Database password
- `CLICKHOUSE_DATABASE`: Database name

## Performance Considerations

### ClickHouse Strengths
- Columnar storage optimized for analytical queries
- Excellent compression ratios
- Horizontal scalability
- Optimized for append-only workloads

### Postgres Strengths
- ACID transactions (no need for staging pattern)
- Rich ecosystem and tooling
- Better support for updates and deletes
- Simpler deployment and operations

### Recommended Use Cases

**Use ClickHouse when:**
- Running at scale (100+ chains, billions of rows)
- Analytical queries are the primary workload
- You need horizontal scalability
- Append-only access pattern is sufficient

**Use Postgres when:**
- Simpler deployment is preferred
- ACID transactions are required
- You have existing Postgres infrastructure
- Scale is moderate (<10 chains, millions of rows)
- You need richer transaction semantics

## Development

### Running Tests with Postgres

```bash
# Start Postgres
docker run -d \
  --name canopy-postgres \
  -e POSTGRES_USER=canopy \
  -e POSTGRES_PASSWORD=canopy \
  -e POSTGRES_DB=postgres \
  -p 5432:5432 \
  postgres:16

# Set environment variables
export DB_BACKEND=postgres
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5432
export POSTGRES_USER=canopy
export POSTGRES_PASSWORD=canopy
export POSTGRES_DATABASE=postgres
export POSTGRES_SSL_MODE=disable

# Run tests
go test ./pkg/db/postgres/...
```

### Running Tests with ClickHouse

```bash
# Start ClickHouse
docker run -d \
  --name canopy-clickhouse \
  -p 9000:9000 \
  -p 8123:8123 \
  clickhouse/clickhouse-server:latest

# Set environment variables
export DB_BACKEND=clickhouse
export CLICKHOUSE_HOST=localhost
export CLICKHOUSE_PORT=9000

# Run tests
go test ./pkg/db/admin/... ./pkg/db/chain/... ./pkg/db/crosschain/...
```

## Troubleshooting

### Postgres Connection Issues

```
Error: failed to connect to postgres: connection refused
```
**Solution**: Ensure Postgres is running and accepting connections on the configured host/port.

### Schema Creation Failures

```
Error: relation "accounts" already exists
```
**Solution**: The code uses `CREATE TABLE IF NOT EXISTS`, so this shouldn't happen. If it does, drop and recreate the database.

### Type Conversion Errors

```
Error: cannot convert UInt64 to BIGINT
```
**Solution**: This indicates mixing ClickHouse and Postgres models. Ensure you're using the correct package imports.

## Contributing

When adding new tables or queries:

1. **Add to both backends**: Maintain feature parity between ClickHouse and Postgres
2. **Follow type mappings**: Use the type mapping table above
3. **Test both backends**: Run integration tests against both ClickHouse and Postgres
4. **Update documentation**: Keep this README current with implementation status

## References

- [ClickHouse Documentation](https://clickhouse.com/docs/)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
- [pgx Driver Documentation](https://github.com/jackc/pgx)
- [ClickHouse Go Driver](https://github.com/ClickHouse/clickhouse-go)

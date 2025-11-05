# CanopyX Testing Guide

This document provides comprehensive information about running tests in the CanopyX project.

## Test Organization

The project follows a clear test organization strategy:

```
tests/
├── integration/          # Integration tests (require Docker/ClickHouse)
│   ├── db/              # Database integration tests
│   │   ├── main_test.go
│   │   └── chain_test.go
│   ├── helpers/         # Test helpers and utilities
│   │   ├── runtime.go
│   │   └── db_helpers.go
│   ├── indexer/         # Indexer integration tests
│   │   ├── main_test.go
│   │   ├── accounts_test.go         # Entity 1 (Accounts) tests
│   │   ├── clickhouse_queries_test.go
│   │   └── clickhouse_tables_test.go
│   └── query/           # Query API integration tests
│       ├── main_test.go
│       └── placeholder_test.go
└── unit/                # Unit tests (fast, no external dependencies)
    └── (future unit tests)

pkg/
├── rpc/
│   └── accounts_test.go             # RPC client unit tests
└── indexer/
    └── activity/
        ├── accounts_test.go         # Activity unit tests (with mocks)
        ├── context_test.go
        ├── promote_test.go
        ├── activity_test.go
        └── genesis_test.go
```

## Test Commands

### Run All Tests

```bash
make test
```

This runs both unit and integration tests.

### Run Integration Tests Only

Integration tests require Docker to be running (for ClickHouse testcontainers).

```bash
# Run all integration tests
make test-integration

# Run with verbose output
make test-integration

# Run specific integration test package
go test -v ./tests/integration/indexer/...

# Run specific test by name
make test-integration RUN=TestAccountsTableCreation

# Run specific test with full pattern
go test -v -run TestAccountsChangeDetection ./tests/integration/indexer/
```

### Run Unit Tests Only

Unit tests are fast and don't require Docker.

```bash
# Run all unit tests
make test-unit

# Run unit tests in a specific package
go test ./pkg/rpc/...

# Run specific unit test
go test -run TestAccountsByHeight_Success ./pkg/rpc/
```

### Test with Coverage

```bash
# Run tests with coverage report
go test -cover ./...

# Generate detailed HTML coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Coverage for integration tests only
go test -cover ./tests/integration/...
```

### Test with Race Detection

```bash
# Run with race detector (slower but catches concurrency issues)
go test -race ./...

# Race detection for specific package
go test -race ./tests/integration/indexer/...
```

### Skip Long-Running Tests

Some tests are marked to skip in short mode:

```bash
# Skip slow tests (useful for quick feedback during development)
go test -short ./...
```

## Accounts Entity Tests (Entity 1)

The accounts entity has comprehensive test coverage:

### RPC Client Tests
Location: `pkg/rpc/accounts_test.go`

Tests the RPC client's ability to fetch accounts from blockchain nodes:
- Successful account fetching with pagination
- Multiple page handling
- Empty responses
- Network error handling
- Genesis state fetching

```bash
# Run RPC accounts tests
go test -v ./pkg/rpc/ -run TestAccounts

# Run specific RPC test
go test -v ./pkg/rpc/ -run TestAccountsByHeight_Success
```

### Activity Tests (Unit)
Location: `pkg/indexer/activity/accounts_test.go`

Tests the account indexing activity with mocks:
- Change detection logic
- Genesis handling at height 1
- RPC failure handling
- Large dataset processing
- No-change scenarios

```bash
# Run activity accounts tests
go test -v ./pkg/indexer/activity/ -run TestIndexAccounts

# Run all activity tests
go test -v ./pkg/indexer/activity/...
```

### Database Integration Tests
Location: `tests/integration/indexer/accounts_test.go`

Comprehensive integration tests with real ClickHouse:
- **Table Creation**: Verify schema, engine, and indexes
- **Insert and Query**: Basic CRUD operations
- **Change Detection**: Snapshot-on-change pattern validation
- **Staging/Promote/Clean**: Full workflow testing
- **Genesis Handling**: Height 0 and height 1 edge cases
- **ReplacingMergeTree**: Deduplication verification
- **Parallel Indexing**: Concurrent writes
- **Large Datasets**: Performance with 10K+ accounts
- **Chain Isolation**: Multi-chain data separation
- **Empty Results**: Graceful handling of no data

```bash
# Run all accounts integration tests
go test -v ./tests/integration/indexer/ -run TestAccounts

# Run specific accounts integration test
go test -v ./tests/integration/indexer/ -run TestAccountsChangeDetection

# Run staging workflow test
go test -v ./tests/integration/indexer/ -run TestAccountsStagingPromoteClean

# Skip slow tests (skips large dataset test)
go test -v -short ./tests/integration/indexer/ -run TestAccounts
```

## Integration Test Requirements

Integration tests use [testcontainers-go](https://golang.testcontainers.org/) to spin up ClickHouse automatically.

### Prerequisites

1. **Docker** must be running
2. Docker socket must be accessible at `/var/run/docker.sock`
3. Sufficient disk space for ClickHouse image (~400MB)

### Troubleshooting Integration Tests

**If tests are skipped**:
```
Docker not available, skipping integration tests
```
Solution: Start Docker daemon.

**If container fails to start**:
```
Failed to start ClickHouse container
```
Solutions:
- Check Docker is running: `docker ps`
- Check Docker logs: `docker logs <container_id>`
- Ensure port 9000 is available

**If tests are slow**:
- First run downloads ClickHouse image (one-time ~30s)
- Subsequent runs are faster (~3-5s startup)
- Use `go test -short` to skip slow tests during development

## Test Patterns and Best Practices

### Integration Tests

Integration tests in `tests/integration/` follow these patterns:

1. **Setup**: Use `helpers.TestDB()` and `helpers.NewChainDb()`
2. **Cleanup**: Use `helpers.CleanDB()` between tests
3. **Skip**: Use `helpers.SkipIfNoDB()` to skip if Docker unavailable
4. **Assertions**: Use `testify/assert` and `testify/require`
5. **Isolation**: Each test should be independent and idempotent

Example:
```go
func TestMyFeature(t *testing.T) {
    ctx := context.Background()
    if helpers.SkipIfNoDB(ctx, t) {
        return
    }
    helpers.CleanDB(ctx, t)

    // Test code here
}
```

### Unit Tests

Unit tests in `pkg/` alongside source code follow these patterns:

1. **Mocking**: Use `testify/mock` for dependencies
2. **Table-Driven**: Use subtests with `t.Run()`
3. **Fast**: No external dependencies (RPC, DB, etc.)
4. **Coverage**: Test happy path and error cases

Example:
```go
func TestAccountsByHeight(t *testing.T) {
    tests := []struct {
        name    string
        height  uint64
        want    int
        wantErr bool
    }{
        {"success", 100, 2, false},
        {"empty", 200, 0, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test code
        })
    }
}
```

## Continuous Integration

For CI environments:

```bash
# Run all tests with race detection and coverage
go test -v -race -cover ./...

# Run only fast tests
go test -v -short ./...

# Run with timeout
go test -v -timeout=10m ./...
```

## Test Helpers

### Database Helpers

Located in `tests/integration/helpers/`:

- `helpers.TestDB()` - Get shared admin database connection
- `helpers.NewChainDb(ctx, chainID)` - Create chain-specific database
- `helpers.CleanDB(ctx, t)` - Clean all test data
- `helpers.SkipIfNoDB(ctx, t)` - Skip if no database available
- `helpers.CreateTestChain(id, name, opts...)` - Create test chain config
- `helpers.SeedData(ctx, t, opts...)` - Seed test data
- `helpers.WaitForMaterializedView(t, duration)` - Wait for MV to process
- `helpers.AssertGaps(ctx, t, chainID, expected)` - Assert index gaps

### Seed Data Examples

```go
// Create and seed a chain
chain := helpers.CreateTestChain("my-chain", "My Test Chain",
    helpers.WithRPCEndpoints("http://localhost:8545"),
    helpers.WithImage("indexer:v1"),
)
helpers.SeedData(ctx, t, helpers.WithChains(chain))

// Seed multiple chains
chain1 := helpers.CreateTestChain("chain-1", "Chain 1")
chain2 := helpers.CreateTestChain("chain-2", "Chain 2")
helpers.SeedData(ctx, t, helpers.WithChains(chain1, chain2))
```

## Performance Benchmarks

Run benchmarks:

```bash
# Run all benchmarks
go test -bench=. ./...

# Run specific benchmark
go test -bench=BenchmarkAccountsByHeight ./pkg/rpc/

# Run benchmarks with memory profiling
go test -bench=. -benchmem ./...

# Run benchmarks multiple times for statistical significance
go test -bench=. -count=10 ./...
```

## Debugging Tests

### Verbose Output

```bash
# See all test output
go test -v ./tests/integration/indexer/
```

### Run Single Test Repeatedly

```bash
# Run test 100 times to catch flaky tests
go test -count=100 -run TestAccountsChangeDetection ./tests/integration/indexer/
```

### Use Delve Debugger

```bash
# Debug a specific test
dlv test ./tests/integration/indexer/ -- -test.run TestAccountsTableCreation
```

## Next Steps

1. Run all tests to ensure everything passes: `make test`
2. Add tests for new features following existing patterns
3. Maintain test coverage above 70% for critical paths
4. Run `make lint` before committing to catch issues early

## Summary of Test Locations

| Test Type | Location | Purpose | Requirements |
|-----------|----------|---------|--------------|
| RPC Unit | `pkg/rpc/*_test.go` | Test RPC client logic | None |
| Activity Unit | `pkg/indexer/activity/*_test.go` | Test indexing activities | Mocks |
| DB Integration | `tests/integration/db/` | Test database operations | Docker |
| Indexer Integration | `tests/integration/indexer/` | Test full indexing workflow | Docker |
| Query Integration | `tests/integration/query/` | Test query API | Docker |

## Contact

For questions about testing:
- Check existing test files for patterns
- Review this guide
- Ask in team chat

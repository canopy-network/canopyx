package clickhouse

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/canopy-network/canopyx/pkg/retry"
	"github.com/canopy-network/canopyx/pkg/utils"
	"go.uber.org/zap"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type Client struct {
	Logger         *zap.Logger
	Db             driver.Conn
	TargetDatabase string // Target database name (may differ from the current connection)
	IsCloud        bool   // True for ClickHouse Cloud (no ON CLUSTER needed), false for self-hosted
}

// PoolConfig defines connection pool settings for a specific component
type PoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	Component       string // For logging/debugging
}

const (
	MergeTree            = "MergeTree"
	AggregatingMergeTree = "AggregatingMergeTree"
	ReplacingMergeTree   = "ReplacingMergeTree"
)

// New initializes and returns a new database client for ClickHouse with provided context and logger.
// Includes connection pooling optimizations for high-throughput workloads.
// Accepts optional poolConfig parameter for component-specific pool sizing.
func New(ctx context.Context, logger *zap.Logger, dbName string, poolConfig ...*PoolConfig) (client Client, e error) {
	// Add timeout to context for initial connection
	connCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	client.Logger = logger
	client.IsCloud = strings.ToLower(os.Getenv("CLICKHOUSE_CLOUD")) == "true"
	retryConfig := retry.DefaultConfig()

	dsn := utils.Env("CLICKHOUSE_ADDR", "clickhouse://localhost:9000")
	// Parse DSN using the official clickhouse-go parser
	// This properly handles TLS configuration via ?secure=true/false parameter
	options, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return Client{}, fmt.Errorf("failed to parse CLICKHOUSE_ADDR DSN: %w", err)
	}

	// Log TLS state for debugging
	if logger != nil {
		logger.Debug("Parsed ClickHouse DSN",
			zap.String("dsn", dsn),
			zap.Bool("tls_enabled", options.TLS != nil),
			zap.Strings("addrs", options.Addr),
		)
	}

	// First, connect without specifying a database to create it
	debugEnabled := logger != nil && logger.Core().Enabled(zap.DebugLevel)

	// Connection pool settings - use provided config or fallback to legacy defaults
	var config PoolConfig
	if len(poolConfig) > 0 && poolConfig[0] != nil {
		config = *poolConfig[0]
	} else {
		// Fallback to legacy defaults for backward compatibility
		config = PoolConfig{
			MaxOpenConns:    utils.EnvInt("CLICKHOUSE_MAX_OPEN_CONNS", 75),
			MaxIdleConns:    utils.EnvInt("CLICKHOUSE_MAX_IDLE_CONNS", 75),
			ConnMaxLifetime: ParseConnMaxLifetime(""),
			Component:       "unknown",
		}
	}

	maxOpenConns := config.MaxOpenConns
	maxIdleConns := config.MaxIdleConns
	connMaxLifetime := config.ConnMaxLifetime

	// Parse connection strategy from environment(override DSN if set)
	// Strategies:
	//   - in_order: Always use first replica, fallback to others on failure
	//               Use for: Indexer (read-after-write consistency)
	//   - round_robin: Distribute connections evenly across all replicas
	//               Use for: SuperApp/API (read distribution, high throughput)
	//   - random: Random replica selection
	//               Use for: SuperApp/API (load balancing)
	connStrategyEnv := utils.Env("CLICKHOUSE_CONN_STRATEGY", "")
	if connStrategyEnv != "" {
		options.ConnOpenStrategy = parseConnOpenStrategy(connStrategyEnv)
	} else if options.ConnOpenStrategy == 0 {
		// Default to in_order if not set in DSN or env
		options.ConnOpenStrategy = clickhouse.ConnOpenInOrder
	}

	// Override pool settings from config (takes precedence over DSN)
	options.Auth.Database = "default" // Connect to default database first
	options.DialTimeout = 30 * time.Second
	options.MaxOpenConns = maxOpenConns
	options.MaxIdleConns = maxIdleConns
	options.ConnMaxLifetime = connMaxLifetime
	options.BlockBufferSize = 10
	options.MaxCompressionBuffer = 10240
	options.Debug = false

	// Set compression if not already set
	if options.Compression == nil {
		options.Compression = &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		}
	}

	// Merge settings
	if options.Settings == nil {
		options.Settings = make(clickhouse.Settings)
	}

	options.Settings["prefer_column_name_to_alias"] = 1
	//options.Settings["allow_experimental_object_type"] = 1

	if debugEnabled {
		sugar := logger.Named("clickhouse.driver").Sugar()
		options.Debugf = sugar.Debugf
	}

	retryErr := retry.WithBackoff(connCtx, retryConfig, logger, "clickhouse_connection", func() error {
		// Open connection to a default database
		conn, openErr := clickhouse.Open(options)
		if openErr != nil {
			return fmt.Errorf("failed to open clickhouse connection: %w", openErr)
		}

		client.Db = conn

		client.Logger.Debug("Pinging ClickHouse connection",
			zap.String("db", dbName),
			zap.String("component", config.Component),
		)
		pingErr := client.Db.Ping(connCtx)
		if pingErr != nil {
			return fmt.Errorf("failed to ping clickhouse: %w", pingErr)
		}

		// NOTE: Keep connection to 'default' database for now
		// The wrapper's InitializeDB() will create the target database, then switch to it
		// This avoids the chicken-and-egg problem where we can't connect to a non-existent database
		client.Db = conn
		client.TargetDatabase = dbName // Store target database name for later use

		client.Logger.Info("ClickHouse connection pool configured",
			zap.String("database", dbName),
			zap.String("component", config.Component),
			zap.Strings("replicas", options.Addr),
			zap.String("conn_strategy", formatConnOpenStrategy(options.ConnOpenStrategy)),
			zap.Bool("tls_enabled", options.TLS != nil),
			zap.Int("max_open_conns", maxOpenConns),
			zap.Int("max_idle_conns", maxIdleConns),
			zap.Duration("conn_max_lifetime", connMaxLifetime),
		)
		return nil
	})

	if retryErr != nil {
		return Client{}, retryErr
	}

	return client, nil
}

// ParseConnMaxLifetime parses a connection max lifetime duration string.
// If lifetimeStr is empty, falls back to CLICKHOUSE_CONN_MAX_LIFETIME environment variable.
// If neither exists, returns default of 1 hour.
func ParseConnMaxLifetime(lifetimeStr string) time.Duration {
	// Try parsing the provided string first
	if lifetimeStr != "" {
		if d, err := time.ParseDuration(lifetimeStr); err == nil {
			return d
		}
	}

	// Fall back to environment variable
	if envStr := os.Getenv("CLICKHOUSE_CONN_MAX_LIFETIME"); envStr != "" {
		if d, err := time.ParseDuration(envStr); err == nil {
			return d
		}
	}

	// Default to 1 hour
	return 1 * time.Hour
}

// parseConnOpenStrategy converts a string to clickhouse.ConnOpenStrategy
// Supported values: "in_order", "round_robin", "random"
// Defaults to in_order if invalid value provided
func parseConnOpenStrategy(strategy string) clickhouse.ConnOpenStrategy {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "round_robin", "roundrobin":
		return clickhouse.ConnOpenRoundRobin
	case "random":
		return clickhouse.ConnOpenRandom
	case "in_order", "inorder", "":
		return clickhouse.ConnOpenInOrder
	default:
		// Default to in_order for safety (read-after-write consistency)
		return clickhouse.ConnOpenInOrder
	}
}

// formatConnOpenStrategy converts clickhouse.ConnOpenStrategy to human-readable string
func formatConnOpenStrategy(strategy clickhouse.ConnOpenStrategy) string {
	switch strategy {
	case clickhouse.ConnOpenRoundRobin:
		return "round_robin"
	case clickhouse.ConnOpenRandom:
		return "random"
	case clickhouse.ConnOpenInOrder:
		return "in_order"
	default:
		return "unknown"
	}
}

// WithSequentialConsistency wraps a context to enable select_sequential_consistency for the next query.
// This ensures the query sees all data that was previously written, preventing read-after-write inconsistencies
// in replicated ClickHouse clusters.
//
// Usage:
//
//	ctx = clickhouse.WithSequentialConsistency(ctx)
//	row := db.QueryRowContext(ctx, "SELECT * FROM table WHERE id = ?", id)
//
// Performance Note:
// This setting adds ClickHouse Keeper coordination overhead to the query.
// Only use when read-after-write consistency is critical (e.g., PromoteData reading staging, RecordIndexed reading production).
func WithSequentialConsistency(ctx context.Context) context.Context {
	return clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
		"select_sequential_consistency": 1,
	}))
}

// NOTE: insert_quorum and external aggregation settings are configured at the
// ClickHouse server level in deploy/k8s/clickhouse/base/installation.yaml.
// This provides consistent behavior across all queries and allows tuning
// based on actual database resources without code changes.

// SanitizeName sanitizes the provided database name to be compatible with ClickHouse.
func SanitizeName(id string) string {
	s := strings.ToLower(id)
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	return s
}

// ReplicatedEngine returns the appropriate engine string for replicated ClickHouse clusters.
// Uses automatic UUID-based ZooKeeper paths to avoid REPLICA_ALREADY_EXISTS errors.
//
// For ReplacingMergeTree with version column:
//   - engine: "ReplacingMergeTree", versionCol: "updated_at"
//   - Returns: ReplicatedReplacingMergeTree(updated_at)
//
// For AggregatingMergeTree:
//   - engine: "AggregatingMergeTree", versionCol: ""
//   - Returns: ReplicatedAggregatingMergeTree
//
// IMPORTANT: Omitting ZK paths lets ClickHouse auto-generate unique UUID-based paths.
// This prevents conflicts when tables are dropped/recreated.
// See: https://github.com/ClickHouse/ClickHouse/issues/47920
//
//	https://github.com/ClickHouse/ClickHouse/issues/20243
func ReplicatedEngine(engine, versionCol string) string {
	replicatedEngine := "Replicated" + engine

	// Let ClickHouse auto-generate UUID-based ZK paths (ClickHouse 20.4+)
	// This avoids REPLICA_ALREADY_EXISTS errors from static paths
	if versionCol != "" {
		return fmt.Sprintf("%s(%s)", replicatedEngine, versionCol)
	}
	return replicatedEngine
}

// Exec Helper method to execute raw SQL queries
func (c *Client) Exec(ctx context.Context, query string, args ...interface{}) error {
	return c.Db.Exec(ctx, query, args...)
}

// QueryRow Helper method to query a single row
func (c *Client) QueryRow(ctx context.Context, query string, args ...interface{}) driver.Row {
	return c.Db.QueryRow(ctx, query, args...)
}

// Query Helper method to query multiple rows.
// IMPORTANT: Caller MUST call rows.Close() when done to release the connection back to pool.
// Prefer Select() when possible as it handles connection release automatically.
func (c *Client) Query(ctx context.Context, query string, args ...interface{}) (driver.Rows, error) {
	return c.Db.Query(ctx, query, args...)
}

// Select Helper method to select into a slice.
// This is the preferred method for queries as it automatically handles row closing
// and releases the connection back to the pool immediately after scanning.
func (c *Client) Select(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return c.Db.Select(ctx, dest, query, args...)
}

// QueryWithTimeout wraps Query with a query-level timeout.
// This prevents long-running queries from holding connections indefinitely.
// Uses ClickHouse's max_execution_time setting for server-side enforcement.
func (c *Client) QueryWithTimeout(ctx context.Context, timeout time.Duration, query string, args ...interface{}) (driver.Rows, error) {
	queryCtx := clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
		"max_execution_time": int(timeout.Seconds()),
	}))
	return c.Db.Query(queryCtx, query, args...)
}

// SelectWithTimeout wraps Select with a query-level timeout.
// This is the preferred method for queries with timeout requirements.
func (c *Client) SelectWithTimeout(ctx context.Context, timeout time.Duration, dest interface{}, query string, args ...interface{}) error {
	queryCtx := clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
		"max_execution_time": int(timeout.Seconds()),
	}))
	return c.Db.Select(queryCtx, dest, query, args...)
}

// QueryRowWithTimeout wraps QueryRow with a query-level timeout.
func (c *Client) QueryRowWithTimeout(ctx context.Context, timeout time.Duration, query string, args ...interface{}) driver.Row {
	queryCtx := clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
		"max_execution_time": int(timeout.Seconds()),
	}))
	return c.Db.QueryRow(queryCtx, query, args...)
}

// PrepareBatch Helper method for batch inserts
func (c *Client) PrepareBatch(ctx context.Context, query string) (driver.Batch, error) {
	return c.Db.PrepareBatch(ctx, query, driver.WithReleaseConnection())
}

// Close Helper method to close the connection
func (c *Client) Close() error {
	return c.Db.Close()
}

// OnCluster returns ON CLUSTER statement for distributed DDL operations.
// - ClickHouse Cloud (IsCloud=true): Returns "" (Cloud handles replication automatically)
// - Self-hosted (IsCloud=false): Returns "ON CLUSTER canopyx"
// See: https://clickhouse.com/docs/sql-reference/distributed-ddl
func (c *Client) OnCluster() string {
	if c.IsCloud {
		return ""
	}
	return "ON CLUSTER canopyx"
}

// DbEngine returns the database engine string based on deployment mode.
// - ClickHouse Cloud (IsCloud=true): Returns "" (uses default Atomic engine, Cloud handles replication)
// - Self-hosted (IsCloud=false): Returns Replicated engine for automatic DDL replication
//
// The Replicated database engine automatically replicates DDL (CREATE TABLE, etc.) across nodes
// after the database is created with ON CLUSTER. Uses {uuid} macro which is resolved identically
// across all replicas when used with ON CLUSTER.
func (c *Client) DbEngine() string {
	if c.IsCloud {
		return "" // Cloud uses default Atomic, handles replication automatically
	}
	return "ENGINE = Replicated('/clickhouse/databases/{uuid}', '{shard}', '{replica}')"
}

// CreateDbIfNotExists ensures that the specified database exists by creating it if it does not already exist.
// - Self-hosted: Uses ON CLUSTER + Replicated engine for DDL replication
// - ClickHouse Cloud: No ON CLUSTER, default engine (Cloud handles replication)
func (c *Client) CreateDbIfNotExists(ctx context.Context, dbName string) error {
	query := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s %s %s", dbName, c.OnCluster(), c.DbEngine())
	c.Logger.Info("Creating database", zap.String("database", dbName), zap.String("query", query))
	return c.Exec(ctx, query)
}

// IsNoRows Helper to check if the error is no rows
func IsNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

// QueryWithFinal verify than a query contains a FINAL statement.
// The FINAL modifier ensures you get the most recent version of deduplicated rows,
// which is essential for correctness when reading from ReplacingMergeTree tables.
//
// IMPORTANT: Use FINAL only when necessary as it has performance implications.
// Staging tables should use FINAL for reads, production tables may not need it
// depending on your merge schedule.
//
// Example usage:
//
//	rows, err := client.QueryWithFinal(ctx, `
//	    SELECT * FROM events_staging
//	    WHERE height = ?
//	`, height)
func (c *Client) QueryWithFinal(ctx context.Context, query string, args ...interface{}) (driver.Rows, error) {
	// Check if a query already has a FINAL keyword
	if !strings.Contains(query, "FINAL") {
		return nil, fmt.Errorf("QueryWithFinal called but query doesn't contain FINAL keyword - ensure FINAL is placed after table name")
	}
	return c.Db.Query(ctx, query, args...)
}

// SelectWithFinal verify than a Select query contains a FINAL statement.
// This is a convenience wrapper around Select that enforces FINAL usage for correctness.
//
// Example usage:
//
//	var events []*Event
//	err := client.SelectWithFinal(ctx, &events, `
//	    SELECT * FROM events_staging FINAL
//	    WHERE height = ?
//	`, height)
func (c *Client) SelectWithFinal(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	// Check if a query already has a FINAL keyword
	// TODO: add caller information
	if !strings.Contains(query, "FINAL") {
		return fmt.Errorf("SelectWithFinal called but query doesn't contain FINAL keyword - ensure FINAL is placed after table name")
	}
	return c.Db.Select(ctx, dest, query, args...)
}

// TableHealthStatus represents the health status of a table
type TableHealthStatus struct {
	Database            string    `ch:"database"`
	Table               string    `ch:"table"`
	Engine              string    `ch:"engine"`
	TotalRows           uint64    `ch:"total_rows"`
	TotalBytes          uint64    `ch:"total_bytes"`
	TotalBytesUncomp    uint64    `ch:"total_bytes_uncompressed"`
	CompressionRatio    float64   `ch:"compression_ratio"`
	PartitionCount      uint64    `ch:"partition_count"`
	ActivePartitions    uint64    `ch:"active_partitions"`
	LastModificationAge string    `ch:"last_modification_age"`
	LastModifyTime      time.Time `ch:"last_modify_time"`
}

// CheckTableHealth retrieves comprehensive health metrics for a table.
// This includes size, row counts, compression ratios, and partition information.
//
// Example usage:
//
//	health, err := client.CheckTableHealth(ctx, "mydb", "events")
//	fmt.Printf("Table has %d rows in %d MB\n", health.TotalRows, health.TotalBytes/1024/1024)
//	fmt.Printf("Compression ratio: %.2f\n", health.CompressionRatio)
func (c *Client) CheckTableHealth(ctx context.Context, database, table string) (*TableHealthStatus, error) {
	query := `
		SELECT
			database,
			table,
			engine,
			sum(rows) as total_rows,
			sum(bytes) as total_bytes,
			sum(bytes_on_disk) as total_bytes_uncompressed,
			if(sum(bytes_on_disk) > 0, sum(bytes) / sum(bytes_on_disk), 0) as compression_ratio,
			count(DISTINCT partition) as partition_count,
			sum(active) as active_partitions,
			formatReadableTimeDelta(now() - max(modification_time)) as last_modification_age,
			max(modification_time) as last_modify_time
		FROM system.parts
		WHERE database = ? AND table = ?
		GROUP BY database, table, engine
	`

	var health TableHealthStatus
	err := c.QueryRow(ctx, query, database, table).Scan(
		&health.Database,
		&health.Table,
		&health.Engine,
		&health.TotalRows,
		&health.TotalBytes,
		&health.TotalBytesUncomp,
		&health.CompressionRatio,
		&health.PartitionCount,
		&health.ActivePartitions,
		&health.LastModificationAge,
		&health.LastModifyTime,
	)

	if err != nil {
		// Table might not exist or have no data yet
		if IsNoRows(err) {
			return nil, fmt.Errorf("table %s.%s not found or has no data", database, table)
		}
		return nil, fmt.Errorf("check table health for %s.%s: %w", database, table, err)
	}

	return &health, nil
}

// TableExists checks if a table exists in the database.
// This is useful for conditional table creation or migration logic.
//
// Example usage:
//
//	exists, err := client.TableExists(ctx, "mydb", "events")
//	if !exists {
//	    // Create table
//	}
func (c *Client) TableExists(ctx context.Context, database, table string) (bool, error) {
	query := `
		SELECT count()
		FROM system.tables
		WHERE database = ? AND name = ?
	`

	var count uint64
	err := c.QueryRow(ctx, query, database, table).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check if table exists %s.%s: %w", database, table, err)
	}

	return count > 0, nil
}

// OptimizeTable runs an OPTIMIZE TABLE command to force merges.
// This is useful after bulk inserts or for testing merge behavior.
//
// WARNING: OPTIMIZE can be expensive and block other operations.
// Use sparingly and prefer OPTIMIZE TABLE ... FINAL only when necessary.
//
// Example usage:
//
//	err := client.OptimizeTable(ctx, "mydb", "events", false)
func (c *Client) OptimizeTable(ctx context.Context, database, table string, final bool) error {
	query := fmt.Sprintf(`OPTIMIZE TABLE "%s"."%s" %s`, database, table, c.OnCluster())
	if final {
		query += " FINAL"
	}

	c.Logger.Info("Optimizing table",
		zap.String("database", database),
		zap.String("table", table),
		zap.Bool("final", final))

	if err := c.Exec(ctx, query); err != nil {
		return fmt.Errorf("optimize table %s.%s: %w", database, table, err)
	}

	return nil
}

// GetPoolConfigForComponent returns deterministic pool settings for each component.
// No environment variable overrides - fixed values for predictable behavior.
func GetPoolConfigForComponent(component string) *PoolConfig {
	var maxOpen, maxIdle int
	connMaxLifetime := 5 * time.Minute // Fixed 5-minute lifetime for all components

	// Component-specific fixed values (no env overrides)
	switch component {
	case "indexer_admin":
		maxOpen = 15
		maxIdle = 5
	case "indexer_chain":
		maxOpen = 40
		maxIdle = 15
	case "admin":
		maxOpen = 10
		maxIdle = 3
	case "admin_chain":
		maxOpen = 5
		maxIdle = 2
	case "controller":
		maxOpen = 10
		maxIdle = 3
	case "crosschain":
		maxOpen = 10
		maxIdle = 3
	default:
		// Unknown component - use legacy defaults with env overrides for backward compatibility
		maxOpen = utils.EnvInt("CLICKHOUSE_MAX_OPEN_CONNS", 75)
		maxIdle = utils.EnvInt("CLICKHOUSE_MAX_IDLE_CONNS", 75)
		// Parse connection lifetime from env for legacy components only
		lifetime := parseConnMaxLifetimeFromEnv()
		if lifetime > 0 {
			connMaxLifetime = lifetime
		}
	}

	// Enforce MaxIdleConns <= MaxOpenConns
	if maxIdle > maxOpen {
		maxIdle = maxOpen
	}

	return &PoolConfig{
		MaxOpenConns:    maxOpen,
		MaxIdleConns:    maxIdle,
		ConnMaxLifetime: connMaxLifetime,
		Component:       component,
	}
}

// parseConnMaxLifetimeFromEnv parses CLICKHOUSE_CONN_MAX_LIFETIME environment variable.
// Returns 0 if not set or invalid.
func parseConnMaxLifetimeFromEnv() time.Duration {
	val := os.Getenv("CLICKHOUSE_CONN_MAX_LIFETIME")
	if val == "" {
		return 0
	}

	duration, err := time.ParseDuration(val)
	if err != nil {
		return 0
	}

	return duration
}

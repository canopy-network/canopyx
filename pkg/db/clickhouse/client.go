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
	retryConfig := retry.DefaultConfig()

	dsn := utils.Env("CLICKHOUSE_ADDR", "clickhouse://localhost:9000?sslmode=disable")
	// Parse credentials and replica addresses from DSN
	username, password := extractCredentials(dsn)
	replicas := extractReplicas(dsn)

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

	// Parse connection strategy from environment
	// Strategies:
	//   - in_order: Always use first replica, fallback to others on failure
	//               Use for: Indexer (read-after-write consistency)
	//   - round_robin: Distribute connections evenly across all replicas
	//               Use for: SuperApp/API (read distribution, high throughput)
	//   - random: Random replica selection
	//               Use for: SuperApp/API (load balancing)
	connStrategy := parseConnOpenStrategy(utils.Env("CLICKHOUSE_CONN_STRATEGY", "in_order"))

	options := &clickhouse.Options{
		// Use array of replica addresses for failover
		Addr: replicas,

		// Connection strategy (configurable via CLICKHOUSE_CONN_STRATEGY)
		// Default: in_order for backward compatibility and indexer read-after-write consistency
		ConnOpenStrategy: connStrategy,

		Auth: clickhouse.Auth{
			Database: "default", // Connect to default database first
			Username: username,
			Password: password,
		},
		DialTimeout:     30 * time.Second, // Increased for high-concurrency scenarios with parallel cleanup
		MaxOpenConns:    maxOpenConns,     // Configurable for testing
		MaxIdleConns:    maxIdleConns,     // Configurable for testing
		ConnMaxLifetime: connMaxLifetime,  // Configurable for testing
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
		Settings: clickhouse.Settings{
			"prefer_column_name_to_alias":    1,
			"allow_experimental_object_type": 1,
			// Replication consistency: NOT NEEDED with ConnOpenInOrder
			// ConnOpenInOrder ensures same-replica routing for read-after-write consistency
			// Only use WithSequentialConsistency() for rare edge cases where you cannot
			// guarantee same-replica routing (e.g., cross-service reads)
		},
		Debug: false,
	}

	if debugEnabled {
		sugar := logger.Named("clickhouse.driver").Sugar()
		options.Debugf = sugar.Debugf
	}

	err := retry.WithBackoff(connCtx, retryConfig, logger, "clickhouse_connection", func() error {
		// Open connection to a default database
		conn, err := clickhouse.Open(options)
		if err != nil {
			return fmt.Errorf("failed to open clickhouse connection: %w", err)
		}

		client.Db = conn

		client.Logger.Debug("Pinging ClickHouse connection")
		err = client.Db.Ping(connCtx)
		if err != nil {
			return fmt.Errorf("failed to ping clickhouse: %w", err)
		}

		// NOTE: Keep connection to 'default' database for now
		// The wrapper's InitializeDB() will create the target database, then switch to it
		// This avoids the chicken-and-egg problem where we can't connect to a non-existent database
		client.Db = conn
		client.TargetDatabase = dbName // Store target database name for later use

		client.Logger.Info("ClickHouse connection pool configured",
			zap.String("database", dbName),
			zap.String("component", config.Component),
			zap.Strings("replicas", replicas),
			zap.String("conn_strategy", formatConnOpenStrategy(connStrategy)),
			zap.Int("max_open_conns", maxOpenConns),
			zap.Int("max_idle_conns", maxIdleConns),
			zap.Duration("conn_max_lifetime", connMaxLifetime),
		)
		return nil
	})

	if err != nil {
		return Client{}, err
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

// WithInsertQuorum is REMOVED - not needed with ConnOpenInOrder connection strategy.
// ConnOpenInOrder ensures we read from the same replica we wrote to, providing
// read-after-write consistency without the overhead and timeout issues of quorum.

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

// extractReplicas parses comma-separated replica addresses from DSN
// Supports formats:
//   - Single host: clickhouse://user:pass@host:9000/db
//   - Multiple hosts: clickhouse://user:pass@host1:9000,host2:9000/db
//   - With query params: clickhouse://user:pass@host1:9000,host2:9000/db?sslmode=disable
func extractReplicas(dsn string) []string {
	// Remove protocol prefix
	cleaned := strings.TrimPrefix(dsn, "clickhouse://")
	cleaned = strings.TrimPrefix(cleaned, "tcp://")

	// Extract host portion (between @ and / or ?)
	hostPart := cleaned
	if idx := strings.Index(cleaned, "@"); idx != -1 {
		hostPart = cleaned[idx+1:]
	}
	if idx := strings.IndexAny(hostPart, "/?"); idx != -1 {
		hostPart = hostPart[:idx]
	}

	// Split on comma for multiple replicas
	replicas := strings.Split(hostPart, ",")

	// Clean up and validate
	result := make([]string, 0, len(replicas))
	for _, r := range replicas {
		r = strings.TrimSpace(r)
		if r != "" {
			result = append(result, r)
		}
	}

	if len(result) == 0 {
		return []string{"localhost:9000"}
	}

	return result
}

// extractCredentials extracts username and password from a DSN string
// Format: clickhouse://username:password@host:port/...
// Returns: username, password (defaults to "default" and "" if not found)
func extractCredentials(dsn string) (string, string) {
	// Remove protocol prefix
	dsn = strings.TrimPrefix(dsn, "clickhouse://")
	dsn = strings.TrimPrefix(dsn, "tcp://")

	// Check if credentials are present (format: username:password@...)
	atIdx := strings.Index(dsn, "@")
	if atIdx == -1 {
		// No credentials in DSN, use defaults
		return "default", ""
	}

	// Extract credentials part (everything before @)
	credentials := dsn[:atIdx]

	// Split username:password
	colonIdx := strings.Index(credentials, ":")
	if colonIdx == -1 {
		// Only username provided, no password
		return credentials, ""
	}

	username := credentials[:colonIdx]
	password := credentials[colonIdx+1:]

	return username, password
}

// Exec Helper method to execute raw SQL queries
func (c *Client) Exec(ctx context.Context, query string, args ...interface{}) error {
	return c.Db.Exec(ctx, query, args...)
}

// QueryRow Helper method to query a single row
func (c *Client) QueryRow(ctx context.Context, query string, args ...interface{}) driver.Row {
	return c.Db.QueryRow(ctx, query, args...)
}

// Query Helper method to query multiple rows
func (c *Client) Query(ctx context.Context, query string, args ...interface{}) (driver.Rows, error) {
	return c.Db.Query(ctx, query, args...)
}

// Select Helper method to select into a slice
func (c *Client) Select(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return c.Db.Select(ctx, dest, query, args...)
}

// PrepareBatch Helper method for batch inserts
func (c *Client) PrepareBatch(ctx context.Context, query string) (driver.Batch, error) {
	return c.Db.PrepareBatch(ctx, query)
}

// Close Helper method to close the connection
func (c *Client) Close() error {
	return c.Db.Close()
}

// SwitchToTargetDatabase closes the current connection and reconnects to the TargetDatabase.
// This is useful when New() connected to 'default' database and you want to switch to
// the actual target database without calling InitializeDB().
// Returns an error if TargetDatabase is not set or if reconnection fails.
func (c *Client) SwitchToTargetDatabase(ctx context.Context) error {
	if c.TargetDatabase == "" {
		return errors.New("TargetDatabase is not set")
	}

	// Re-parse the DSN to get connection options
	dsn := utils.Env("CLICKHOUSE_ADDR", "clickhouse://localhost:9000")
	options, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("failed to parse CLICKHOUSE_ADDR DSN: %w", err)
	}

	// Close the current connection
	if err := c.Db.Close(); err != nil {
		c.Logger.Warn("Failed to close existing connection during database switch", zap.Error(err))
	}

	// Set the target database and reconnect
	options.Auth.Database = c.TargetDatabase
	options.DialTimeout = 30 * time.Second

	// Set compression if not already set
	if options.Compression == nil {
		options.Compression = &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		}
	}

	conn, err := clickhouse.Open(options)
	if err != nil {
		return fmt.Errorf("failed to open connection to database %s: %w", c.TargetDatabase, err)
	}

	// Verify connection
	if err := conn.Ping(ctx); err != nil {
		conn.Close()
		return fmt.Errorf("failed to ping database %s: %w", c.TargetDatabase, err)
	}

	c.Db = conn
	c.Logger.Info("Switched to target database", zap.String("database", c.TargetDatabase))

	return nil
}

// OnCluster returns ON CLUSTER statement
// This is required to force the replicas sync on some operations: https://clickhouse.com/docs/sql-reference/distributed-ddl
func (c *Client) OnCluster() string {
	return "ON CLUSTER canopyx"
}

// DbEngine returns the database engine type as a string.
func (c *Client) DbEngine() string {
	return "ENGINE = Atomic"
}

// CreateDbIfNotExists ensures that the specified database exists by creating it if it does not already exist.
func (c *Client) CreateDbIfNotExists(ctx context.Context, dbName string) error {
	// The expected result will be:
	// CREATE DATABASE IF NOT EXISTS canopyx_indexer ON CLUSTER canopyx ENGINE = Atomic
	query := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s %s %s", dbName, c.OnCluster(), c.DbEngine())
	c.Logger.Info("Creating admin database", zap.String("database", dbName), zap.String("query", query))
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

// PartitionInfo represents metadata about a ClickHouse table partition
type PartitionInfo struct {
	Database       string    `ch:"database"`
	Table          string    `ch:"table"`
	Partition      string    `ch:"partition"`
	PartitionID    string    `ch:"partition_id"`
	Rows           uint64    `ch:"rows"`
	Bytes          uint64    `ch:"bytes"`
	MinDate        time.Time `ch:"min_date"`
	MaxDate        time.Time `ch:"max_date"`
	ModifyTime     time.Time `ch:"modification_time"`
	RemoveTime     time.Time `ch:"remove_time"`
	Active         uint8     `ch:"active"`
	MinBlockNumber uint64    `ch:"min_block_number"`
	MaxBlockNumber uint64    `ch:"max_block_number"`
}

// GetPartitions retrieves partition metadata for a given table.
// This is useful for monitoring partition sizes, planning partition drops, and debugging.
//
// Example usage:
//
//	partitions, err := client.GetPartitions(ctx, "mydb", "events")
//	for _, p := range partitions {
//	    fmt.Printf("Partition %s: %d rows, %d bytes\n", p.Partition, p.Rows, p.Bytes)
//	}
func (c *Client) GetPartitions(ctx context.Context, database, table string) ([]PartitionInfo, error) {
	query := `
		SELECT
			database,
			table,
			partition,
			partition_id,
			rows,
			bytes,
			min_date,
			max_date,
			modification_time,
			remove_time,
			active,
			min_block_number,
			max_block_number
		FROM system.parts
		WHERE database = ? AND table = ? AND active = 1
		ORDER BY partition
	`

	var partitions []PartitionInfo
	err := c.Select(ctx, &partitions, query, database, table)
	if err != nil {
		return nil, fmt.Errorf("get partitions for %s.%s: %w", database, table, err)
	}

	return partitions, nil
}

// DropOldPartitions drops partitions older than the specified retention period.
// This is essential for managing disk space in time-series data.
//
// WARNING: This operation is irreversible. Ensure you have backups or understand
// your retention requirements before calling this.
//
// Example usage:
//
//	// Drop partitions older than 90 days
//	dropped, err := client.DropOldPartitions(ctx, "mydb", "events", 90*24*time.Hour)
func (c *Client) DropOldPartitions(ctx context.Context, database, table string, retention time.Duration) ([]string, error) {
	// Get all partitions
	partitions, err := c.GetPartitions(ctx, database, table)
	if err != nil {
		return nil, err
	}

	cutoffTime := time.Now().Add(-retention)
	droppedPartitions := make([]string, 0)

	for _, p := range partitions {
		// Check if partition's max_date is older than retention period
		if p.MaxDate.Before(cutoffTime) {
			// CRITICAL: Uses ON CLUSTER with replication_alter_partitions_sync setting
			// replication_alter_partitions_sync=2 ensures partition drop completes on all replicas before returning
			// Possible values: 0=async, 1=wait for local only (default), 2=wait for all replicas
			dropQuery := fmt.Sprintf(`ALTER TABLE "%s"."%s" ON CLUSTER canopyx DROP PARTITION '%s' SETTINGS replication_alter_partitions_sync = 2`, database, table, p.Partition)
			c.Logger.Info("Dropping old partition",
				zap.String("database", database),
				zap.String("table", table),
				zap.String("partition", p.Partition),
				zap.Time("max_date", p.MaxDate),
				zap.Uint64("rows", p.Rows))

			if err := c.Exec(ctx, dropQuery); err != nil {
				return droppedPartitions, fmt.Errorf("drop partition %s: %w", p.Partition, err)
			}

			droppedPartitions = append(droppedPartitions, p.Partition)
		}
	}

	return droppedPartitions, nil
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
	// CRITICAL: Uses ON CLUSTER with alter_sync setting to ensure optimization completes on all replicas
	// alter_sync=2 waits for completion on all replicas (0=async, 1=local only, 2=all replicas)
	query := fmt.Sprintf(`OPTIMIZE TABLE "%s"."%s" %s`, database, table, c.OnCluster())
	if final {
		query += " FINAL"
	}
	query += " SETTINGS alter_sync = 2"

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
	connMaxLifetime := 5 * time.Minute // Fixed 5 minute lifetime for all components

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

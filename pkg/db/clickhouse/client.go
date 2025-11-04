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
	Logger *zap.Logger
	Db     driver.Conn
}

// New initializes and returns a new database client for ClickHouse with provided context and logger.
// Includes connection pooling optimizations for high-throughput workloads.
func New(ctx context.Context, logger *zap.Logger, dbName string) (client Client, e error) {
	// Add timeout to context for initial connection
	connCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	client.Logger = logger
	retryConfig := retry.DefaultConfig()

	err := retry.WithBackoff(connCtx, retryConfig, logger, "clickhouse_connection", func() error {
		dsn := utils.Env("CLICKHOUSE_ADDR", "clickhouse://localhost:9000?sslmode=disable")

		// First, connect without specifying a database to create it
		debugEnabled := logger != nil && logger.Core().Enabled(zap.DebugLevel)

		// Connection pool settings (configurable via environment variables for testing)
		maxOpenConns := utils.EnvInt("CLICKHOUSE_MAX_OPEN_CONNS", 75)
		maxIdleConns := utils.EnvInt("CLICKHOUSE_MAX_IDLE_CONNS", 75)

		// Parse ConnMaxLifetime from environment variable
		connMaxLifetime := time.Duration(0)
		if lifetimeStr := os.Getenv("CLICKHOUSE_CONN_MAX_LIFETIME"); lifetimeStr != "" {
			if d, err := time.ParseDuration(lifetimeStr); err == nil {
				connMaxLifetime = d
			}
		}

		options := &clickhouse.Options{
			Addr: []string{extractHost(dsn)},
			Auth: clickhouse.Auth{
				Database: "default", // Connect to default database first
				Username: "default",
				Password: "",
			},
			DialTimeout:     5 * time.Second,
			MaxOpenConns:    maxOpenConns,    // Configurable for testing
			MaxIdleConns:    maxIdleConns,    // Configurable for testing
			ConnMaxLifetime: connMaxLifetime, // Configurable for testing
			Compression: &clickhouse.Compression{
				Method: clickhouse.CompressionLZ4,
			},
			Settings: clickhouse.Settings{
				"prefer_column_name_to_alias":    1,
				"allow_experimental_object_type": 1,
			},
			Debug: debugEnabled,
		}

		if debugEnabled {
			sugar := logger.Named("clickhouse.driver").Sugar()
			options.Debugf = sugar.Debugf
		}

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

		// Create a database if it doesn't exist
		createDbQuery := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", dbName)
		client.Logger.Debug("Creating database if not exists", zap.String("database", dbName))
		err = client.Db.Exec(connCtx, createDbQuery)
		if err != nil {
			return fmt.Errorf("failed to create database %s: %w", dbName, err)
		}

		// Close the default connection
		err = conn.Close()
		if err != nil {
			client.Logger.Error("Failed to close default connection", zap.Error(err))
			return err
		}

		// Now reconnect to the specific database
		options.Auth.Database = dbName
		conn, err = clickhouse.Open(options)
		if err != nil {
			return fmt.Errorf("failed to open clickhouse connection to %s: %w", dbName, err)
		}

		client.Db = conn

		// Verify connection to the new database
		err = client.Db.Ping(connCtx)
		if err != nil {
			return fmt.Errorf("failed to ping database %s: %w", dbName, err)
		}

		client.Logger.Debug("ClickHouse connection ready to work", zap.String("database", dbName))
		return nil
	})

	if err != nil {
		return Client{}, err
	}

	return client, nil
}

// SanitizeName sanitizes the provided database name to be compatible with ClickHouse.
func SanitizeName(id string) string {
	s := strings.ToLower(id)
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	return s
}

// extractHost extracts the host from a DSN string
func extractHost(dsn string) string {
	// Remove protocol prefix
	dsn = strings.TrimPrefix(dsn, "clickhouse://")
	dsn = strings.TrimPrefix(dsn, "tcp://")

	// Find the end of host (either / or ?)
	if idx := strings.IndexAny(dsn, "/?"); idx != -1 {
		dsn = dsn[:idx]
	}

	// Remove any credentials
	if idx := strings.Index(dsn, "@"); idx != -1 {
		dsn = dsn[idx+1:]
	}

	// Default to localhost:9000 if empty
	if dsn == "" {
		return "localhost:9000"
	}

	return dsn
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

// IsNoRows Helper to check if the error is no rows
func IsNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

// QueryWithFinal executes a query with FINAL modifier for ReplacingMergeTree tables.
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
	// Check if query already has FINAL keyword
	queryUpper := strings.ToUpper(query)
	if !strings.Contains(queryUpper, "FINAL") {
		c.Logger.Warn("QueryWithFinal called but query doesn't contain FINAL keyword - ensure FINAL is placed after table name")
	}
	return c.Db.Query(ctx, query, args...)
}

// SelectWithFinal executes a Select query with FINAL modifier for ReplacingMergeTree tables.
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
	// Check if query already has FINAL keyword
	queryUpper := strings.ToUpper(query)
	if !strings.Contains(queryUpper, "FINAL") {
		c.Logger.Warn("SelectWithFinal called but query doesn't contain FINAL keyword - ensure FINAL is placed after table name")
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
			dropQuery := fmt.Sprintf(`ALTER TABLE "%s"."%s" DROP PARTITION '%s'`, database, table, p.Partition)
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
	query := fmt.Sprintf(`OPTIMIZE TABLE "%s"."%s"`, database, table)
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

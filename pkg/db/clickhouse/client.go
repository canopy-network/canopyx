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

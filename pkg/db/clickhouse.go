package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/canopy-network/canopyx/pkg/utils"
	"go.uber.org/zap"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type Client struct {
	Logger *zap.Logger
	Db     driver.Conn
}

// NewDB initializes and returns a new database client for ClickHouse with provided context and logger.
// Includes connection pooling optimizations for high-throughput workloads.
func NewDB(ctx context.Context, logger *zap.Logger, dbName string) (client Client, e error) {
	dsn := utils.Env("CLICKHOUSE_ADDR", "clickhouse://localhost:9000?sslmode=disable")

	client.Logger = logger

	// First, connect without specifying a database to create it
	options := &clickhouse.Options{
		Addr: []string{extractHost(dsn)},
		Auth: clickhouse.Auth{
			Database: "default", // Connect to default database first
		},
		DialTimeout:     5 * time.Second,
		MaxOpenConns:    75, // Max open connections per indexer (increased for 12+ indexers)
		MaxIdleConns:    75, // Keep connections alive for reuse
		ConnMaxLifetime: 0,  // Connections never expire (default: 0)
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
		Settings: clickhouse.Settings{
			"prefer_column_name_to_alias":    1,
			"allow_experimental_object_type": 1,
		},
		Debug: utils.Env("CHDEBUG", "") != "", // Enable debug if CHDEBUG is set
	}

	// Open connection to default database
	conn, err := clickhouse.Open(options)
	if err != nil {
		return client, fmt.Errorf("failed to open clickhouse connection: %w", err)
	}

	client.Db = conn

	logger.Info("Pinging ClickHouse connection")
	err = client.Db.Ping(ctx)
	if err != nil {
		return client, fmt.Errorf("failed to ping clickhouse: %w", err)
	}

	// Create database if it doesn't exist
	createDbQuery := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", dbName)
	logger.Info("Creating database if not exists", zap.String("database", dbName))
	err = client.Db.Exec(ctx, createDbQuery)
	if err != nil {
		return client, fmt.Errorf("failed to create database %s: %w", dbName, err)
	}

	// Close the default connection
	err = conn.Close()
	if err != nil {
		logger.Error("Failed to close default connection", zap.Error(err))
		return Client{}, err
	}

	// Now reconnect to the specific database
	options.Auth.Database = dbName
	conn, err = clickhouse.Open(options)
	if err != nil {
		return client, fmt.Errorf("failed to open clickhouse connection to %s: %w", dbName, err)
	}

	client.Db = conn

	// Verify connection to the new database
	err = client.Db.Ping(ctx)
	if err != nil {
		return client, fmt.Errorf("failed to ping database %s: %w", dbName, err)
	}

	logger.Info("ClickHouse connection ready to work", zap.String("database", dbName))
	return client, nil
}

// NewBasicDbs creates and returns a new instance of AdminDB and ReportsDB configured with the provided client.
func NewBasicDbs(ctx context.Context, logger *zap.Logger) (*AdminDB, *ReportsDB, error) {
	// Database name for the db that hold chains to be indexed and the tracking of them
	indexerDbName := utils.Env("INDEXER_DB", "canopyx_indexer")
	// Database name for the db that holds reports across chains
	reportsDbName := utils.Env("REPORTS_DB", "canopyx_reports")

	logger.Info("Creating databases", zap.String("indexerDbName", indexerDbName), zap.String("reportsDbName", reportsDbName))
	indexerDb, indexerDbErr := NewDB(ctx, logger, indexerDbName)
	if indexerDbErr != nil {
		return nil, nil, indexerDbErr
	}

	reportsDb, reportsDbErr := NewDB(ctx, logger, reportsDbName)
	if reportsDbErr != nil {
		return nil, nil, reportsDbErr
	}

	return &AdminDB{
			Client: indexerDb,
			Name:   indexerDbName,
		}, &ReportsDB{
			Client: reportsDb,
			Name:   reportsDbName,
		}, nil
}

// NewChainDb creates and returns a new instance of ChainDB configured with the provided client.
func NewChainDb(ctx context.Context, logger *zap.Logger, chainId string) (*ChainDB, error) {
	dbName := SanitizeDbName(chainId)
	chainDb, chainDbErr := NewDB(ctx, logger.With(
		zap.String("db", dbName),
		zap.String("component", "chain_db"),
		zap.String("chainID", chainId),
	), dbName)
	if chainDbErr != nil {
		return nil, chainDbErr
	}

	chainDbWrapper := &ChainDB{
		Client:  chainDb,
		Name:    dbName,
		ChainID: chainId,
	}

	chainDbInitErr := chainDbWrapper.InitializeDB(ctx)
	if chainDbInitErr != nil {
		return nil, chainDbInitErr
	}

	return chainDbWrapper, nil
}

// SanitizeDbName sanitizes the provided database name to be compatible with ClickHouse.
func SanitizeDbName(id string) string {
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

// IsNoRows Helper to check if error is no rows
func IsNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

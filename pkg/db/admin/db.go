package admin

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	"go.uber.org/zap"
)

// DB represents a database connection for handling indexing operations and tracking progress.
type DB struct {
	clickhouse.Client
	Name string
}

// NewWithPoolConfig creates and initializes an admin database instance with custom pool configuration.
// This allows passing calculated pool sizes directly instead of relying on environment variables.
func NewWithPoolConfig(ctx context.Context, logger *zap.Logger, name string, poolConfig clickhouse.PoolConfig) (*DB, error) {
	client, err := clickhouse.New(ctx, logger.With(
		zap.String("db", name),
		zap.String("component", poolConfig.Component),
	), name, &poolConfig)
	if err != nil {
		return nil, err
	}

	adminDB := &DB{
		Client: client,
		Name:   name,
	}

	if err := adminDB.InitializeDB(ctx); err != nil {
		return nil, err
	}

	return adminDB, nil
}

// Close terminates the underlying ClickHouse connection.
func (db *DB) Close() error {
	return db.Db.Close()
}

func (db *DB) GetConnection() driver.Conn {
	return db.Db
}

// DatabaseName returns the name of the cross-chain database
func (db *DB) DatabaseName() string {
	return db.Name
}

// DropChainDatabase drops the chain-specific database.
// Uses DROP DATABASE IF EXISTS, which is safe to call even if the database doesn't exist.
func (db *DB) DropChainDatabase(ctx context.Context, chainID uint64) error {
	dbName := clickhouse.SanitizeName(fmt.Sprintf("chain_%d", chainID))

	// Now drop the database on all cluster nodes
	// SYNC ensures drop completes on all replicas before returning
	query := fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" %s SYNC`, dbName, db.OnCluster())
	if err := db.Db.Exec(ctx, query); err != nil {
		return fmt.Errorf("drop chain database %s: %w", dbName, err)
	}
	return nil
}

// InitializeDB ensures the required database and tables for indexing are created if they do not already exist.
func (db *DB) InitializeDB(ctx context.Context) error {
	db.Logger.Info("Initializing indexer database", zap.String("database", db.Name))

	if err := db.CreateDbIfNotExists(ctx, db.Name); err != nil {
		return fmt.Errorf("failed to create database %s: %w", db.Name, err)
	}
	db.Logger.Info("Database created successfully", zap.String("database", db.Name))

	db.Logger.Info("Initialize chains table", zap.String("database", db.Name))
	if err := db.initChains(ctx); err != nil {
		return err
	}

	db.Logger.Info("Initialize index_progress model", zap.String("name", db.Name))
	err := db.initIndexProgress(ctx)
	if err != nil {
		return err
	}

	db.Logger.Debug("Initialize reindex request model", zap.String("name", db.Name))
	if err := db.initReindexRequests(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize rpc_endpoints model", zap.String("name", db.Name))
	if err := db.initRPCEndpoints(ctx); err != nil {
		return err
	}

	return nil
}

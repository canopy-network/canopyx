package crosschain

import (
	"context"
	"fmt"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"go.uber.org/zap"
)

// DB manages the cross-chain global database for querying data across all chains.
// It provides methods for:
// - Schema initialization (global tables)
// - Per-chain sync setup (materialized views)
// - Sync removal and cleanup
// - Data backfilling and optimization
// - Health and sync status monitoring
type DB struct {
	clickhouse.Client
	Name string
}

// NewWithPoolConfig creates a cross-chain store with custom pool configuration.
// This allows passing calculated pool sizes directly instead of relying on environment variables.
func NewWithPoolConfig(ctx context.Context, logger *zap.Logger, dbName string, poolConfig clickhouse.PoolConfig) (*DB, error) {
	client, err := clickhouse.New(ctx, logger.With(
		zap.String("db", dbName),
		zap.String("component", poolConfig.Component),
	), dbName, &poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create cross-chain clickhouse client: %w", err)
	}

	store := &DB{
		Client: client,
		Name:   dbName,
	}

	return store, nil
}

// NewWithSharedClient creates a cross-chain store that reuses an existing ClickHouse connection pool.
// This is efficient for scenarios where the cross-chain DB is accessed as part of the same workload (e.g., admin maintenance).
// The database and tables must already exist - this constructor does NOT call InitializeDB.
// Use this when you want to avoid creating separate connection pools.
func NewWithSharedClient(client clickhouse.Client, dbName string) *DB {
	return &DB{
		Client: client, // Reuse existing connection pool
		Name:   dbName,
	}
}

// DatabaseName returns the name of the cross-chain database
func (db *DB) DatabaseName() string {
	return db.Name
}

// InitializeDB creates all 14 global tables if they don't exist.
// Tables use ReplacingMergeTree(height) engine for automatic deduplication.
// Height is used as the version column to ensure the highest block height wins,
// regardless of insertion order (protects against out-of-order materialized view writes).
// This operation is idempotent - safe to call multiple times.
// All tables are created in parallel for optimal performance.
func (db *DB) InitializeDB(ctx context.Context) error {
	db.Logger.Info("Initializing cross-chain database", zap.String("database", db.Name))

	// Create a database if it doesn't exist
	if err := db.CreateDbIfNotExists(ctx, db.Name); err != nil {
		return fmt.Errorf("failed to create database %s: %w", db.Name, err)
	}
	db.Logger.Info("Database created successfully", zap.String("database", db.Name))

	// Define all table initialization tasks
	configs := GetTableConfigs()

	for _, cfg := range configs {
		if err := db.createGlobalTable(ctx, cfg); err != nil {
			return err
		}
	}

	if err := db.initLPPositionSnapshots(ctx); err != nil {
		return err
	}

	if err := db.initTVLSnapshots(ctx); err != nil {
		return err
	}

	db.Logger.Info("Cross-chain schema initialization complete",
		zap.String("database", db.Name),
		zap.Int("total", len(configs)+2)) // +2 for LP position snapshots and TVL snapshots tables

	return nil
}

// createGlobalTable creates a global table with chain_id and updated_at columns.
// Uses ReplacingMergeTree for automatic deduplication based on height (version column).
// The height column is used as the version to ensure records with higher block heights
// are kept during deduplication, regardless of insertion order.
// Schema is copied from the source table definitions in pkg/db/chain/*.go
func (db *DB) createGlobalTable(ctx context.Context, cfg TableConfig) error {
	globalTableName := db.getGlobalTableName(cfg.TableName)

	// Build ORDER BY clause
	orderBy := strings.Join(cfg.PrimaryKey, ", ")

	// Build bloom filter index if table has address column
	bloomFilter := ""
	if cfg.HasAddressColumn {
		bloomFilter = ", INDEX address_bloom address TYPE bloom_filter GRANULARITY 4"
	}

	// Create a table with chain_id + entity columns + updated_at
	// Schema SQL is copied from pkg/db/chain/*.go init functions
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s"
		(
			chain_id UInt64,
			%s,
			updated_at DateTime64(6)
			%s
		)
		ENGINE = %s
		ORDER BY (%s)
	`, db.Name, globalTableName, cfg.SchemaSQL, bloomFilter, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"), orderBy)

	if err := db.Exec(ctx, query); err != nil {
		return fmt.Errorf("failed to create table %s: %w", globalTableName, err)
	}

	return nil
}

// getGlobalTableName generates the name of a global table by appending "_global" to the base table name.
func (db *DB) getGlobalTableName(baseTableName string) string {
	return baseTableName + "_global"
}

// getMaterializedViewName generates the name of a materialized view based on chain ID and base table name.
func (db *DB) getMaterializedViewName(chainID uint64, baseTableName string) string {
	return fmt.Sprintf("%s_sync_chain_%d", baseTableName, chainID)
}

// materializedViewExists checks if a materialized view exists in the specified database.
func (db *DB) materializedViewExists(ctx context.Context, dbName, viewName string) (bool, error) {
	query := `SELECT count() FROM system.tables WHERE database = ? AND name = ? AND engine LIKE '%MaterializedView%'`
	var count uint64
	err := db.QueryRow(ctx, query, dbName, viewName).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check view existence: %w", err)
	}
	return count > 0, nil
}

// createSyncMaterializedView creates a materialized view that syncs source table to global table.
// The materialized view automatically copies new rows as they are inserted into the source table.
// Uses explicit column names to ensure correct ordering.
func (db *DB) createSyncMaterializedView(ctx context.Context, chainID uint64, chainDBName string, cfg TableConfig) error {
	mvName := db.getMaterializedViewName(chainID, cfg.TableName)
	globalTableName := db.getGlobalTableName(cfg.TableName)
	sourceTableName := cfg.TableName

	// Verify source table exists
	sourceExists, err := db.TableExists(ctx, chainDBName, sourceTableName)
	if err != nil {
		return fmt.Errorf("failed to check if source table exists: %w", err)
	}
	if !sourceExists {
		db.Logger.Warn("Source table does not exist, skipping materialized view creation",
			zap.String("source_table", sourceTableName),
			zap.String("chain_db", chainDBName))
		return nil
	}

	// Build column list for SELECT using GetCrossChainSelectExprs
	// This handles CrossChainRename (e.g., "chain_id AS pool_chain_id" for pools table)
	selectExprs := indexer.GetCrossChainSelectExprs(GetColumnsForTable(cfg.TableName))
	columnList := strings.Join(selectExprs, ", ")

	// Create a materialized view that inserts into a global table
	// Use explicit column expressions to handle renamed columns
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."%s"
		TO "%s"."%s"
		AS SELECT
			? AS chain_id,
			%s,
			now64(6) AS updated_at
		FROM "%s"."%s"
	`, chainDBName, mvName, db.Name, globalTableName, columnList, chainDBName, sourceTableName)

	db.Logger.Debug("Creating cross-chain materialized view",
		zap.String("view", mvName),
		zap.String("chain_db", chainDBName),
		zap.Uint64("chain_id", chainID),
		zap.String("query", query))

	if err := db.Exec(ctx, query, chainID); err != nil {
		return fmt.Errorf("failed to create materialized view %s: %w", mvName, err)
	}

	db.Logger.Debug("Materialized view created successfully",
		zap.String("view", mvName),
		zap.String("source_table", sourceTableName),
		zap.String("global_table", globalTableName),
		zap.Uint64("chain_id", chainID))

	return nil
}

// validateTableName checks if the table name is in our whitelist of supported tables.
// This prevents SQL injection attacks via malicious table names.
func (db *DB) validateTableName(tableName string) error {
	validTables := make(map[string]bool)
	for _, cfg := range GetTableConfigs() {
		validTables[cfg.TableName] = true
	}

	if !validTables[tableName] {
		return fmt.Errorf("invalid table name: %s (not in whitelist)", tableName)
	}

	return nil
}

// getTableConfig retrieves the configuration for a specific table.
// Returns error if the table is not in the whitelist.
func (db *DB) getTableConfig(tableName string) (*TableConfig, error) {
	if err := db.validateTableName(tableName); err != nil {
		return nil, err
	}

	for _, cfg := range GetTableConfigs() {
		if cfg.TableName == tableName {
			return &cfg, nil
		}
	}

	// Should never reach here if validateTableName passed
	return nil, fmt.Errorf("table config not found: %s", tableName)
}

// OptimizeTables runs OPTIMIZE TABLE FINAL on all global tables.
// This forces the final merge of ReplacingMergeTree tables, removing old versions.
// Should be run periodically (e.g., weekly) to reduce storage and improve query performance.
// This operation can take significant time on large tables.
func (db *DB) OptimizeTables(ctx context.Context) error {
	db.Logger.Info("Optimizing cross-chain global tables", zap.String("database", db.Name))

	configs := GetTableConfigs()
	for _, cfg := range configs {
		globalTableName := db.getGlobalTableName(cfg.TableName)
		if err := db.OptimizeTable(ctx, db.Name, globalTableName, true); err != nil {
			db.Logger.Error("Failed to optimize table",
				zap.String("table", globalTableName),
				zap.Error(err))
			// Continue with other tables even if one fails
		} else {
			db.Logger.Info("Table optimized",
				zap.String("table", globalTableName))
		}
	}

	db.Logger.Info("Optimization complete for all global tables")
	return nil
}

// OptimizeTable runs OPTIMIZE TABLE FINAL on a specific table.
func (db *DB) OptimizeTable(ctx context.Context, database, table string, final bool) error {
	return db.Client.OptimizeTable(ctx, database, table, final)
}

// Close terminates the underlying ClickHouse connection.
func (db *DB) Close() error {
	return db.Db.Close()
}

func (db *DB) GetConnection() driver.Conn {
	return db.Db
}

func (db *DB) Exec(ctx context.Context, query string, args ...interface{}) error {
	return db.Db.Exec(ctx, query, args...)
}

func (db *DB) QueryRow(ctx context.Context, query string, args ...interface{}) driver.Row {
	return db.Db.QueryRow(ctx, query, args...)
}

func (db *DB) Query(ctx context.Context, query string, args ...interface{}) (driver.Rows, error) {
	return db.Db.Query(ctx, query, args...)
}

func (db *DB) Select(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return db.Db.Select(ctx, dest, query, args...)
}

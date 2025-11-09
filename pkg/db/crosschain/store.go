package crosschain

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"go.uber.org/zap"
)

// Store manages the cross-chain global database for querying data across all chains.
// It provides methods for:
// - Schema initialization (global tables)
// - Per-chain sync setup (materialized views)
// - Sync removal and cleanup
// - Data backfilling and optimization
// - Health and sync status monitoring
type Store struct {
	clickhouse.Client
	Name string
}

// DatabaseName returns the name of the cross-chain database
func (s *Store) DatabaseName() string {
	return s.Name
}

// TableConfig defines the schema configuration for a global table.
type TableConfig struct {
	TableName        string   // Base table name (e.g., "accounts")
	PrimaryKey       []string // ORDER BY columns
	HasAddressColumn bool     // Whether to add bloom filter on address
	SchemaSQL        string   // Full CREATE TABLE schema (columns only, without ENGINE/ORDER BY)
	ColumnNames      []string // Explicit column names for INSERT/SELECT
}

// GetTableConfigs returns the configuration for all 12 entities to sync.
// Each config is now derived from the ColumnDef definitions in pkg/db/models/indexer/*.go
func GetTableConfigs() []TableConfig {
	return []TableConfig{
		{
			TableName:        indexer.AccountsProductionTableName,
			PrimaryKey:       []string{"chain_id", "address"},
			HasAddressColumn: true,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.AccountColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.AccountColumns),
		},
		{
			TableName:        indexer.ValidatorsProductionTableName,
			PrimaryKey:       []string{"chain_id", "address"},
			HasAddressColumn: true,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.ValidatorColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.ValidatorColumns),
		},
		{
			TableName:        indexer.ValidatorSigningInfoProductionTableName,
			PrimaryKey:       []string{"chain_id", "address"},
			HasAddressColumn: true,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.ValidatorSigningInfoColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.ValidatorSigningInfoColumns),
		},
		{
			TableName:        indexer.ValidatorDoubleSigningInfoProductionTableName,
			PrimaryKey:       []string{"chain_id", "address"},
			HasAddressColumn: true,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.ValidatorDoubleSigningInfoColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.ValidatorDoubleSigningInfoColumns),
		},
		{
			TableName:        indexer.PoolsProductionTableName,
			PrimaryKey:       []string{"chain_id", "pool_id"},
			HasAddressColumn: false,
			SchemaSQL:        indexer.ColumnsToCrossChainSchemaSQL(indexer.PoolColumns),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.PoolColumns),
		},
		{
			TableName:        indexer.PoolPointsByHolderProductionTableName,
			PrimaryKey:       []string{"chain_id", "address", "pool_id"},
			HasAddressColumn: true,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.PoolPointsByHolderColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.PoolPointsByHolderColumns),
		},
		{
			TableName:        indexer.OrdersProductionTableName,
			PrimaryKey:       []string{"chain_id", "order_id"},
			HasAddressColumn: false,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.OrderColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.OrderColumns),
		},
		{
			TableName:        indexer.DexOrdersProductionTableName,
			PrimaryKey:       []string{"chain_id", "order_id"},
			HasAddressColumn: false,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.DexOrderColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.DexOrderColumns),
		},
		{
			TableName:        indexer.DexDepositsProductionTableName,
			PrimaryKey:       []string{"chain_id", "order_id"},
			HasAddressColumn: false,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.DexDepositColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.DexDepositColumns),
		},
		{
			TableName:        indexer.DexWithdrawalsProductionTableName,
			PrimaryKey:       []string{"chain_id", "order_id"},
			HasAddressColumn: false,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.DexWithdrawalColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.DexWithdrawalColumns),
		},
		{
			TableName:        indexer.BlockSummariesProductionTableName,
			PrimaryKey:       []string{"chain_id", "height"},
			HasAddressColumn: false,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.BlockSummaryColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.BlockSummaryColumns),
		},
		{
			TableName:        indexer.CommitteePaymentsProductionTableName,
			PrimaryKey:       []string{"chain_id", "committee_id", "address", "height"},
			HasAddressColumn: true,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.CommitteePaymentColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.CommitteePaymentColumns),
		},
	}
}

// NewStore creates a new cross-chain store instance with the specified database name.
// This function is idempotent - safe to call multiple times.
func NewStore(ctx context.Context, logger *zap.Logger, dbName string) (*Store, error) {
	client, err := clickhouse.New(ctx, logger.With(
		zap.String("db", dbName),
		zap.String("component", "crosschain_db"),
	), dbName)
	if err != nil {
		return nil, fmt.Errorf("failed to create cross-chain clickhouse client: %w", err)
	}

	store := &Store{
		Client: client,
		Name:   dbName,
	}

	return store, nil
}

// InitializeSchema creates all 12 global tables if they don't exist.
// Tables use ReplacingMergeTree(height) engine for automatic deduplication.
// Height is used as the version column to ensure the highest block height wins,
// regardless of insertion order (protects against out-of-order materialized view writes).
// This operation is idempotent - safe to call multiple times.
func (s *Store) InitializeSchema(ctx context.Context) error {
	s.Logger.Info("Initializing cross-chain schema", zap.String("database", s.Name))

	configs := GetTableConfigs()
	for _, cfg := range configs {
		if err := s.createGlobalTable(ctx, cfg); err != nil {
			return fmt.Errorf("failed to create global table %s: %w", cfg.TableName, err)
		}
		s.Logger.Info("Global table ready",
			zap.String("table", s.getGlobalTableName(cfg.TableName)),
			zap.String("database", s.Name))

		// Validate schema matches column definitions (fail-fast on mismatch)
		if err := s.validateTableSchema(ctx, cfg); err != nil {
			return fmt.Errorf("schema validation failed for table %s: %w", cfg.TableName, err)
		}
	}

	s.Logger.Info("Cross-chain schema initialized successfully", zap.String("database", s.Name))
	return nil
}

// createGlobalTable creates a global table with chain_id and updated_at columns.
// Uses ReplacingMergeTree for automatic deduplication based on height (version column).
// The height column is used as the version to ensure records with higher block heights
// are kept during deduplication, regardless of insertion order.
// Schema is copied from the source table definitions in pkg/db/chain/*.go
func (s *Store) createGlobalTable(ctx context.Context, cfg TableConfig) error {
	globalTableName := s.getGlobalTableName(cfg.TableName)

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
		ENGINE = ReplacingMergeTree(height)
		ORDER BY (%s)
	`, s.Name, globalTableName, cfg.SchemaSQL, bloomFilter, orderBy)

	if err := s.Exec(ctx, query); err != nil {
		return fmt.Errorf("failed to create table %s: %w", globalTableName, err)
	}

	return nil
}

// validateTableSchema verifies that the deployed table schema matches our column definitions.
// This provides fail-fast validation to prevent schema drift between code and database.
// Returns an error with detailed mismatch information if validation fails.
func (s *Store) validateTableSchema(ctx context.Context, cfg TableConfig) error {
	globalTableName := s.getGlobalTableName(cfg.TableName)

	// Query the table's column definitions from ClickHouse system tables
	query := `
		SELECT name, type
		FROM system.columns
		WHERE database = ? AND table = ?
		ORDER BY position
	`

	rows, err := s.Query(ctx, query, s.Name, globalTableName)
	if err != nil {
		return fmt.Errorf("failed to query table schema: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Build map of actual columns in database
	actualColumns := make(map[string]string) // column_name -> type
	for rows.Next() {
		var name, colType string
		if err := rows.Scan(&name, &colType); err != nil {
			return fmt.Errorf("failed to scan column info: %w", err)
		}
		actualColumns[name] = colType
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating rows: %w", err)
	}

	// Verify expected columns exist
	// Note: We skip 'chain_id' and 'updated_at' as they're added by createGlobalTable
	for _, colName := range cfg.ColumnNames {
		if _, exists := actualColumns[colName]; !exists {
			return fmt.Errorf("column '%s' is defined in code but missing from database table '%s'", colName, globalTableName)
		}
	}

	// Log successful validation
	s.Logger.Debug("Schema validation passed",
		zap.String("table", globalTableName),
		zap.Int("columns", len(cfg.ColumnNames)))

	return nil
}

// SetupChainSync creates 12 materialized views to sync a chain's data to global tables.
// Materialized views automatically copy new data as it arrives in the source chain database.
// This operation is idempotent - safe to call multiple times.
//
// IMPORTANT: This only syncs NEW data. For existing data, call ResyncChain after setup.
func (s *Store) SetupChainSync(ctx context.Context, chainID uint64) error {
	s.Logger.Info("Setting up cross-chain sync for chain",
		zap.Uint64("chain_id", chainID),
		zap.String("target_db", s.Name))

	chainDBName := fmt.Sprintf("chain_%d", chainID)

	// Verify source chain database exists
	exists, err := s.chainDatabaseExists(ctx, chainDBName)
	if err != nil {
		return fmt.Errorf("failed to check if chain database exists: %w", err)
	}
	if !exists {
		return fmt.Errorf("chain database %s does not exist", chainDBName)
	}

	configs := GetTableConfigs()
	for _, cfg := range configs {
		if err := s.createSyncMaterializedView(ctx, chainID, chainDBName, cfg); err != nil {
			s.Logger.Error("Failed to create materialized view",
				zap.Uint64("chain_id", chainID),
				zap.String("table", cfg.TableName),
				zap.Error(err))
			return fmt.Errorf("failed to create sync view for %s: %w", cfg.TableName, err)
		}
	}

	s.Logger.Info("Cross-chain sync setup complete for chain",
		zap.Uint64("chain_id", chainID),
		zap.Int("tables_synced", len(configs)))

	return nil
}

// createSyncMaterializedView creates a materialized view that syncs source table to global table.
// The materialized view automatically copies new rows as they are inserted into the source table.
// Uses explicit column names to ensure correct ordering.
func (s *Store) createSyncMaterializedView(ctx context.Context, chainID uint64, chainDBName string, cfg TableConfig) error {
	mvName := s.getMaterializedViewName(chainID, cfg.TableName)
	globalTableName := s.getGlobalTableName(cfg.TableName)
	sourceTableName := cfg.TableName

	// Verify source table exists
	sourceExists, err := s.TableExists(ctx, chainDBName, sourceTableName)
	if err != nil {
		return fmt.Errorf("failed to check if source table exists: %w", err)
	}
	if !sourceExists {
		s.Logger.Warn("Source table does not exist, skipping materialized view creation",
			zap.String("source_table", sourceTableName),
			zap.String("chain_db", chainDBName))
		return nil
	}

	// Build column list for SELECT using GetCrossChainSelectExprs
	// This handles CrossChainRename (e.g., "chain_id AS pool_chain_id" for pools table)
	selectExprs := indexer.GetCrossChainSelectExprs(getColumnsForTable(cfg.TableName))
	columnList := strings.Join(selectExprs, ", ")

	// Create materialized view that inserts into global table
	// Use explicit column expressions to handle renamed columns
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."%s"
		TO "%s"."%s"
		AS SELECT
			? AS chain_id,
			%s,
			now64(6) AS updated_at
		FROM "%s"."%s"
	`, chainDBName, mvName, s.Name, globalTableName, columnList, chainDBName, sourceTableName)

	if err := s.Exec(ctx, query, chainID); err != nil {
		return fmt.Errorf("failed to create materialized view %s: %w", mvName, err)
	}

	s.Logger.Debug("Materialized view created",
		zap.String("view", mvName),
		zap.String("source_table", sourceTableName),
		zap.String("global_table", globalTableName),
		zap.Uint64("chain_id", chainID))

	return nil
}

// RemoveChainSync removes all materialized views and deletes chain data from global tables.
// This is called when a chain is deleted from the system.
// This operation is idempotent - safe to call multiple times.
func (s *Store) RemoveChainSync(ctx context.Context, chainID uint64) error {
	s.Logger.Info("Removing cross-chain sync for chain",
		zap.Uint64("chain_id", chainID),
		zap.String("target_db", s.Name))

	chainDBName := fmt.Sprintf("chain_%d", chainID)

	// Check if chain database still exists
	exists, err := s.chainDatabaseExists(ctx, chainDBName)
	if err != nil {
		s.Logger.Warn("Failed to check if chain database exists, continuing with cleanup",
			zap.Uint64("chain_id", chainID),
			zap.Error(err))
	}

	configs := GetTableConfigs()

	// Drop materialized views (only if database exists)
	if exists {
		for _, cfg := range configs {
			mvName := s.getMaterializedViewName(chainID, cfg.TableName)
			query := fmt.Sprintf(`DROP VIEW IF EXISTS "%s"."%s"`, chainDBName, mvName)
			if err := s.Exec(ctx, query); err != nil {
				s.Logger.Warn("Failed to drop materialized view",
					zap.String("view", mvName),
					zap.Uint64("chain_id", chainID),
					zap.Error(err))
				// Continue with other views even if one fails
			} else {
				s.Logger.Debug("Materialized view dropped",
					zap.String("view", mvName),
					zap.Uint64("chain_id", chainID))
			}
		}
	}

	// Delete chain data from global tables (always attempt this)
	for _, cfg := range configs {
		globalTableName := s.getGlobalTableName(cfg.TableName)
		query := fmt.Sprintf(`DELETE FROM "%s"."%s" WHERE chain_id = ?`, s.Name, globalTableName)
		if err := s.Exec(ctx, query, chainID); err != nil {
			s.Logger.Warn("Failed to delete chain data from global table",
				zap.String("table", globalTableName),
				zap.Uint64("chain_id", chainID),
				zap.Error(err))
			// Continue with other tables even if one fails
		} else {
			s.Logger.Debug("Chain data deleted from global table",
				zap.String("table", globalTableName),
				zap.Uint64("chain_id", chainID))
		}
	}

	s.Logger.Info("Cross-chain sync removed for chain",
		zap.Uint64("chain_id", chainID))

	return nil
}

// validateTableName checks if the table name is in our whitelist of supported tables.
// This prevents SQL injection attacks via malicious table names.
func (s *Store) validateTableName(tableName string) error {
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
// Returns error if table is not in the whitelist.
func (s *Store) getTableConfig(tableName string) (*TableConfig, error) {
	if err := s.validateTableName(tableName); err != nil {
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

// ResyncTable backfills existing data for a specific table and chain.
// This is needed because materialized views only sync NEW data after creation.
// Uses append-only pattern for safety (relies on ReplacingMergeTree deduplication).
// This operation is idempotent - safe to retry if it fails.
func (s *Store) ResyncTable(ctx context.Context, chainID uint64, tableName string) error {
	s.Logger.Info("Resyncing table for chain",
		zap.Uint64("chain_id", chainID),
		zap.String("table", tableName))

	// Validate table name against whitelist (SECURITY: prevents SQL injection)
	cfg, err := s.getTableConfig(tableName)
	if err != nil {
		return err
	}

	chainDBName := fmt.Sprintf("chain_%d", chainID)
	globalTableName := s.getGlobalTableName(tableName)

	// Verify source table exists
	sourceExists, err := s.TableExists(ctx, chainDBName, tableName)
	if err != nil {
		return fmt.Errorf("failed to check if source table exists: %w", err)
	}
	if !sourceExists {
		return fmt.Errorf("source table %s.%s does not exist", chainDBName, tableName)
	}

	// Build column list for INSERT
	columnList := strings.Join(cfg.ColumnNames, ", ")

	// First, delete existing data for this chain+table combination
	// This ensures we don't accumulate duplicates if resync is run multiple times
	deleteQuery := fmt.Sprintf(`DELETE FROM "%s"."%s" WHERE chain_id = ?`, s.Name, globalTableName)
	if err := s.Exec(ctx, deleteQuery, chainID); err != nil {
		return fmt.Errorf("failed to delete existing data: %w", err)
	}

	// Then, insert all data from source table using explicit column names
	insertQuery := fmt.Sprintf(`
		INSERT INTO "%s"."%s" (chain_id, %s, updated_at)
		SELECT
			? AS chain_id,
			%s,
			now64(6) AS updated_at
		FROM "%s"."%s"
	`, s.Name, globalTableName, columnList, columnList, chainDBName, tableName)

	if err := s.Exec(ctx, insertQuery, chainID); err != nil {
		return fmt.Errorf("failed to insert data: %w", err)
	}

	s.Logger.Info("Table resync complete",
		zap.Uint64("chain_id", chainID),
		zap.String("table", tableName))

	return nil
}

// ResyncChain backfills existing data for all tables of a chain.
// This should be called after SetupChainSync to populate historical data.
// This operation is idempotent - safe to retry if it fails.
func (s *Store) ResyncChain(ctx context.Context, chainID uint64) error {
	s.Logger.Info("Resyncing all tables for chain",
		zap.Uint64("chain_id", chainID))

	configs := GetTableConfigs()
	successCount := 0
	failCount := 0

	for _, cfg := range configs {
		if err := s.ResyncTable(ctx, chainID, cfg.TableName); err != nil {
			s.Logger.Error("Failed to resync table",
				zap.Uint64("chain_id", chainID),
				zap.String("table", cfg.TableName),
				zap.Error(err))
			failCount++
			// Continue with other tables even if one fails
		} else {
			successCount++
		}
	}

	s.Logger.Info("Chain resync complete",
		zap.Uint64("chain_id", chainID),
		zap.Int("success", successCount),
		zap.Int("failed", failCount))

	if failCount > 0 {
		return fmt.Errorf("resync completed with %d failures", failCount)
	}

	return nil
}

// OptimizeTables runs OPTIMIZE TABLE FINAL on all global tables.
// This forces final merge of ReplacingMergeTree tables, removing old versions.
// Should be run periodically (e.g., weekly) to reduce storage and improve query performance.
// This operation can take significant time on large tables.
func (s *Store) OptimizeTables(ctx context.Context) error {
	s.Logger.Info("Optimizing cross-chain global tables", zap.String("database", s.Name))

	configs := GetTableConfigs()
	for _, cfg := range configs {
		globalTableName := s.getGlobalTableName(cfg.TableName)
		if err := s.OptimizeTable(ctx, s.Name, globalTableName, true); err != nil {
			s.Logger.Error("Failed to optimize table",
				zap.String("table", globalTableName),
				zap.Error(err))
			// Continue with other tables even if one fails
		} else {
			s.Logger.Info("Table optimized",
				zap.String("table", globalTableName))
		}
	}

	s.Logger.Info("Optimization complete for all global tables")
	return nil
}

// SyncStatus represents the sync status for a specific chain and table.
type SyncStatus struct {
	ChainID                uint64    `json:"chain_id"`
	TableName              string    `json:"table_name"`
	SourceRowCount         uint64    `json:"source_row_count"`
	GlobalRowCount         uint64    `json:"global_row_count"`
	Lag                    int64     `json:"lag"` // Difference in row count (can be negative if global has more)
	LastSourceUpdate       time.Time `json:"last_source_update"`
	LastGlobalUpdate       time.Time `json:"last_global_update"`
	MaterializedView       string    `json:"materialized_view"`
	MaterializedViewExists bool      `json:"materialized_view_exists"`
}

// GetSyncStatus returns the sync status for a specific chain and table.
// Returns lag metrics to help monitor data freshness and identify sync issues.
func (s *Store) GetSyncStatus(ctx context.Context, chainID uint64, tableName string) (*SyncStatus, error) {
	// Validate table name against whitelist (SECURITY: prevents SQL injection)
	if err := s.validateTableName(tableName); err != nil {
		return nil, err
	}

	chainDBName := fmt.Sprintf("chain_%d", chainID)
	globalTableName := s.getGlobalTableName(tableName)
	mvName := s.getMaterializedViewName(chainID, tableName)

	status := &SyncStatus{
		ChainID:          chainID,
		TableName:        tableName,
		MaterializedView: mvName,
	}

	// Check if materialized view exists
	mvExists, err := s.materializedViewExists(ctx, chainDBName, mvName)
	if err != nil {
		s.Logger.Warn("Failed to check materialized view existence",
			zap.String("view", mvName),
			zap.Error(err))
	}
	status.MaterializedViewExists = mvExists

	// Get source table row count and last update
	sourceQuery := fmt.Sprintf(`
		SELECT
			count() AS row_count,
			max(height_time) AS last_update
		FROM "%s"."%s"
	`, chainDBName, tableName)

	var sourceRowCount uint64
	var lastSourceUpdate time.Time
	if err := s.QueryRow(ctx, sourceQuery).Scan(&sourceRowCount, &lastSourceUpdate); err != nil {
		if !clickhouse.IsNoRows(err) {
			return nil, fmt.Errorf("failed to query source table: %w", err)
		}
		// Table is empty or doesn't exist
		sourceRowCount = 0
	}
	status.SourceRowCount = sourceRowCount
	status.LastSourceUpdate = lastSourceUpdate

	// Get global table row count and last update for this chain
	globalQuery := fmt.Sprintf(`
		SELECT
			count() AS row_count,
			max(updated_at) AS last_update
		FROM "%s"."%s"
		WHERE chain_id = ?
	`, s.Name, globalTableName)

	var globalRowCount uint64
	var lastGlobalUpdate time.Time
	if err := s.QueryRow(ctx, globalQuery, chainID).Scan(&globalRowCount, &lastGlobalUpdate); err != nil {
		if !clickhouse.IsNoRows(err) {
			return nil, fmt.Errorf("failed to query global table: %w", err)
		}
		// No data for this chain yet
		globalRowCount = 0
	}
	status.GlobalRowCount = globalRowCount
	status.LastGlobalUpdate = lastGlobalUpdate

	// Calculate lag
	status.Lag = int64(sourceRowCount) - int64(globalRowCount)

	return status, nil
}

// HealthStatus represents the overall health of the cross-chain sync system.
type HealthStatus struct {
	DatabaseName      string         `json:"database_name"`
	TotalChainsSynced int            `json:"total_chains_synced"`
	MaxLag            int64          `json:"max_lag"`
	TablesHealthy     int            `json:"tables_healthy"`
	TablesUnhealthy   int            `json:"tables_unhealthy"`
	Details           []HealthDetail `json:"details,omitempty"`
	CheckedAt         time.Time      `json:"checked_at"`
}

// HealthDetail provides per-chain health information.
type HealthDetail struct {
	ChainID       uint64 `json:"chain_id"`
	TablesHealthy int    `json:"tables_healthy"`
	TablesWithLag int    `json:"tables_with_lag"`
	MaxLag        int64  `json:"max_lag"`
}

// GetHealthStatus returns overall health status of the cross-chain sync system.
// Checks all chains that have data in global tables.
func (s *Store) GetHealthStatus(ctx context.Context) (*HealthStatus, error) {
	health := &HealthStatus{
		DatabaseName: s.Name,
		CheckedAt:    time.Now(),
	}

	// Get list of chains with data in global tables
	chainsQuery := fmt.Sprintf(`
		SELECT DISTINCT chain_id
		FROM "%s"."%s"
		ORDER BY chain_id
	`, s.Name, s.getGlobalTableName(indexer.AccountsProductionTableName))

	rows, err := s.Query(ctx, chainsQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query chains: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var chainIDs []uint64
	for rows.Next() {
		var chainID uint64
		if err := rows.Scan(&chainID); err != nil {
			s.Logger.Warn("Failed to scan chain ID", zap.Error(err))
			continue
		}
		chainIDs = append(chainIDs, chainID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	health.TotalChainsSynced = len(chainIDs)

	// Check health for each chain
	configs := GetTableConfigs()
	for _, chainID := range chainIDs {
		detail := HealthDetail{ChainID: chainID}

		for _, cfg := range configs {
			status, err := s.GetSyncStatus(ctx, chainID, cfg.TableName)
			if err != nil {
				s.Logger.Warn("Failed to get sync status",
					zap.Uint64("chain_id", chainID),
					zap.String("table", cfg.TableName),
					zap.Error(err))
				continue
			}

			// Check if table is healthy (lag <= 10 rows or 0.1% of total)
			isHealthy := status.Lag >= 0 && (status.Lag <= 10 || status.Lag <= int64(status.SourceRowCount/1000))
			if isHealthy {
				detail.TablesHealthy++
				health.TablesHealthy++
			} else {
				detail.TablesWithLag++
				health.TablesUnhealthy++
			}

			// Track max lag
			if status.Lag > detail.MaxLag {
				detail.MaxLag = status.Lag
			}
			if status.Lag > health.MaxLag {
				health.MaxLag = status.Lag
			}
		}

		health.Details = append(health.Details, detail)
	}

	return health, nil
}

// Helper functions

func (s *Store) getGlobalTableName(baseTableName string) string {
	return baseTableName + "_global"
}

func (s *Store) getMaterializedViewName(chainID uint64, baseTableName string) string {
	return fmt.Sprintf("%s_sync_chain_%d", baseTableName, chainID)
}

// getColumnsForTable returns the column definitions for a given table name.
// This mapping connects table names to their ColumnDef definitions in pkg/db/models/indexer.
func getColumnsForTable(tableName string) []indexer.ColumnDef {
	switch tableName {
	case indexer.AccountsProductionTableName:
		return indexer.AccountColumns
	case indexer.ValidatorsProductionTableName:
		return indexer.ValidatorColumns
	case indexer.ValidatorSigningInfoProductionTableName:
		return indexer.ValidatorSigningInfoColumns
	case indexer.ValidatorDoubleSigningInfoProductionTableName:
		return indexer.ValidatorDoubleSigningInfoColumns
	case indexer.PoolsProductionTableName:
		return indexer.PoolColumns
	case indexer.PoolPointsByHolderProductionTableName:
		return indexer.PoolPointsByHolderColumns
	case indexer.OrdersProductionTableName:
		return indexer.OrderColumns
	case indexer.DexOrdersProductionTableName:
		return indexer.DexOrderColumns
	case indexer.DexDepositsProductionTableName:
		return indexer.DexDepositColumns
	case indexer.DexWithdrawalsProductionTableName:
		return indexer.DexWithdrawalColumns
	case indexer.BlockSummariesProductionTableName:
		return indexer.BlockSummaryColumns
	case indexer.CommitteePaymentsProductionTableName:
		return indexer.CommitteePaymentColumns
	default:
		// Return empty slice for unknown tables (should never happen if validateTableName is used)
		return []indexer.ColumnDef{}
	}
}

func (s *Store) chainDatabaseExists(ctx context.Context, dbName string) (bool, error) {
	query := `SELECT count() FROM system.databases WHERE name = ?`
	var count uint64
	err := s.QueryRow(ctx, query, dbName).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check database existence: %w", err)
	}
	return count > 0, nil
}

func (s *Store) materializedViewExists(ctx context.Context, dbName, viewName string) (bool, error) {
	query := `SELECT count() FROM system.tables WHERE database = ? AND name = ? AND engine LIKE '%MaterializedView%'`
	var count uint64
	err := s.QueryRow(ctx, query, dbName, viewName).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check view existence: %w", err)
	}
	return count > 0, nil
}

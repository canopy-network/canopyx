package crosschain

import (
	"fmt"
	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"strings"
	"time"
)

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

// SetupChainSync creates 14 materialized views to sync a chain's data to global tables.
// Materialized views automatically copy new data as it arrives in the source chain database.
// This operation is idempotent - safe to call multiple times.
//
// IMPORTANT: This only syncs NEW data. For existing data, call ResyncChain after setup.
func (db *DB) SetupChainSync(ctx context.Context, chainID uint64) error {
	db.Logger.Info("Setting up cross-chain sync for chain",
		zap.Uint64("chain_id", chainID),
		zap.String("target_db", db.Name))

	chainDBName := fmt.Sprintf("chain_%d", chainID)

	configs := GetTableConfigs()
	for _, cfg := range configs {
		if err := db.createSyncMaterializedView(ctx, chainID, chainDBName, cfg); err != nil {
			db.Logger.Error("Failed to create materialized view",
				zap.Uint64("chain_id", chainID),
				zap.String("table", cfg.TableName),
				zap.Error(err))
			return fmt.Errorf("failed to create sync view for %s: %w", cfg.TableName, err)
		}
	}

	db.Logger.Info("Cross-chain sync setup complete for chain",
		zap.Uint64("chain_id", chainID),
		zap.Int("tables_synced", len(configs)))

	return nil
}

// GetSyncStatus returns the sync status for a specific chain and table.
// Returns lag metrics to help monitor data freshness and identify sync issues.
func (db *DB) GetSyncStatus(ctx context.Context, chainID uint64, tableName string) (*SyncStatus, error) {
	// Validate table name against whitelist (SECURITY: prevents SQL injection)
	if err := db.validateTableName(tableName); err != nil {
		return nil, err
	}

	chainDBName := fmt.Sprintf("chain_%d", chainID)
	globalTableName := db.getGlobalTableName(tableName)
	mvName := db.getMaterializedViewName(chainID, tableName)

	status := &SyncStatus{
		ChainID:          chainID,
		TableName:        tableName,
		MaterializedView: mvName,
	}

	// Check if a materialized view exists
	mvExists, err := db.materializedViewExists(ctx, chainDBName, mvName)
	if err != nil {
		db.Logger.Warn("Failed to check materialized view existence",
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
	if err := db.QueryRow(ctx, sourceQuery).Scan(&sourceRowCount, &lastSourceUpdate); err != nil {
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
	`, db.Name, globalTableName)

	var globalRowCount uint64
	var lastGlobalUpdate time.Time
	if err := db.QueryRow(ctx, globalQuery, chainID).Scan(&globalRowCount, &lastGlobalUpdate); err != nil {
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

// ResyncTable backfills existing data for a specific table and chain.
// This is needed because materialized views only sync NEW data after creation.
// Uses an append-only pattern for safety (relies on ReplacingMergeTree deduplication).
// This operation is idempotent - safe to retry if it fails.
func (db *DB) ResyncTable(ctx context.Context, chainID uint64, tableName string) error {
	db.Logger.Info("Resyncing table for chain",
		zap.Uint64("chain_id", chainID),
		zap.String("table", tableName))

	// Validate table name against whitelist (SECURITY: prevents SQL injection)
	cfg, err := db.getTableConfig(tableName)
	if err != nil {
		return err
	}

	chainDBName := fmt.Sprintf("chain_%d", chainID)
	globalTableName := db.getGlobalTableName(tableName)

	// Verify source table exists
	sourceExists, err := db.TableExists(ctx, chainDBName, tableName)
	if err != nil {
		return fmt.Errorf("failed to check if source table exists: %w", err)
	}
	if !sourceExists {
		return fmt.Errorf("source table %s.%s does not exist", chainDBName, tableName)
	}

	// Build column list for INSERT
	columnList := strings.Join(cfg.ColumnNames, ", ")

	// First, delete existing data for this chain+table combination
	// This ensures we don't accumulate duplicates if the resyncing is run multiple times
	deleteQuery := fmt.Sprintf(`DELETE FROM "%s"."%s" %s WHERE chain_id = ?`, db.Name, globalTableName, db.OnCluster())
	if err := db.Exec(ctx, deleteQuery, chainID); err != nil {
		return fmt.Errorf("failed to delete existing data: %w", err)
	}

	// Then, insert all data from the source table using explicit column names
	insertQuery := fmt.Sprintf(`
		INSERT INTO "%s"."%s" (chain_id, %s, updated_at)
		SELECT
			? AS chain_id,
			%s,
			now64(6) AS updated_at
		FROM "%s"."%s"
	`, db.Name, globalTableName, columnList, columnList, chainDBName, tableName)

	if err := db.Exec(ctx, insertQuery, chainID); err != nil {
		return fmt.Errorf("failed to insert data: %w", err)
	}

	db.Logger.Info("Table resync complete",
		zap.Uint64("chain_id", chainID),
		zap.String("table", tableName))

	return nil
}

// ResyncChain backfills existing data for all tables of a chain.
// This should be called after SetupChainSync to populate historical data.
// This operation is idempotent - safe to retry if it fails.
func (db *DB) ResyncChain(ctx context.Context, chainID uint64) error {
	db.Logger.Info("Resyncing all tables for chain",
		zap.Uint64("chain_id", chainID))

	configs := GetTableConfigs()
	successCount := 0
	failCount := 0

	for _, cfg := range configs {
		// TODO: we should parallelize this otherwise will take forever
		if err := db.ResyncTable(ctx, chainID, cfg.TableName); err != nil {
			db.Logger.Error("Failed to resync table",
				zap.Uint64("chain_id", chainID),
				zap.String("table", cfg.TableName),
				zap.Error(err))
			failCount++
			// Continue with other tables even if one fails
		} else {
			successCount++
		}
	}

	db.Logger.Info("Chain resync complete",
		zap.Uint64("chain_id", chainID),
		zap.Int("success", successCount),
		zap.Int("failed", failCount))

	if failCount > 0 {
		return fmt.Errorf("resync completed with %d failures", failCount)
	}

	return nil
}

// RemoveChainSync removes all materialized views and deletes chain data from global tables.
// This is called when a chain is deleted from the system.
// This operation is idempotent - safe to call multiple times.
func (db *DB) RemoveChainSync(ctx context.Context, chainID uint64) error {
	db.Logger.Info("Removing cross-chain sync for chain",
		zap.Uint64("chain_id", chainID),
		zap.String("target_db", db.Name))

	chainDBName := fmt.Sprintf("chain_%d", chainID)

	configs := GetTableConfigs()

	for _, cfg := range configs {
		mvName := db.getMaterializedViewName(chainID, cfg.TableName)
		query := fmt.Sprintf(`DROP VIEW IF EXISTS "%s"."%s" %s`, chainDBName, mvName, db.OnCluster())
		if err := db.Exec(ctx, query); err != nil {
			db.Logger.Warn("Failed to drop materialized view",
				zap.String("view", mvName),
				zap.Uint64("chain_id", chainID),
				zap.Error(err))
			// Continue with other views even if one fails
		} else {
			db.Logger.Debug("Materialized view dropped",
				zap.String("view", mvName),
				zap.Uint64("chain_id", chainID))
		}
	}

	// Delete chain data from global tables (always attempt this)
	for _, cfg := range configs {
		globalTableName := db.getGlobalTableName(cfg.TableName)
		query := fmt.Sprintf(
			`DELETE FROM "%s"."%s" %s WHERE chain_id = ?`,
			db.Name,
			globalTableName,
			db.OnCluster(),
		)
		if err := db.Exec(ctx, query, chainID); err != nil {
			db.Logger.Warn("Failed to delete chain data from global table",
				zap.String("table", globalTableName),
				zap.Uint64("chain_id", chainID),
				zap.Error(err))
			// Continue with other tables even if one fails
		} else {
			db.Logger.Debug("Chain data deleted from global table",
				zap.String("table", globalTableName),
				zap.Uint64("chain_id", chainID))
		}
	}

	db.Logger.Info("Cross-chain sync removed for chain",
		zap.Uint64("chain_id", chainID))

	return nil
}

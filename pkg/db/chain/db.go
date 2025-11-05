package chain

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	"github.com/canopy-network/canopyx/pkg/db/entities"
	"go.uber.org/zap"
)

// DB represents a database associated with a blockchain and provides methods to manage and query its data.
// It includes a database client, a logger for capturing logs, the chain's name, and its unique identifier.
// It implements Store.
type DB struct {
	clickhouse.Client
	Name    string
	ChainID uint64
}

// New creates and initializes a chain-specific ClickHouse database instance.
func New(ctx context.Context, logger *zap.Logger, chainID uint64) (*DB, error) {
	chainIDStr := fmt.Sprintf("chain_%d", chainID)
	dbName := clickhouse.SanitizeName(chainIDStr)

	client, err := clickhouse.New(ctx, logger.With(
		zap.String("db", dbName),
		zap.String("component", "chain_db"),
		zap.Uint64("chainID", chainID),
	), dbName)
	if err != nil {
		return nil, err
	}

	chainDB := &DB{
		Client:  client,
		Name:    dbName,
		ChainID: chainID,
	}

	if err := chainDB.InitializeDB(ctx); err != nil {
		return nil, err
	}

	return chainDB, nil
}

// Close terminates the underlying ClickHouse connection.
func (db *DB) Close() error {
	return db.Db.Close()
}

// InitializeDB ensures the required database and tables for indexing are created if they do not already exist.
func (db *DB) InitializeDB(ctx context.Context) error {
	db.Logger.Debug("Initialize blocks model", zap.String("name", db.Name))
	if err := db.initBlocks(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize block_summaries model", zap.String("name", db.Name))
	if err := db.initBlockSummaries(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize transactions model", zap.String("name", db.Name))
	if err := db.initTransactions(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize accounts model", zap.String("name", db.Name))
	if err := db.initAccounts(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize account_created_height view", zap.String("name", db.Name))
	if err := db.initAccountCreatedHeightView(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize events model", zap.String("name", db.Name))
	if err := db.initEvents(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize pools model", zap.String("name", db.Name))
	if err := db.initPools(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize orders model", zap.String("name", db.Name))
	if err := db.initOrders(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize order_created_height view", zap.String("name", db.Name))
	if err := db.initOrderCreatedHeightView(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize dex_prices model", zap.String("name", db.Name))
	if err := db.initDexPrices(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize dex_orders model", zap.String("name", db.Name))
	if err := db.initDexOrders(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize dex_order_created_height view", zap.String("name", db.Name))
	if err := db.initDexOrderCreatedHeightView(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize dex_deposits model", zap.String("name", db.Name))
	if err := db.initDexDeposits(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize dex_deposit_created_height view", zap.String("name", db.Name))
	if err := db.initDexDepositCreatedHeightView(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize dex_withdrawals model", zap.String("name", db.Name))
	if err := db.initDexWithdrawals(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize dex_withdrawal_created_height view", zap.String("name", db.Name))
	if err := db.initDexWithdrawalCreatedHeightView(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize genesis table", zap.String("name", db.Name))
	if err := db.initGenesis(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize pool_points_by_holder model", zap.String("name", db.Name))
	if err := db.initPoolPointsByHolder(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize pool_points_created_height view", zap.String("name", db.Name))
	if err := db.initPoolPointsCreatedHeightView(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize validators model", zap.String("name", db.Name))
	if err := db.initValidators(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize validator_created_height view", zap.String("name", db.Name))
	if err := db.initValidatorCreatedHeightView(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize validator_signing_info model", zap.String("name", db.Name))
	if err := db.initValidatorSigningInfo(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize validator_double_signing_info model", zap.String("name", db.Name))
	if err := db.initValidatorDoubleSigningInfo(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize params model", zap.String("name", db.Name))
	if err := db.initParams(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize params_change_height materialized view", zap.String("name", db.Name))
	if err := db.initParamsChangeHeightView(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize committees model", zap.String("name", db.Name))
	if err := db.initCommittees(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize committee_created_height materialized view", zap.String("name", db.Name))
	if err := db.initCommitteeCreatedHeightView(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize committee_validators junction table", zap.String("name", db.Name))
	if err := db.initCommitteeValidators(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize poll_snapshots model", zap.String("name", db.Name))
	if err := db.initPollSnapshots(ctx); err != nil {
		return err
	}

	return nil
}

// DatabaseName returns the ClickHouse database backing this chain.
func (db *DB) DatabaseName() string {
	return db.Name
}

// ChainKey returns the identifier associated with this chain store.
func (db *DB) ChainKey() string {
	return fmt.Sprintf("%d", db.ChainID)
}

// PromoteEntity promotes entity data from staging to production table.
// Uses entity constants for type-safe table names.
// The operation is idempotent - safe to retry if it fails.
func (db *DB) PromoteEntity(ctx context.Context, entity entities.Entity, height uint64) error {
	// Validate entity is known
	if !entity.IsValid() {
		return fmt.Errorf("invalid entity: %q", entity)
	}

	// Build promotion query: INSERT INTO production SELECT * FROM staging WHERE height = ?
	query := fmt.Sprintf(
		`INSERT INTO "%s"."%s" SELECT * FROM "%s"."%s" WHERE height = ?`,
		db.Name, entity.TableName(),
		db.Name, entity.StagingTableName(),
	)

	if err := db.Db.Exec(ctx, query, height); err != nil {
		return fmt.Errorf("promote %s at height %d: %w", entity, height, err)
	}

	db.Logger.Debug("Promoted entity data",
		zap.String("entity", entity.String()),
		zap.Uint64("height", height),
		zap.String("database", db.Name))

	return nil
}

// CleanEntityStaging removes promoted data from staging table.
// Uses DELETE FROM for synchronous deletion (OLTP-friendly).
// The operation is idempotent - safe to retry if it fails.
func (db *DB) CleanEntityStaging(ctx context.Context, entity entities.Entity, height uint64) error {
	// Validate entity is known
	if !entity.IsValid() {
		return fmt.Errorf("invalid entity: %q", entity)
	}

	// Build cleanup query using lightweight DELETE FROM (not ALTER TABLE DELETE which is async)
	query := fmt.Sprintf(
		`DELETE FROM "%s"."%s" WHERE height = ?`,
		db.Name, entity.StagingTableName(),
	)

	if err := db.Db.Exec(ctx, query, height); err != nil {
		// Log warning but don't fail - cleanup is non-critical
		db.Logger.Warn("Failed to clean staging data",
			zap.String("entity", entity.String()),
			zap.Uint64("height", height),
			zap.String("database", db.Name),
			zap.Error(err))
		return fmt.Errorf("clean %s staging at height %d: %w", entity, height, err)
	}

	db.Logger.Debug("Cleaned staging data",
		zap.String("entity", entity.String()),
		zap.Uint64("height", height),
		zap.String("database", db.Name))

	return nil
}

package global

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	"github.com/canopy-network/canopyx/pkg/db/entities"
	"go.uber.org/zap"
)

// DefaultGlobalDBName is the default database name for the global (single-db) architecture.
const DefaultGlobalDBName = "canopyx_global"

// DB represents a global database instance that stores data for all chains.
// Unlike chain.DB which uses separate databases per chain (chain_0, chain_1, etc.),
// GlobalDB uses a single database with chain_id as a column in each table.
// This eliminates the need for materialized views to sync data across chains.
//
// It implements Store.
type DB struct {
	clickhouse.Client
	Name    string // Database name (e.g., "canopyx_global")
	ChainID uint64 // Chain ID context for writes - set when creating DB for indexer
}

// NewWithPoolConfig creates and initializes a global ClickHouse database instance with custom pool configuration.
// The chainID parameter sets the context for all write operations (inserts, promotes, cleanup).
// For admin app initialization (where no specific chain context is needed), use chainID=0.
func NewWithPoolConfig(ctx context.Context, logger *zap.Logger, dbName string, chainID uint64, poolConfig clickhouse.PoolConfig) (*DB, error) {
	sanitizedName := clickhouse.SanitizeName(dbName)

	client, err := clickhouse.New(ctx, logger.With(
		zap.String("db", sanitizedName),
		zap.String("component", poolConfig.Component),
		zap.Uint64("chainID", chainID),
	), sanitizedName, &poolConfig)
	if err != nil {
		return nil, err
	}

	globalDB := &DB{
		Client:  client,
		Name:    sanitizedName,
		ChainID: chainID,
	}

	if err := globalDB.InitializeDB(ctx); err != nil {
		return nil, err
	}

	return globalDB, nil
}

// NewWithSharedClient creates a global DB that reuses an existing ClickHouse connection pool.
// This is efficient for scenarios where the global DB is accessed alongside other DBs.
// The chainID parameter sets the context for all write operations.
// The database and tables must already exist - this constructor does NOT call InitializeDB.
func NewWithSharedClient(client clickhouse.Client, dbName string, chainID uint64) *DB {
	sanitizedName := clickhouse.SanitizeName(dbName)

	return &DB{
		Client:  client,
		Name:    sanitizedName,
		ChainID: chainID,
	}
}

// WithChainID returns a new DB instance with the specified chain ID context.
// This is useful when you have a global DB and want to perform operations for a specific chain.
// The returned DB shares the same connection pool but has a different chain context.
func (db *DB) WithChainID(chainID uint64) *DB {
	return &DB{
		Client:  db.Client,
		Name:    db.Name,
		ChainID: chainID,
	}
}

// Close terminates the underlying ClickHouse connection.
func (db *DB) Close() error {
	return db.Db.Close()
}

// GetConnection returns the underlying ClickHouse driver connection.
func (db *DB) GetConnection() driver.Conn {
	return db.Db
}

// GetClient returns the underlying ClickHouse client.
func (db *DB) GetClient() clickhouse.Client {
	return db.Client
}

// InitializeDB ensures the required database and tables for indexing are created if they do not already exist.
// Creates all tables with chain_id as the first column for multi-tenant data isolation.
func (db *DB) InitializeDB(ctx context.Context) error {
	initStart := time.Now()

	if err := db.CreateDbIfNotExists(ctx, db.Name); err != nil {
		return fmt.Errorf("failed to create database %s: %w", db.Name, err)
	}

	// Create all base tables in parallel using goroutines
	db.Logger.Info("Creating all global tables in parallel",
		zap.String("database", db.Name))

	// Define all table init operations
	initOps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"blocks", db.initBlocks},
		{"block_summaries", db.initBlockSummaries},
		{"transactions", db.initTransactions},
		{"accounts", db.initAccounts},
		{"events", db.initEvents},
		{"pools", db.initPools},
		{"orders", db.initOrders},
		{"dex_prices", db.initDexPrices},
		{"dex_orders", db.initDexOrders},
		{"dex_deposits", db.initDexDeposits},
		{"dex_withdrawals", db.initDexWithdrawals},
		{"pool_points_by_holder", db.initPoolPointsByHolder},
		{"validators", db.initValidators},
		{"validator_non_signing_info", db.initValidatorNonSigningInfo},
		{"validator_double_signing_info", db.initValidatorDoubleSigningInfo},
		{"params", db.initParams},
		{"committees", db.initCommittees},
		{"committee_validators", db.initCommitteeValidators},
		{"committee_payments", db.initCommitteePayments},
		{"poll_snapshots", db.initPollSnapshots},
		{"proposal_snapshots", db.initProposalSnapshots},
		{"supply", db.initSupply},
		{"lp_position_snapshots", db.initLPPositionSnapshots},
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(initOps))

	// Launch all init operations in parallel
	for _, op := range initOps {
		wg.Add(1)
		go func(name string, fn func(context.Context) error) {
			defer wg.Done()
			if err := fn(ctx); err != nil {
				errChan <- fmt.Errorf("init %s: %w", name, err)
			}
		}(op.name, op.fn)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		return err
	}

	// Create all materialized views in parallel using goroutines
	db.Logger.Info("Creating all global materialized views in parallel",
		zap.String("database", db.Name))

	// Define all materialized view init operations
	viewOps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"account_created_height", db.initAccountCreatedHeightView},
		{"order_created_height", db.initOrderCreatedHeightView},
		{"dex_order_created_height", db.initDexOrderCreatedHeightView},
		{"dex_deposit_created_height", db.initDexDepositCreatedHeightView},
		{"dex_withdrawal_created_height", db.initDexWithdrawalCreatedHeightView},
		{"pool_points_created_height", db.initPoolPointsCreatedHeightView},
		{"pool_created_height", db.initPoolCreatedHeightView},
		{"validator_created_height", db.initValidatorCreatedHeightView},
		{"params_change_height", db.initParamsChangeHeightView},
		{"committee_created_height", db.initCommitteeCreatedHeightView},
	}

	var wg2 sync.WaitGroup
	errChan2 := make(chan error, len(viewOps))

	// Launch all view init operations in parallel
	for _, op := range viewOps {
		wg2.Add(1)
		go func(name string, fn func(context.Context) error) {
			defer wg2.Done()
			if err := fn(ctx); err != nil {
				errChan2 <- fmt.Errorf("init %s view: %w", name, err)
			}
		}(op.name, op.fn)
	}

	// Wait for all goroutines to complete
	wg2.Wait()
	close(errChan2)

	// Check for any errors
	for err := range errChan2 {
		return err
	}

	db.Logger.Info("Global database initialization complete",
		zap.String("database", db.Name),
		zap.Duration("total_duration", time.Since(initStart)))

	return nil
}

// DatabaseName returns the ClickHouse database backing this global store.
func (db *DB) DatabaseName() string {
	return db.Name
}

// ChainKey returns the identifier associated with this chain context.
func (db *DB) ChainKey() string {
	return fmt.Sprintf("%d", db.ChainID)
}

// GetChainID returns the chain ID context for this DB instance.
func (db *DB) GetChainID() uint64 {
	return db.ChainID
}

// DeleteChainData removes ALL data for the specified chain_id from ALL tables.
// This is used for hard-deleting a chain from the global database.
// Unlike other methods, this accepts an explicit chainID parameter so it can be called from admin context.
//
// DANGER: This operation is irreversible and will permanently delete all indexed data for the chain.
// The operation runs in parallel across all tables for efficiency.
func (db *DB) DeleteChainData(ctx context.Context, chainID uint64) error {
	db.Logger.Warn("Starting hard delete of all chain data",
		zap.String("database", db.Name),
		zap.Uint64("chain_id", chainID))

	startTime := time.Now()

	// Get all entities
	allEntities := entities.All()

	db.Logger.Info("Deleting chain data from all tables",
		zap.String("database", db.Name),
		zap.Uint64("chain_id", chainID),
		zap.Int("table_count", len(allEntities)))

	var wg sync.WaitGroup
	errChan := make(chan error, len(allEntities))

	// Launch DELETE for each entity in parallel
	for _, entity := range allEntities {
		wg.Add(1)
		go func(e entities.Entity) {
			defer wg.Done()

			// Use entity's chain ID column (most use "chain_id", but some like LP snapshots use "source_chain_id")
			query := fmt.Sprintf(
				`DELETE FROM "%s"."%s" WHERE %s = ?`,
				db.Name, e.TableName(), e.ChainIDColumn(),
			)

			if err := db.Db.Exec(ctx, query, chainID); err != nil {
				errChan <- fmt.Errorf("delete chain %d data from %s: %w", chainID, e.TableName(), err)
			}
		}(entity)
	}

	// Wait for all DELETEs to complete
	wg.Wait()
	close(errChan)

	// Collect all errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		db.Logger.Error("Failed to delete chain data from some tables",
			zap.String("database", db.Name),
			zap.Uint64("chain_id", chainID),
			zap.Int("error_count", len(errors)),
			zap.Error(errors[0]))
		return fmt.Errorf("failed to delete chain %d data: %d errors, first: %w", chainID, len(errors), errors[0])
	}

	db.Logger.Warn("Completed hard delete of all chain data",
		zap.String("database", db.Name),
		zap.Uint64("chain_id", chainID),
		zap.Int("tables_deleted", len(allEntities)),
		zap.Duration("duration", time.Since(startTime)))

	return nil
}

// DeleteEntityData removes ALL data for the specified chain_id from a specific entity's table.
// Useful for selective cleanup of specific entity data.
func (db *DB) DeleteEntityData(ctx context.Context, chainID uint64, entity entities.Entity) error {
	if !entity.IsValid() {
		return fmt.Errorf("invalid entity: %q", entity)
	}

	db.Logger.Info("Deleting entity data for chain",
		zap.String("database", db.Name),
		zap.Uint64("chain_id", chainID),
		zap.String("entity", entity.String()))

	// Use entity's chain ID column (most use "chain_id", but some like LP snapshots use "source_chain_id")
	query := fmt.Sprintf(
		`DELETE FROM "%s"."%s" WHERE %s = ?`,
		db.Name, entity.TableName(), entity.ChainIDColumn(),
	)

	if err := db.Db.Exec(ctx, query, chainID); err != nil {
		return fmt.Errorf("delete chain %d data from %s: %w", chainID, entity.TableName(), err)
	}

	db.Logger.Info("Deleted entity data for chain",
		zap.String("database", db.Name),
		zap.Uint64("chain_id", chainID),
		zap.String("entity", entity.String()))

	return nil
}

// DeleteHeightRange removes data for a specific chain within a height range from ALL tables.
// This is useful for re-indexing a range of blocks or cleaning up bad data.
func (db *DB) DeleteHeightRange(ctx context.Context, chainID uint64, fromHeight, toHeight uint64) error {
	if fromHeight > toHeight {
		return fmt.Errorf("invalid height range: fromHeight (%d) > toHeight (%d)", fromHeight, toHeight)
	}

	db.Logger.Warn("Starting delete of chain data in height range",
		zap.String("database", db.Name),
		zap.Uint64("chain_id", chainID),
		zap.Uint64("from_height", fromHeight),
		zap.Uint64("to_height", toHeight))

	startTime := time.Now()

	// Get all entities
	allEntities := entities.All()

	// Get table names for all entities
	var tablesToDelete []string
	for _, entity := range allEntities {
		tablesToDelete = append(tablesToDelete, entity.TableName())
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(tablesToDelete))

	// Launch DELETE for each table in parallel
	for _, tableName := range tablesToDelete {
		wg.Add(1)
		go func(table string) {
			defer wg.Done()

			query := fmt.Sprintf(
				`DELETE FROM "%s"."%s" WHERE chain_id = ? AND height >= ? AND height <= ?`,
				db.Name, table,
			)

			if err := db.Db.Exec(ctx, query, chainID, fromHeight, toHeight); err != nil {
				errChan <- fmt.Errorf("delete chain %d heights %d-%d from %s: %w", chainID, fromHeight, toHeight, table, err)
			}
		}(tableName)
	}

	// Wait for all DELETEs to complete
	wg.Wait()
	close(errChan)

	// Collect all errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		db.Logger.Error("Failed to delete height range from some tables",
			zap.String("database", db.Name),
			zap.Uint64("chain_id", chainID),
			zap.Uint64("from_height", fromHeight),
			zap.Uint64("to_height", toHeight),
			zap.Int("error_count", len(errors)),
			zap.Error(errors[0]))
		return fmt.Errorf("failed to delete height range: %d errors, first: %w", len(errors), errors[0])
	}

	db.Logger.Warn("Completed delete of chain data in height range",
		zap.String("database", db.Name),
		zap.Uint64("chain_id", chainID),
		zap.Uint64("from_height", fromHeight),
		zap.Uint64("to_height", toHeight),
		zap.Int("tables_deleted", len(tablesToDelete)),
		zap.Duration("duration", time.Since(startTime)))

	return nil
}

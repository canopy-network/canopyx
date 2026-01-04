package chain

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

// DB represents a database associated with a blockchain and provides methods to manage and query its data.
// It includes a database client, a logger for capturing logs, the chain's name, and its unique identifier.
// It implements Store.
type DB struct {
	clickhouse.Client
	Name    string
	ChainID uint64
}

// NewWithPoolConfig creates and initializes a chain-specific ClickHouse database instance with custom pool configuration.
// This allows passing calculated pool sizes directly instead of relying on environment variables.
func NewWithPoolConfig(ctx context.Context, logger *zap.Logger, chainID uint64, poolConfig clickhouse.PoolConfig) (*DB, error) {
	chainIDStr := fmt.Sprintf("chain_%d", chainID)
	dbName := clickhouse.SanitizeName(chainIDStr)

	client, err := clickhouse.New(ctx, logger.With(
		zap.String("db", dbName),
		zap.String("component", poolConfig.Component),
		zap.Uint64("chainID", chainID),
	), dbName, &poolConfig)
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

// NewWithSharedClient creates a chain DB that reuses an existing ClickHouse connection pool.
// This is efficient for scenarios where many chain DBs are accessed occasionally (e.g., admin web UI).
// The database and tables must already exist - this constructor does NOT call InitializeDB.
// Use this when you want to avoid creating separate connection pools for each chain.
func NewWithSharedClient(client clickhouse.Client, chainID uint64) *DB {
	chainIDStr := fmt.Sprintf("chain_%d", chainID)
	dbName := clickhouse.SanitizeName(chainIDStr)

	return &DB{
		Client:  client, // Reuse existing connection pool
		Name:    dbName,
		ChainID: chainID,
	}
}

// Close terminates the underlying ClickHouse connection.
func (db *DB) Close() error {
	return db.Db.Close()
}

func (db *DB) GetConnection() driver.Conn {
	return db.Db
}

// InitializeDB ensures the required database and tables for indexing are created if they do not already exist.
// Uses PARALLEL WITH to batch all CREATE statements into 3 DDL operations instead of 40+:
//  1. CREATE DATABASE
//  2. All CREATE TABLEs with PARALLEL WITH (single batch)
//  3. All CREATE MATERIALIZED VIEWs with PARALLEL WITH (single batch)
func (db *DB) InitializeDB(ctx context.Context) error {
	initStart := time.Now()

	if err := db.CreateDbIfNotExists(ctx, db.Name); err != nil {
		return fmt.Errorf("failed to create database %s: %w", db.Name, err)
	}

	// Create all base tables in parallel using goroutines
	// This maximizes usage of distributed_ddl pool_size by issuing all CREATE TABLE
	// statements concurrently. Each ON CLUSTER operation then gets parallelized by ClickHouse.
	db.Logger.Info("Creating all base tables in parallel",
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
	db.Logger.Info("Creating all materialized views in parallel",
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

	var wg3 sync.WaitGroup
	errChan3 := make(chan error, len(viewOps)) // Buffer for 9 materialized view operations

	// Launch all view init operations in parallel
	for _, op := range viewOps {
		wg3.Add(1)
		go func(name string, fn func(context.Context) error) {
			defer wg3.Done()
			if err := fn(ctx); err != nil {
				errChan3 <- fmt.Errorf("init %s view: %w", name, err)
			}
		}(op.name, op.fn)
	}

	// Wait for all goroutines to complete
	wg3.Wait()
	close(errChan3)

	// Check for any errors
	for err := range errChan3 {
		return err
	}

	db.Logger.Info("Chain database initialization complete",
		zap.String("database", db.Name),
		zap.Duration("total_duration", time.Since(initStart)))

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
//
// Consistency and memory settings are configured at the ClickHouse server level:
// - insert_quorum=2: Ensures all replicas have data before INSERT returns
// - max_bytes_before_external_group_by/sort: Allows disk spill for large MVs
// See: deploy/k8s/clickhouse/base/installation.yaml
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
// Uses lightweight DELETE for instant, non-blocking deletion (ClickHouse 23.3+).
// The operation is idempotent - safe to retry if it fails.
func (db *DB) CleanEntityStaging(ctx context.Context, entity entities.Entity, height uint64) error {
	// Validate entity is known
	if !entity.IsValid() {
		return fmt.Errorf("invalid entity: %q", entity)
	}

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

	db.Logger.Debug("Cleaned staging data (lightweight DELETE)",
		zap.String("entity", entity.String()),
		zap.Uint64("height", height),
		zap.String("database", db.Name))

	return nil
}

// CleanAllEntitiesStaging removes promoted data from all staging tables using parallel goroutines.
// This executes all DELETE statements concurrently for maximum performance.
// Uses lightweight DELETE for instant, non-blocking deletion (ClickHouse 23.3+).
// The operation is idempotent - safe to retry if it fails.
func (db *DB) CleanAllEntitiesStaging(ctx context.Context, entitiesToClean []entities.Entity, height uint64) error {
	if len(entitiesToClean) == 0 {
		return nil
	}

	db.Logger.Debug("Cleaning all staging data with parallel goroutines",
		zap.Uint64("height", height),
		zap.String("database", db.Name),
		zap.Int("entity_count", len(entitiesToClean)))

	var wg sync.WaitGroup
	errChan := make(chan error, len(entitiesToClean))

	// Launch DELETE for each entity in parallel
	for _, entity := range entitiesToClean {
		if !entity.IsValid() {
			return fmt.Errorf("invalid entity: %q", entity)
		}

		wg.Add(1)
		go func(entity entities.Entity) {
			defer wg.Done()

			query := fmt.Sprintf(
				`DELETE FROM "%s"."%s" WHERE height = ?`,
				db.Name, entity.StagingTableName(),
			)

			if err := db.Db.Exec(ctx, query, height); err != nil {
				errChan <- fmt.Errorf("clean %s staging at height %d: %w", entity.String(), height, err)
			}
		}(entity)
	}

	// Wait for all DELETEs to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		db.Logger.Warn("Failed to clean staging data",
			zap.Uint64("height", height),
			zap.String("database", db.Name),
			zap.Error(err))
		return err
	}

	db.Logger.Debug("Cleaned all staging data (parallel goroutines)",
		zap.Uint64("height", height),
		zap.String("database", db.Name),
		zap.Int("entity_count", len(entitiesToClean)))

	return nil
}

// CleanStagingBatch removes data for multiple heights from all staging tables.
// Uses a single DELETE ... WHERE height IN (...) per table, creating only ONE mutation
// per table instead of one per height. This is much more efficient for batch cleanup.
//
// This method is designed for the hourly cleanup workflow that cleans heights
// that have been confirmed indexed (with a safety buffer).
func (db *DB) CleanStagingBatch(ctx context.Context, heights []uint64) (int, error) {
	if len(heights) == 0 {
		return 0, nil
	}

	// Get entities that use the staging pattern (excludes PollSnapshots which has no staging table)
	stagingEntities := entities.AllWithStaging()

	db.Logger.Info("Cleaning staging data in batch",
		zap.String("database", db.Name),
		zap.Int("heights", len(heights)),
		zap.Int("entities", len(stagingEntities)))

	var wg sync.WaitGroup
	errChan := make(chan error, len(stagingEntities))
	cleanedCount := 0

	// Launch DELETE for each entity in parallel
	for _, entity := range stagingEntities {
		wg.Add(1)
		go func(entity entities.Entity) {
			defer wg.Done()

			// Build the IN clause with placeholders
			// DELETE FROM staging WHERE height IN (?, ?, ?, ...)
			query := fmt.Sprintf(
				`DELETE FROM "%s"."%s" WHERE height IN ?`,
				db.Name, entity.StagingTableName(),
			)

			if err := db.Db.Exec(ctx, query, heights); err != nil {
				errChan <- fmt.Errorf("batch clean %s staging: %w", entity.String(), err)
			}
		}(entity)
	}

	// Wait for all DELETEs to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		db.Logger.Warn("Failed to batch clean staging data",
			zap.String("database", db.Name),
			zap.Error(err))
		return 0, err
	}

	cleanedCount = len(stagingEntities)

	db.Logger.Info("Batch cleaned staging data",
		zap.String("database", db.Name),
		zap.Int("heights", len(heights)),
		zap.Int("entities_cleaned", cleanedCount))

	return cleanedCount, nil
}

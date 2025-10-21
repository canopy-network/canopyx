package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"
	pkgdb "github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/db/entities"
	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/tests/integration/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// TestPromoteEntity_Success verifies that data is successfully moved from staging to production
func TestPromoteEntity_Success(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	// Setup test database
	chainDB, cleanup := setupTestChainDB(t)
	defer cleanup()

	// Create test tables (blocks and blocks_staging)
	err := createTestTables(ctx, chainDB, entities.Blocks)
	require.NoError(t, err)

	// Insert test data into staging
	testHeight := uint64(1000)
	err = insertTestStagingData(ctx, chainDB, entities.Blocks, testHeight)
	require.NoError(t, err)

	// Execute promotion
	err = chainDB.PromoteEntity(ctx, entities.Blocks, testHeight)
	assert.NoError(t, err)

	// Verify data exists in production table
	exists, err := verifyProductionData(ctx, chainDB, entities.Blocks, testHeight)
	assert.NoError(t, err)
	assert.True(t, exists, "Data should exist in production table after promotion")

	// Verify data still exists in staging (promotion doesn't delete)
	exists, err = verifyStagingData(ctx, chainDB, entities.Blocks, testHeight)
	assert.NoError(t, err)
	assert.True(t, exists, "Data should still exist in staging after promotion")
}

// TestPromoteEntity_InvalidEntity verifies error handling for invalid entity names
func TestPromoteEntity_InvalidEntity(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	db := setupTestChainDB(t, logger)
	defer cleanupTestDB(t, db)

	// Try to promote with invalid entity
	invalidEntity := entities.Entity("invalid_entity")
	err := db.PromoteEntity(ctx, invalidEntity, 1000)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid entity")
}

// TestPromoteEntity_Idempotent verifies that promotion is idempotent
func TestPromoteEntity_Idempotent(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	db := setupTestChainDB(t, logger)
	defer cleanupTestDB(t, db)

	// Create test tables
	err := createTestTables(ctx, db, entities.Blocks)
	require.NoError(t, err)

	// Insert test data into staging
	testHeight := uint64(2000)
	err = insertTestStagingData(ctx, db, entities.Blocks, testHeight)
	require.NoError(t, err)

	// Promote once
	err = db.PromoteEntity(ctx, entities.Blocks, testHeight)
	assert.NoError(t, err)

	// Promote again (should be idempotent)
	err = db.PromoteEntity(ctx, entities.Blocks, testHeight)
	assert.NoError(t, err, "Second promotion should not error (idempotent)")

	// Verify data still exists in production
	exists, err := verifyProductionData(ctx, db, entities.Blocks, testHeight)
	assert.NoError(t, err)
	assert.True(t, exists)
}

// TestCleanEntityStaging_Success verifies staging data is deleted after cleaning
func TestCleanEntityStaging_Success(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	db := setupTestChainDB(t, logger)
	defer cleanupTestDB(t, db)

	// Create test tables
	err := createTestTables(ctx, db, entities.Blocks)
	require.NoError(t, err)

	// Insert test data into staging
	testHeight := uint64(3000)
	err = insertTestStagingData(ctx, db, entities.Blocks, testHeight)
	require.NoError(t, err)

	// Clean staging data
	err = db.CleanEntityStaging(ctx, entities.Blocks, testHeight)
	assert.NoError(t, err)

	// Verify data no longer exists in staging
	exists, err := verifyStagingData(ctx, db, entities.Blocks, testHeight)
	assert.NoError(t, err)
	assert.False(t, exists, "Data should not exist in staging after cleanup")
}

// TestCleanEntityStaging_Idempotent verifies that cleanup is idempotent
func TestCleanEntityStaging_Idempotent(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	db := setupTestChainDB(t, logger)
	defer cleanupTestDB(t, db)

	// Create test tables
	err := createTestTables(ctx, db, entities.Blocks)
	require.NoError(t, err)

	// Insert test data into staging
	testHeight := uint64(4000)
	err = insertTestStagingData(ctx, db, entities.Blocks, testHeight)
	require.NoError(t, err)

	// Clean once
	err = db.CleanEntityStaging(ctx, entities.Blocks, testHeight)
	assert.NoError(t, err)

	// Clean again (should be idempotent)
	err = db.CleanEntityStaging(ctx, entities.Blocks, testHeight)
	assert.NoError(t, err, "Second cleanup should not error (idempotent)")
}

// TestValidateQueryHeight_Valid tests validation with a valid indexed height
func TestValidateQueryHeight_Valid(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	db := setupTestChainDB(t, logger)
	defer cleanupTestDB(t, db)

	// Setup index_progress with test data
	err := setupIndexProgress(ctx, db, 5000)
	require.NoError(t, err)

	// Test with valid height
	requestedHeight := uint64(3000)
	validHeight, err := db.ValidateQueryHeight(ctx, &requestedHeight)

	assert.NoError(t, err)
	assert.Equal(t, requestedHeight, validHeight)
}

// TestValidateQueryHeight_NotIndexed tests error when height not yet indexed
func TestValidateQueryHeight_NotIndexed(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	db := setupTestChainDB(t, logger)
	defer cleanupTestDB(t, db)

	// Setup index_progress with test data
	err := setupIndexProgress(ctx, db, 5000)
	require.NoError(t, err)

	// Test with height beyond indexed
	requestedHeight := uint64(6000)
	_, err = db.ValidateQueryHeight(ctx, &requestedHeight)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not fully indexed")
	assert.Contains(t, err.Error(), "latest indexed: 5000")
}

// TestValidateQueryHeight_Latest tests default to latest when nil height
func TestValidateQueryHeight_Latest(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	db := setupTestChainDB(t, logger)
	defer cleanupTestDB(t, db)

	// Setup index_progress with test data
	expectedLatest := uint64(7500)
	err := setupIndexProgress(ctx, db, expectedLatest)
	require.NoError(t, err)

	// Test with nil height (should return latest)
	validHeight, err := db.ValidateQueryHeight(ctx, nil)

	assert.NoError(t, err)
	assert.Equal(t, expectedLatest, validHeight)
}

// TestGetFullyIndexedHeight_Success tests retrieval of current indexed height
func TestGetFullyIndexedHeight_Success(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	db := setupTestChainDB(t, logger)
	defer cleanupTestDB(t, db)

	// Setup index_progress with test data
	expectedHeight := uint64(8888)
	err := setupIndexProgress(ctx, db, expectedHeight)
	require.NoError(t, err)

	// Get fully indexed height
	height, err := db.GetFullyIndexedHeight(ctx)

	assert.NoError(t, err)
	assert.Equal(t, expectedHeight, height)
}

// TestGetFullyIndexedHeight_NoData tests error when no indexed data exists
func TestGetFullyIndexedHeight_NoData(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	db := setupTestChainDB(t, logger)
	defer cleanupTestDB(t, db)

	// Don't setup any index_progress data
	_, err := db.GetFullyIndexedHeight(ctx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no indexed heights found")
}

// TestPromoteAllEntities tests promoting all defined entities
func TestPromoteAllEntities(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	db := setupTestChainDB(t, logger)
	defer cleanupTestDB(t, db)

	testHeight := uint64(9999)

	// Test each entity defined in the entities package
	for _, entity := range entities.All() {
		t.Run(fmt.Sprintf("Promote_%s", entity), func(t *testing.T) {
			// Create tables for this entity
			err := createTestTables(ctx, db, entity)
			require.NoError(t, err)

			// Insert test data into staging
			err = insertTestStagingData(ctx, db, entity, testHeight)
			require.NoError(t, err)

			// Promote
			err = db.PromoteEntity(ctx, entity, testHeight)
			assert.NoError(t, err)

			// Clean staging
			err = db.CleanEntityStaging(ctx, entity, testHeight)
			assert.NoError(t, err)
		})
	}
}

// Helper functions

func setupTestChainDB(t *testing.T, logger *zap.Logger) *pkgdb.ChainDB {
	// This would connect to a test ClickHouse instance
	// For unit tests, you might use a mock or test container
	// Example using testcontainers or local test instance:

	ctx := context.Background()

	// Connect to test ClickHouse
	conn, err := ch.Open(&ch.Options{
		Addr:     []string{"localhost:9000"},
		Database: "test_db",
		User:     "default",
		Password: "",
	})
	require.NoError(t, err)

	// Create test database
	dbName := fmt.Sprintf("test_chain_%d", time.Now().UnixNano())
	_, err = conn.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", dbName))
	require.NoError(t, err)

	// Create admin database and tables for index_progress
	_, err = conn.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS admin")
	require.NoError(t, err)

	err = admin.InitIndexProgress(ctx, conn, "admin")
	require.NoError(t, err)

	return &pkgdb.ChainDB{
		Client: pkgdb.Client{
			Db:     conn,
			Logger: logger.Sugar(),
		},
		Name:    dbName,
		ChainID: "test-chain",
	}
}

func cleanupTestDB(t *testing.T, db *pkgdb.ChainDB) {
	ctx := context.Background()

	// Drop test database
	_, err := db.Db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", db.Name))
	assert.NoError(t, err)

	// Clean test data from admin database
	_, err = db.Db.ExecContext(ctx, "ALTER TABLE admin.index_progress DELETE WHERE chain_id = ?", db.ChainID)
	assert.NoError(t, err)

	err = db.Close()
	assert.NoError(t, err)
}

func createTestTables(ctx context.Context, db *pkgdb.ChainDB, entity entities.Entity) error {
	// Create production table based on entity type
	prodTableSQL := getCreateTableSQL(entity, entity.TableName(), db.Name)
	if _, err := db.Db.ExecContext(ctx, prodTableSQL); err != nil {
		return fmt.Errorf("create production table: %w", err)
	}

	// Create staging table
	stagingTableSQL := getCreateTableSQL(entity, entity.StagingTableName(), db.Name)
	if _, err := db.Db.ExecContext(ctx, stagingTableSQL); err != nil {
		return fmt.Errorf("create staging table: %w", err)
	}

	return nil
}

func getCreateTableSQL(entity entities.Entity, tableName, dbName string) string {
	switch entity {
	case entities.Blocks:
		return fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s.%s (
				height UInt64,
				hash String,
				parent_hash String,
				time DateTime64(6),
				proposer_address String,
				size Int32
			) ENGINE = ReplacingMergeTree()
			ORDER BY height`, dbName, tableName)

	case entities.Transactions:
		return fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s.%s (
				height UInt64,
				tx_hash String,
				time DateTime64(6),
				height_time DateTime64(6),
				message_type String,
				signer String,
				counterparty Nullable(String),
				amount Nullable(UInt64),
				fee UInt64,
				created_height UInt64
			) ENGINE = ReplacingMergeTree()
			ORDER BY (height, tx_hash)`, dbName, tableName)

	case entities.BlockSummaries:
		return fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s.%s (
				height UInt64,
				height_time DateTime64(6),
				num_txs UInt32
			) ENGINE = ReplacingMergeTree()
			ORDER BY height`, dbName, tableName)

	default:
		// Generic table for testing other entities
		return fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s.%s (
				height UInt64,
				data String
			) ENGINE = ReplacingMergeTree()
			ORDER BY height`, dbName, tableName)
	}
}

func insertTestStagingData(ctx context.Context, db *pkgdb.ChainDB, entity entities.Entity, height uint64) error {
	switch entity {
	case entities.Blocks:
		block := &indexer.Block{
			Height:          height,
			Hash:            fmt.Sprintf("hash_%d", height),
			ParentHash:      fmt.Sprintf("parent_%d", height-1),
			Time:            time.Now(),
			ProposerAddress: "proposer_test",
			Size:            1234,
		}
		_, err := db.Db.NewInsert().
			Model(block).
			Table(fmt.Sprintf("%s.%s", db.Name, entity.StagingTableName())).
			Exec(ctx)
		return err

	default:
		// Generic insert for other entities
		query := fmt.Sprintf(
			"INSERT INTO %s.%s (height, data) VALUES (?, ?)",
			db.Name, entity.StagingTableName(),
		)
		_, err := db.Db.ExecContext(ctx, query, height, "test_data")
		return err
	}
}

func verifyProductionData(ctx context.Context, db *pkgdb.ChainDB, entity entities.Entity, height uint64) (bool, error) {
	var count int
	query := fmt.Sprintf(
		"SELECT count() FROM %s.%s WHERE height = ?",
		db.Name, entity.TableName(),
	)
	err := db.Db.NewRaw(query, height).Scan(ctx, &count)
	return count > 0, err
}

func verifyStagingData(ctx context.Context, db *pkgdb.ChainDB, entity entities.Entity, height uint64) (bool, error) {
	var count int
	query := fmt.Sprintf(
		"SELECT count() FROM %s.%s WHERE height = ?",
		db.Name, entity.StagingTableName(),
	)
	err := db.Db.NewRaw(query, height).Scan(ctx, &count)
	return count > 0, err
}

func setupIndexProgress(ctx context.Context, db *pkgdb.ChainDB, maxHeight uint64) error {
	// Insert test data into index_progress
	for h := uint64(1); h <= maxHeight; h += 100 {
		ip := &admin.IndexProgress{
			ChainID:        db.ChainID,
			Height:         h,
			IndexedAt:      time.Now(),
			IndexingTime:   1.0,
			IndexingTimeMs: 1000.0,
			IndexingDetail: "{}",
		}
		if _, err := db.Db.NewInsert().Model(ip).Exec(ctx); err != nil {
			return err
		}
	}

	// Insert the max height
	ip := &admin.IndexProgress{
		ChainID:        db.ChainID,
		Height:         maxHeight,
		IndexedAt:      time.Now(),
		IndexingTime:   1.0,
		IndexingTimeMs: 1000.0,
		IndexingDetail: "{}",
	}
	_, err := db.Db.NewInsert().Model(ip).Exec(ctx)
	return err
}

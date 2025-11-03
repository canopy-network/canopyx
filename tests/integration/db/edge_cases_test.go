package db

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/entities"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEdgeCase_EmptyArrays tests handling of empty arrays in models.
func TestEdgeCase_EmptyArrays(t *testing.T) {
	adminDB := createAdminStore(t, "edge_empty_arrays")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create chain with empty RPC endpoints array
	chain := generateChain(1001)
	chain.RPCEndpoints = []string{}

	err := adminDB.UpsertChain(ctx, chain)
	require.NoError(t, err)

	// Retrieve and verify empty array is preserved
	retrieved, err := adminDB.GetChain(ctx, 1001)
	require.NoError(t, err)
	assert.NotNil(t, retrieved.RPCEndpoints)
	assert.Empty(t, retrieved.RPCEndpoints)
}

// TestEdgeCase_LargeStringFields tests handling of very long strings.
func TestEdgeCase_LargeStringFields(t *testing.T) {
	chainDB := createChainStore(t, 60001)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create transaction with very long message
	tx := generateTransaction(100, 0)
	tx.Msg = strings.Repeat("x", 10000) // 10KB message

	err := chainDB.InsertTransactionsStaging(ctx, []*indexermodels.Transaction{tx})
	require.NoError(t, err)

	// Promote and verify
	err = chainDB.PromoteEntity(ctx, entities.Transactions, 100)
	require.NoError(t, err)

	// Query to verify it was stored correctly
	var retrievedMsg string
	query := `SELECT msg FROM txs FINAL WHERE height = 100 AND tx_hash = ? LIMIT 1`
	err = chainDB.Db.QueryRow(ctx, query, tx.TxHash).Scan(&retrievedMsg)
	require.NoError(t, err)
	assert.Equal(t, tx.Msg, retrievedMsg)
}

// TestEdgeCase_LargeBatchInsert tests inserting large batches of data.
func TestEdgeCase_LargeBatchInsert(t *testing.T) {
	chainDB := createChainStore(t, 60002)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create 1000 transactions
	txs := make([]*indexermodels.Transaction, 1000)
	for i := 0; i < 1000; i++ {
		txs[i] = generateTransaction(100, i)
	}

	err := chainDB.InsertTransactionsStaging(ctx, txs)
	require.NoError(t, err)

	// Promote and verify count
	err = chainDB.PromoteEntity(ctx, entities.Transactions, 100)
	require.NoError(t, err)

	var count uint64
	query := `SELECT count(*) FROM txs WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1000), count)
}

// TestEdgeCase_NullableFields tests handling of nullable fields.
func TestEdgeCase_NullableFields(t *testing.T) {
	chainDB := createChainStore(t, 60003)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create transaction with nil nullable fields
	tx := generateTransaction(100, 0)
	tx.Counterparty = nil
	tx.Amount = nil
	tx.ValidatorAddress = nil
	tx.Commission = nil
	tx.ChainID = nil
	tx.PublicKey = nil
	tx.Signature = nil

	err := chainDB.InsertTransactionsStaging(ctx, []*indexermodels.Transaction{tx})
	require.NoError(t, err)

	err = chainDB.PromoteEntity(ctx, entities.Transactions, 100)
	require.NoError(t, err)

	// Verify nulls are preserved
	var (
		counterparty     *string
		amount           *uint64
		validatorAddress *string
	)
	query := `SELECT counterparty, amount, validator_address FROM txs FINAL WHERE height = 100 LIMIT 1`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&counterparty, &amount, &validatorAddress)
	require.NoError(t, err)
	assert.Nil(t, counterparty)
	assert.Nil(t, amount)
	assert.Nil(t, validatorAddress)
}

// TestEdgeCase_NullableFieldsWithValues tests nullable fields with actual values.
func TestEdgeCase_NullableFieldsWithValues(t *testing.T) {
	chainDB := createChainStore(t, 60004)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create transaction with all nullable fields populated
	tx := generateTransaction(100, 0)
	counterparty := "recipient-addr"
	amount := uint64(1000000)
	validatorAddr := "validator-addr"
	commission := 5.5
	chainID := uint64(1)

	tx.Counterparty = &counterparty
	tx.Amount = &amount
	tx.ValidatorAddress = &validatorAddr
	tx.Commission = &commission
	tx.ChainID = &chainID

	err := chainDB.InsertTransactionsStaging(ctx, []*indexermodels.Transaction{tx})
	require.NoError(t, err)

	err = chainDB.PromoteEntity(ctx, entities.Transactions, 100)
	require.NoError(t, err)

	// Verify values are correctly stored
	var (
		retrievedCounterparty *string
		retrievedAmount       *uint64
		retrievedValidator    *string
		retrievedCommission   *float64
		retrievedChainID      *uint64
	)
	query := `SELECT counterparty, amount, validator_address, commission, chain_id
	          FROM txs FINAL WHERE height = 100 LIMIT 1`
	err = chainDB.Db.QueryRow(ctx, query).Scan(
		&retrievedCounterparty,
		&retrievedAmount,
		&retrievedValidator,
		&retrievedCommission,
		&retrievedChainID,
	)
	require.NoError(t, err)
	require.NotNil(t, retrievedCounterparty)
	assert.Equal(t, counterparty, *retrievedCounterparty)
	require.NotNil(t, retrievedAmount)
	assert.Equal(t, amount, *retrievedAmount)
	require.NotNil(t, retrievedValidator)
	assert.Equal(t, validatorAddr, *retrievedValidator)
}

// TestEdgeCase_FINALModifierDeduplication tests FINAL modifier correctly deduplicates.
func TestEdgeCase_FINALModifierDeduplication(t *testing.T) {
	chainDB := createChainStore(t, 60005)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert same block twice (simulating re-indexing)
	block1 := generateBlock(100)
	block1.Hash = "hash-v1"
	err := chainDB.InsertBlocksStaging(ctx, block1)
	require.NoError(t, err)
	err = chainDB.PromoteEntity(ctx, entities.Blocks, 100)
	require.NoError(t, err)

	// Insert again with different data
	time.Sleep(10 * time.Millisecond) // Ensure different timestamp
	block2 := generateBlock(100)
	block2.Hash = "hash-v2"
	err = chainDB.InsertBlocksStaging(ctx, block2)
	require.NoError(t, err)
	err = chainDB.PromoteEntity(ctx, entities.Blocks, 100)
	require.NoError(t, err)

	// FINAL should return only the latest version
	block, err := chainDB.GetBlock(ctx, 100)
	require.NoError(t, err)
	assert.Equal(t, "hash-v2", block.Hash, "FINAL should return latest version")
}

// TestEdgeCase_ContextCancellation tests behavior with cancelled context.
func TestEdgeCase_ContextCancellation(t *testing.T) {
	chainDB := createChainStore(t, 60006)

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	block := generateBlock(100)
	err := chainDB.InsertBlocksStaging(ctx, block)
	assert.Error(t, err, "should fail with cancelled context")
}

// TestEdgeCase_ContextTimeout tests behavior with timed-out context.
func TestEdgeCase_ContextTimeout(t *testing.T) {
	chainDB := createChainStore(t, 60007)

	// Create a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for timeout
	time.Sleep(1 * time.Millisecond)

	block := generateBlock(100)
	err := chainDB.InsertBlocksStaging(ctx, block)
	assert.Error(t, err, "should fail with timeout context")
}

// TestEdgeCase_ConcurrentInserts tests concurrent inserts to staging tables.
func TestEdgeCase_ConcurrentInserts(t *testing.T) {
	chainDB := createChainStore(t, 60008)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert different heights concurrently
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for height := uint64(100); height < 110; height++ {
		wg.Add(1)
		go func(h uint64) {
			defer wg.Done()
			block := generateBlock(h)
			if err := chainDB.InsertBlocksStaging(ctx, block); err != nil {
				errors <- err
			}
		}(height)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		require.NoError(t, err, "concurrent inserts should succeed")
	}

	// Verify all blocks were inserted
	for height := uint64(100); height < 110; height++ {
		err := chainDB.PromoteEntity(ctx, entities.Blocks, height)
		require.NoError(t, err)
	}

	var count uint64
	query := `SELECT count(*) FROM blocks WHERE height >= 100 AND height < 110`
	err := chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(10), count)
}

// TestEdgeCase_ConcurrentPromotions tests concurrent promotions of different entities.
func TestEdgeCase_ConcurrentPromotions(t *testing.T) {
	chainDB := createChainStore(t, 60009)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert data for multiple entity types
	block := generateBlock(100)
	require.NoError(t, chainDB.InsertBlocksStaging(ctx, block))

	txs := []*indexermodels.Transaction{generateTransaction(100, 0)}
	require.NoError(t, chainDB.InsertTransactionsStaging(ctx, txs))

	accounts := []*indexermodels.Account{generateAccount("addr1", 100, 1000000)}
	require.NoError(t, chainDB.InsertAccountsStaging(ctx, accounts))

	events := []*indexermodels.Event{generateEvent(100, 0)}
	require.NoError(t, chainDB.InsertEventsStaging(ctx, events))

	// Promote all concurrently
	entitiesToPromote := []entities.Entity{
		entities.Blocks,
		entities.Transactions,
		entities.Accounts,
		entities.Events,
	}

	var wg sync.WaitGroup
	errors := make(chan error, len(entitiesToPromote))

	for _, entity := range entitiesToPromote {
		wg.Add(1)
		go func(e entities.Entity) {
			defer wg.Done()
			if err := chainDB.PromoteEntity(ctx, e, 100); err != nil {
				errors <- err
			}
		}(entity)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		require.NoError(t, err, "concurrent promotions should succeed")
	}

	// Verify all entities were promoted
	_, err := chainDB.GetBlock(ctx, 100)
	assert.NoError(t, err)

	var txCount, accountCount, eventCount uint64
	require.NoError(t, chainDB.Db.QueryRow(ctx, `SELECT count(*) FROM txs WHERE height = 100`).Scan(&txCount))
	require.NoError(t, chainDB.Db.QueryRow(ctx, `SELECT count(*) FROM accounts WHERE height = 100`).Scan(&accountCount))
	require.NoError(t, chainDB.Db.QueryRow(ctx, `SELECT count(*) FROM events WHERE height = 100`).Scan(&eventCount))
	assert.Equal(t, uint64(1), txCount)
	assert.Equal(t, uint64(1), accountCount)
	assert.Equal(t, uint64(1), eventCount)
}

// TestEdgeCase_SpecialCharactersInStrings tests handling of special characters.
func TestEdgeCase_SpecialCharactersInStrings(t *testing.T) {
	chainDB := createChainStore(t, 60010)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create transaction with special characters
	tx := generateTransaction(100, 0)
	tx.Signer = `addr-with-"quotes"`
	tx.Msg = `{"key":"value with 'quotes' and \"escapes\" and unicode: 测试"}`

	err := chainDB.InsertTransactionsStaging(ctx, []*indexermodels.Transaction{tx})
	require.NoError(t, err)

	err = chainDB.PromoteEntity(ctx, entities.Transactions, 100)
	require.NoError(t, err)

	// Verify special characters are preserved
	var signer, msg string
	query := `SELECT signer, msg FROM txs FINAL WHERE height = 100 LIMIT 1`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&signer, &msg)
	require.NoError(t, err)
	assert.Equal(t, tx.Signer, signer)
	assert.Equal(t, tx.Msg, msg)
}

// TestEdgeCase_ZeroTimestamps tests handling of zero/epoch timestamps.
func TestEdgeCase_ZeroTimestamps(t *testing.T) {
	chainDB := createChainStore(t, 60011)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create validator signing info
	signingInfo := generateValidatorSigningInfo("validator-1", 100)

	err := chainDB.InsertValidatorSigningInfoStaging(ctx, []*indexermodels.ValidatorSigningInfo{signingInfo})
	require.NoError(t, err)

	err = chainDB.PromoteEntity(ctx, entities.ValidatorSigningInfo, 100)
	require.NoError(t, err)

	// Verify zero timestamp is handled
	var count uint64
	query := `SELECT count(*) FROM validator_signing_info WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

// TestEdgeCase_PromoteNonExistentEntity tests promoting entity with invalid name.
func TestEdgeCase_PromoteNonExistentEntity(t *testing.T) {
	chainDB := createChainStore(t, 60012)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try to promote invalid entity
	invalidEntity := entities.Entity("invalid_entity")
	err := chainDB.PromoteEntity(ctx, invalidEntity, 100)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid entity")
}

// TestEdgeCase_MultipleHeightsInStaging tests staging table with multiple heights.
func TestEdgeCase_MultipleHeightsInStaging(t *testing.T) {
	chainDB := createChainStore(t, 60013)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert blocks at different heights to staging
	for height := uint64(100); height <= 105; height++ {
		block := generateBlock(height)
		err := chainDB.InsertBlocksStaging(ctx, block)
		require.NoError(t, err)
	}

	// Promote only height 100
	err := chainDB.PromoteEntity(ctx, entities.Blocks, 100)
	require.NoError(t, err)

	// Clean only height 100 from staging
	err = chainDB.CleanEntityStaging(ctx, entities.Blocks, 100)
	require.NoError(t, err)

	// Verify height 100 is in production and not in staging
	_, err = chainDB.GetBlock(ctx, 100)
	assert.NoError(t, err)

	var stagingCount100 uint64
	query := `SELECT count(*) FROM blocks_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&stagingCount100)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), stagingCount100)

	// Verify heights 101-105 are still in staging
	var stagingCount101Plus uint64
	query = `SELECT count(*) FROM blocks_staging WHERE height > 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&stagingCount101Plus)
	require.NoError(t, err)
	assert.Equal(t, uint64(5), stagingCount101Plus)
}

// TestEdgeCase_VeryLargeHeightValue tests handling of very large height values.
func TestEdgeCase_VeryLargeHeightValue(t *testing.T) {
	chainDB := createChainStore(t, 60014)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use a very large height value (close to uint64 max)
	largeHeight := uint64(18446744073709551000) // Near max uint64

	block := generateBlock(largeHeight)
	err := chainDB.InsertBlocksStaging(ctx, block)
	require.NoError(t, err)

	err = chainDB.PromoteEntity(ctx, entities.Blocks, largeHeight)
	require.NoError(t, err)

	// Verify block can be retrieved
	retrieved, err := chainDB.GetBlock(ctx, largeHeight)
	require.NoError(t, err)
	assert.Equal(t, largeHeight, retrieved.Height)
}

// TestEdgeCase_EmptyBatchInsert tests inserting empty arrays.
func TestEdgeCase_EmptyBatchInsert(t *testing.T) {
	chainDB := createChainStore(t, 60015)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert empty transaction array
	err := chainDB.InsertTransactionsStaging(ctx, []*indexermodels.Transaction{})
	require.NoError(t, err, "empty batch insert should succeed")

	// Verify no data was inserted
	var count uint64
	query := `SELECT count(*) FROM txs_staging`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), count)
}

// TestEdgeCase_RepeatedCleanStaging tests cleaning staging multiple times.
func TestEdgeCase_RepeatedCleanStaging(t *testing.T) {
	chainDB := createChainStore(t, 60016)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert and promote block
	block := generateBlock(100)
	err := chainDB.InsertBlocksStaging(ctx, block)
	require.NoError(t, err)
	err = chainDB.PromoteEntity(ctx, entities.Blocks, 100)
	require.NoError(t, err)

	// Clean staging first time
	err = chainDB.CleanEntityStaging(ctx, entities.Blocks, 100)
	require.NoError(t, err)

	// Clean staging second time (should not error)
	err = chainDB.CleanEntityStaging(ctx, entities.Blocks, 100)
	require.NoError(t, err)

	// Clean staging third time (should still not error)
	err = chainDB.CleanEntityStaging(ctx, entities.Blocks, 100)
	require.NoError(t, err)
}

// TestEdgeCase_AdminStoreWithSpecialDatabaseName tests admin store with sanitized names.
func TestEdgeCase_AdminStoreWithSpecialDatabaseName(t *testing.T) {

	// Database names get sanitized, test that it works correctly
	adminDB := createAdminStore(t, "test-db-with-dashes_and_underscores")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	chain := generateChain(1001)
	err := adminDB.UpsertChain(ctx, chain)
	require.NoError(t, err)

	retrieved, err := adminDB.GetChain(ctx, 1001)
	require.NoError(t, err)
	assert.Equal(t, chain.ChainID, retrieved.ChainID)
}

// TestEdgeCase_MaxOpenConnections tests database under connection pressure.
func TestEdgeCase_MaxOpenConnections(t *testing.T) {
	chainDB := createChainStore(t, 60017)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Perform many concurrent queries to stress connection pool
	var wg sync.WaitGroup
	numQueries := 100
	errors := make(chan error, numQueries)

	for i := 0; i < numQueries; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// Try to query table schema
			_, err := chainDB.GetTableSchema(ctx, "blocks")
			if err != nil {
				errors <- fmt.Errorf("query %d failed: %w", index, err)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check that all queries succeeded
	errorCount := 0
	for err := range errors {
		t.Logf("Connection pool error: %v", err)
		errorCount++
	}
	assert.Equal(t, 0, errorCount, "all queries should succeed under connection pressure")
}

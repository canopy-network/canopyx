package indexer

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/entities"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"github.com/canopy-network/canopyx/tests/integration/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAccountsTableCreation verifies accounts table schema
func TestAccountsTableCreation(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainID := "test-chain-accounts-table"
	chain := helpers.CreateTestChain(chainID, "Test Chain Accounts")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	chainDB, err := helpers.NewChainDb(ctx, chainID)
	require.NoError(t, err, "Failed to create chain DB")
	defer chainDB.Close()

	// Verify accounts table exists
	var tableCount int
	query := `
		SELECT count()
		FROM system.tables
		WHERE database = ? AND name = 'accounts'
	`
	err = chainDB.Db.NewRaw(query, chainDB.DatabaseName()).Scan(ctx, &tableCount)
	require.NoError(t, err)
	assert.Equal(t, 1, tableCount, "accounts table should exist")

	// Verify engine
	var engine string
	query = `
		SELECT engine
		FROM system.tables
		WHERE database = ? AND name = 'accounts'
	`
	err = chainDB.Db.NewRaw(query, chainDB.DatabaseName()).Scan(ctx, &engine)
	require.NoError(t, err)
	assert.Contains(t, engine, "ReplacingMergeTree", "accounts table should use ReplacingMergeTree")

	// Verify ORDER BY clause includes address and height
	var sortingKey string
	query = `
		SELECT sorting_key
		FROM system.tables
		WHERE database = ? AND name = 'accounts'
	`
	err = chainDB.Db.NewRaw(query, chainDB.DatabaseName()).Scan(ctx, &sortingKey)
	require.NoError(t, err)
	assert.Contains(t, sortingKey, "address", "ORDER BY should include address")
	assert.Contains(t, sortingKey, "height", "ORDER BY should include height")
}

// TestAccountsInsertAndQuery tests inserting and querying account snapshots
func TestAccountsInsertAndQuery(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainID := "test-chain-accounts-insert"
	chain := helpers.CreateTestChain(chainID, "Test Chain Accounts Insert")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	chainDB, err := helpers.NewChainDb(ctx, chainID)
	require.NoError(t, err)
	defer chainDB.Close()

	// Create test accounts
	now := time.Now().UTC()
	accounts := []*indexer.Account{
		{
			Address:       "0x1111111111111111111111111111111111111111",
			Amount:        1000000,
			Height:        100,
			HeightTime:    now,
			CreatedHeight: 1, // Account created at genesis
		},
		{
			Address:       "0x2222222222222222222222222222222222222222",
			Amount:        2000000,
			Height:        100,
			HeightTime:    now,
			CreatedHeight: 50, // Account created at height 50
		},
		{
			Address:       "0x3333333333333333333333333333333333333333",
			Amount:        3000000,
			Height:        100,
			HeightTime:    now,
			CreatedHeight: 100, // New account at height 100
		},
	}

	// Insert accounts
	err = indexer.InsertAccountsProduction(ctx, chainDB.Db, accounts)
	require.NoError(t, err, "Failed to insert accounts")

	// Query all accounts
	var queriedAccounts []indexer.Account
	err = chainDB.Db.NewSelect().
		Model(&queriedAccounts).
		Where("height = ?", 100).
		Order("address ASC").
		Scan(ctx)
	require.NoError(t, err)
	assert.Len(t, queriedAccounts, 3)

	// Verify first account
	assert.Equal(t, "0x1111111111111111111111111111111111111111", queriedAccounts[0].Address)
	assert.Equal(t, uint64(1000000), queriedAccounts[0].Amount)
	assert.Equal(t, uint64(100), queriedAccounts[0].Height)
	assert.Equal(t, uint64(1), queriedAccounts[0].CreatedHeight)

	// Verify second account
	assert.Equal(t, "0x2222222222222222222222222222222222222222", queriedAccounts[1].Address)
	assert.Equal(t, uint64(2000000), queriedAccounts[1].Amount)
	assert.Equal(t, uint64(50), queriedAccounts[1].CreatedHeight)

	// Verify third account (newly created)
	assert.Equal(t, "0x3333333333333333333333333333333333333333", queriedAccounts[2].Address)
	assert.Equal(t, uint64(3000000), queriedAccounts[2].Amount)
	assert.Equal(t, uint64(100), queriedAccounts[2].CreatedHeight)
}

// TestAccountsChangeDetection tests the snapshot-on-change pattern
func TestAccountsChangeDetection(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainID := "test-chain-accounts-changes"
	chain := helpers.CreateTestChain(chainID, "Test Chain Accounts Changes")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	chainDB, err := helpers.NewChainDb(ctx, chainID)
	require.NoError(t, err)
	defer chainDB.Close()

	now := time.Now().UTC()

	// Simulate account at height 99
	accountsHeight99 := []*indexer.Account{
		{
			Address:       "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			Amount:        1000,
			Height:        99,
			HeightTime:    now.Add(-10 * time.Second),
			CreatedHeight: 1,
		},
		{
			Address:       "0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
			Amount:        2000,
			Height:        99,
			HeightTime:    now.Add(-10 * time.Second),
			CreatedHeight: 1,
		},
	}
	err = indexer.InsertAccountsProduction(ctx, chainDB.Db, accountsHeight99)
	require.NoError(t, err)

	// At height 100:
	// - 0xAAAA changes balance (1000 -> 1500)
	// - 0xBBBB stays same (2000 -> 2000) - should NOT create new snapshot
	// - 0xCCCC is new account

	// Simulate what change detection would find: only changed accounts
	changedAccountsHeight100 := []*indexer.Account{
		{
			Address:       "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", // Changed
			Amount:        1500,                                         // Balance changed
			Height:        100,
			HeightTime:    now,
			CreatedHeight: 1, // Preserved from previous
		},
		{
			Address:       "0xCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC", // New
			Amount:        3000,
			Height:        100,
			HeightTime:    now,
			CreatedHeight: 100, // New account created at height 100
		},
	}
	err = indexer.InsertAccountsProduction(ctx, chainDB.Db, changedAccountsHeight100)
	require.NoError(t, err)

	// Query all snapshots - should have 2 at height 99, 2 at height 100
	var allSnapshots []indexer.Account
	err = chainDB.Db.NewSelect().
		Model(&allSnapshots).
		Order("address ASC", "height ASC").
		Scan(ctx)
	require.NoError(t, err)
	assert.Len(t, allSnapshots, 4, "Should have 4 total snapshots")

	// Verify 0xAAAA has 2 versions
	var aaaaSnapshots []indexer.Account
	err = chainDB.Db.NewSelect().
		Model(&aaaaSnapshots).
		Where("address = ?", "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA").
		Order("height ASC").
		Scan(ctx)
	require.NoError(t, err)
	assert.Len(t, aaaaSnapshots, 2)
	assert.Equal(t, uint64(99), aaaaSnapshots[0].Height)
	assert.Equal(t, uint64(1000), aaaaSnapshots[0].Amount)
	assert.Equal(t, uint64(100), aaaaSnapshots[1].Height)
	assert.Equal(t, uint64(1500), aaaaSnapshots[1].Amount)

	// Verify 0xBBBB has only 1 version (no change, no new snapshot)
	var bbbbSnapshots []indexer.Account
	err = chainDB.Db.NewSelect().
		Model(&bbbbSnapshots).
		Where("address = ?", "0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB").
		Scan(ctx)
	require.NoError(t, err)
	assert.Len(t, bbbbSnapshots, 1, "Unchanged account should have only 1 snapshot")
	assert.Equal(t, uint64(99), bbbbSnapshots[0].Height)
	assert.Equal(t, uint64(2000), bbbbSnapshots[0].Amount)

	// Verify 0xCCCC is new
	var ccccSnapshots []indexer.Account
	err = chainDB.Db.NewSelect().
		Model(&ccccSnapshots).
		Where("address = ?", "0xCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC").
		Scan(ctx)
	require.NoError(t, err)
	assert.Len(t, ccccSnapshots, 1)
	assert.Equal(t, uint64(100), ccccSnapshots[0].Height)
	assert.Equal(t, uint64(100), ccccSnapshots[0].CreatedHeight)
}

// TestAccountsStagingPromoteClean tests the full staging -> promote -> clean workflow
func TestAccountsStagingPromoteClean(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainID := "test-chain-accounts-staging"
	chain := helpers.CreateTestChain(chainID, "Test Chain Accounts Staging")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	chainDB, err := helpers.NewChainDb(ctx, chainID)
	require.NoError(t, err)
	defer chainDB.Close()

	now := time.Now().UTC()
	testHeight := uint64(200)

	// Step 1: Insert to staging
	stagingAccounts := []*indexer.Account{
		{
			Address:       "0xDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD",
			Amount:        5000,
			Height:        testHeight,
			HeightTime:    now,
			CreatedHeight: testHeight,
		},
		{
			Address:       "0xEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEE",
			Amount:        6000,
			Height:        testHeight,
			HeightTime:    now,
			CreatedHeight: testHeight,
		},
	}

	stagingTableName := fmt.Sprintf("%s.%s", chainDB.DatabaseName(), entities.Accounts.StagingTableName())
	err = indexer.InsertAccountsStaging(ctx, chainDB.Db, stagingTableName, stagingAccounts)
	require.NoError(t, err, "Failed to insert to staging")

	// Verify data in staging
	var stagingCount int
	query := fmt.Sprintf("SELECT count() FROM %s WHERE height = ?", stagingTableName)
	err = chainDB.Db.NewRaw(query, testHeight).Scan(ctx, &stagingCount)
	require.NoError(t, err)
	assert.Equal(t, 2, stagingCount, "Should have 2 accounts in staging")

	// Step 2: Promote staging to production
	err = chainDB.PromoteEntity(ctx, entities.Accounts, testHeight)
	require.NoError(t, err, "Failed to promote accounts")

	// Verify data in production
	var prodAccounts []indexer.Account
	err = chainDB.Db.NewSelect().
		Model(&prodAccounts).
		Where("height = ?", testHeight).
		Order("address ASC").
		Scan(ctx)
	require.NoError(t, err)
	assert.Len(t, prodAccounts, 2, "Should have 2 accounts in production after promotion")
	assert.Equal(t, "0xDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD", prodAccounts[0].Address)
	assert.Equal(t, "0xEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEE", prodAccounts[1].Address)

	// Step 3: Clean staging
	err = chainDB.CleanEntityStaging(ctx, entities.Accounts, testHeight)
	require.NoError(t, err, "Failed to clean staging")

	// Verify staging is empty for this height
	time.Sleep(100 * time.Millisecond) // Wait for async DELETE
	err = chainDB.Db.NewRaw(query, testHeight).Scan(ctx, &stagingCount)
	require.NoError(t, err)
	assert.Equal(t, 0, stagingCount, "Staging should be empty after cleanup")

	// Verify production still has data
	var prodCount int
	query = fmt.Sprintf("SELECT count() FROM %s.accounts WHERE height = ?", chainDB.DatabaseName())
	err = chainDB.Db.NewRaw(query, testHeight).Scan(ctx, &prodCount)
	require.NoError(t, err)
	assert.Equal(t, 2, prodCount, "Production should still have data after staging cleanup")
}

// TestAccountsGenesisHandling tests handling of genesis accounts (height 1)
func TestAccountsGenesisHandling(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainID := "test-chain-accounts-genesis"
	chain := helpers.CreateTestChain(chainID, "Test Chain Accounts Genesis")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	chainDB, err := helpers.NewChainDb(ctx, chainID)
	require.NoError(t, err)
	defer chainDB.Close()

	now := time.Now().UTC()

	// Store genesis state (height 0) in genesis table
	genesisAccounts := []*rpc.Account{
		{Address: "0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", Amount: 10000},
		{Address: "0x0000000000000000000000000000000000000000", Amount: 20000},
	}

	// In production, genesis is stored via the genesis caching mechanism
	// For this test, we'll insert genesis accounts at height 0
	genesisTime := now.Add(-100 * time.Second)
	genesisSnapshots := []*indexer.Account{
		{
			Address:       genesisAccounts[0].Address,
			Amount:        genesisAccounts[0].Amount,
			Height:        0, // Genesis pseudo-height
			HeightTime:    genesisTime,
			CreatedHeight: 0,
		},
		{
			Address:       genesisAccounts[1].Address,
			Amount:        genesisAccounts[1].Amount,
			Height:        0,
			HeightTime:    genesisTime,
			CreatedHeight: 0,
		},
	}
	err = indexer.InsertAccountsProduction(ctx, chainDB.Db, genesisSnapshots)
	require.NoError(t, err)

	// At height 1, one account changes, one stays same, one is new
	height1Time := now.Add(-90 * time.Second)
	height1Accounts := []*indexer.Account{
		{
			Address:       "0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", // Changed
			Amount:        15000,                                       // Balance changed from 10000
			Height:        1,
			HeightTime:    height1Time,
			CreatedHeight: 0, // Existed in genesis
		},
		// 0x0000... stays same at 20000, so no snapshot
		{
			Address:       "0x1111111111111111111111111111111111111111", // New
			Amount:        5000,
			Height:        1,
			HeightTime:    height1Time,
			CreatedHeight: 1, // Created at height 1
		},
	}
	err = indexer.InsertAccountsProduction(ctx, chainDB.Db, height1Accounts)
	require.NoError(t, err)

	// Verify total snapshots
	var totalSnapshots int
	query := fmt.Sprintf("SELECT count() FROM %s.accounts", chainDB.DatabaseName())
	err = chainDB.Db.NewRaw(query).Scan(ctx, &totalSnapshots)
	require.NoError(t, err)
	assert.Equal(t, 4, totalSnapshots, "Should have 4 total snapshots (2 genesis + 2 at height 1)")

	// Verify account 0xFFFF has 2 versions (genesis and height 1)
	var ffffSnapshots []indexer.Account
	err = chainDB.Db.NewSelect().
		Model(&ffffSnapshots).
		Where("address = ?", "0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF").
		Order("height ASC").
		Scan(ctx)
	require.NoError(t, err)
	assert.Len(t, ffffSnapshots, 2)
	assert.Equal(t, uint64(0), ffffSnapshots[0].Height) // Genesis
	assert.Equal(t, uint64(10000), ffffSnapshots[0].Amount)
	assert.Equal(t, uint64(1), ffffSnapshots[1].Height) // Height 1
	assert.Equal(t, uint64(15000), ffffSnapshots[1].Amount)
	assert.Equal(t, uint64(0), ffffSnapshots[1].CreatedHeight) // Created in genesis

	// Verify account 0x0000 has only genesis version (no change)
	var zeroSnapshots []indexer.Account
	err = chainDB.Db.NewSelect().
		Model(&zeroSnapshots).
		Where("address = ?", "0x0000000000000000000000000000000000000000").
		Scan(ctx)
	require.NoError(t, err)
	assert.Len(t, zeroSnapshots, 1, "Unchanged genesis account should have only 1 snapshot")
	assert.Equal(t, uint64(0), zeroSnapshots[0].Height)

	// Verify new account 0x1111 created at height 1
	var newSnapshots []indexer.Account
	err = chainDB.Db.NewSelect().
		Model(&newSnapshots).
		Where("address = ?", "0x1111111111111111111111111111111111111111").
		Scan(ctx)
	require.NoError(t, err)
	assert.Len(t, newSnapshots, 1)
	assert.Equal(t, uint64(1), newSnapshots[0].Height)
	assert.Equal(t, uint64(1), newSnapshots[0].CreatedHeight)
}

// TestAccountsReplacingMergeTreeDeduplication tests ReplacingMergeTree deduplication
func TestAccountsReplacingMergeTreeDeduplication(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainID := "test-chain-accounts-dedup"
	chain := helpers.CreateTestChain(chainID, "Test Chain Accounts Dedup")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	chainDB, err := helpers.NewChainDb(ctx, chainID)
	require.NoError(t, err)
	defer chainDB.Close()

	now := time.Now().UTC()
	testHeight := uint64(300)
	address := "0x2222222222222222222222222222222222222222"

	// Insert same account snapshot multiple times (simulating retry/parallel indexing)
	for i := 0; i < 5; i++ {
		account := []*indexer.Account{
			{
				Address:       address,
				Amount:        9999,
				Height:        testHeight,
				HeightTime:    now,
				CreatedHeight: 1,
			},
		}
		err = indexer.InsertAccountsProduction(ctx, chainDB.Db, account)
		require.NoError(t, err)
	}

	// Give ClickHouse a moment to potentially merge (though it's async)
	time.Sleep(200 * time.Millisecond)

	// Count raw rows (may have duplicates before merging)
	var rawCount int
	query := fmt.Sprintf("SELECT count() FROM %s.accounts WHERE address = ? AND height = ?",
		chainDB.DatabaseName())
	err = chainDB.Db.NewRaw(query, address, testHeight).Scan(ctx, &rawCount)
	require.NoError(t, err)
	// Note: ClickHouse merges happen asynchronously, so we may see multiple rows

	// Query with FINAL to get deduplicated result
	var finalAccounts []indexer.Account
	err = chainDB.Db.NewSelect().
		Model(&finalAccounts).
		TableExpr("accounts FINAL"). // Force immediate deduplication
		Where("address = ? AND height = ?", address, testHeight).
		Scan(ctx)
	require.NoError(t, err)
	assert.Len(t, finalAccounts, 1, "Should have exactly 1 account after FINAL deduplication")
	assert.Equal(t, uint64(9999), finalAccounts[0].Amount)
}

// TestAccountsParallelIndexing tests that parallel indexing doesn't cause issues
func TestAccountsParallelIndexing(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainID := "test-chain-accounts-parallel"
	chain := helpers.CreateTestChain(chainID, "Test Chain Accounts Parallel")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	chainDB, err := helpers.NewChainDb(ctx, chainID)
	require.NoError(t, err)
	defer chainDB.Close()

	now := time.Now().UTC()

	// Simulate parallel indexing of different heights
	heights := []uint64{100, 101, 102, 103, 104}
	errChan := make(chan error, len(heights))

	for _, h := range heights {
		go func(height uint64) {
			accounts := []*indexer.Account{
				{
					Address:       fmt.Sprintf("0xAA%040d", height),
					Amount:        height * 1000,
					Height:        height,
					HeightTime:    now.Add(time.Duration(height) * time.Second),
					CreatedHeight: height,
				},
				{
					Address:       fmt.Sprintf("0xBB%040d", height),
					Amount:        height * 2000,
					Height:        height,
					HeightTime:    now.Add(time.Duration(height) * time.Second),
					CreatedHeight: height,
				},
			}
			err := indexer.InsertAccountsProduction(ctx, chainDB.Db, accounts)
			errChan <- err
		}(h)
	}

	// Wait for all goroutines
	for range heights {
		err := <-errChan
		require.NoError(t, err, "Parallel insert should not error")
	}

	// Verify all accounts were inserted
	var totalAccounts int
	query := fmt.Sprintf("SELECT count() FROM %s.accounts", chainDB.DatabaseName())
	err = chainDB.Db.NewRaw(query).Scan(ctx, &totalAccounts)
	require.NoError(t, err)
	assert.Equal(t, len(heights)*2, totalAccounts, "Should have 2 accounts per height")
}

// TestAccountsLargeDataset tests performance with many accounts
func TestAccountsLargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large dataset test in short mode")
	}

	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainID := "test-chain-accounts-large"
	chain := helpers.CreateTestChain(chainID, "Test Chain Accounts Large")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	chainDB, err := helpers.NewChainDb(ctx, chainID)
	require.NoError(t, err)
	defer chainDB.Close()

	now := time.Now().UTC()
	numAccounts := 10000
	testHeight := uint64(1000)

	// Create large batch of accounts
	accounts := make([]*indexer.Account, numAccounts)
	for i := 0; i < numAccounts; i++ {
		accounts[i] = &indexer.Account{
			Address:       fmt.Sprintf("0x%040x", i),
			Amount:        uint64(i * 100),
			Height:        testHeight,
			HeightTime:    now,
			CreatedHeight: testHeight,
		}
	}

	// Measure insert performance
	start := time.Now()
	err = indexer.InsertAccountsProduction(ctx, chainDB.Db, accounts)
	insertDuration := time.Since(start)
	require.NoError(t, err, "Failed to insert large dataset")
	t.Logf("Inserted %d accounts in %v", numAccounts, insertDuration)

	// Verify count
	var count int
	query := fmt.Sprintf("SELECT count() FROM %s.accounts WHERE height = ?", chainDB.DatabaseName())
	err = chainDB.Db.NewRaw(query, testHeight).Scan(ctx, &count)
	require.NoError(t, err)
	assert.Equal(t, numAccounts, count)

	// Measure query performance
	start = time.Now()
	var queriedAccounts []indexer.Account
	err = chainDB.Db.NewSelect().
		Model(&queriedAccounts).
		Where("height = ?", testHeight).
		Limit(1000).
		Scan(ctx)
	queryDuration := time.Since(start)
	require.NoError(t, err)
	assert.Len(t, queriedAccounts, 1000)
	t.Logf("Queried 1000 accounts (limited) in %v", queryDuration)

	// Performance assertions
	assert.Less(t, insertDuration, 10*time.Second, "Large insert should complete within 10 seconds")
	assert.Less(t, queryDuration, 1*time.Second, "Query should complete within 1 second")
}

// TestAccountsIsolationBetweenChains tests that accounts are properly isolated per chain
func TestAccountsIsolationBetweenChains(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	// Create two separate chains
	chain1ID := "test-chain-accounts-iso-1"
	chain2ID := "test-chain-accounts-iso-2"

	chain1 := helpers.CreateTestChain(chain1ID, "Test Chain Accounts Isolation 1")
	chain2 := helpers.CreateTestChain(chain2ID, "Test Chain Accounts Isolation 2")
	helpers.SeedData(ctx, t, helpers.WithChains(chain1, chain2))

	chainDB1, err := helpers.NewChainDb(ctx, chain1ID)
	require.NoError(t, err)
	defer chainDB1.Close()

	chainDB2, err := helpers.NewChainDb(ctx, chain2ID)
	require.NoError(t, err)
	defer chainDB2.Close()

	now := time.Now().UTC()

	// Insert accounts to chain 1
	accounts1 := []*indexer.Account{
		{
			Address:       "0xCHAIN1111111111111111111111111111111111",
			Amount:        1111,
			Height:        100,
			HeightTime:    now,
			CreatedHeight: 1,
		},
	}
	err = indexer.InsertAccountsProduction(ctx, chainDB1.Db, accounts1)
	require.NoError(t, err)

	// Insert accounts to chain 2 (same address, different chain)
	accounts2 := []*indexer.Account{
		{
			Address:       "0xCHAIN1111111111111111111111111111111111", // Same address
			Amount:        2222,                                        // Different balance
			Height:        100,
			HeightTime:    now,
			CreatedHeight: 1,
		},
	}
	err = indexer.InsertAccountsProduction(ctx, chainDB2.Db, accounts2)
	require.NoError(t, err)

	// Verify chain 1 has its data
	var chain1Accounts []indexer.Account
	err = chainDB1.Db.NewSelect().Model(&chain1Accounts).Scan(ctx)
	require.NoError(t, err)
	assert.Len(t, chain1Accounts, 1)
	assert.Equal(t, uint64(1111), chain1Accounts[0].Amount)

	// Verify chain 2 has its own data
	var chain2Accounts []indexer.Account
	err = chainDB2.Db.NewSelect().Model(&chain2Accounts).Scan(ctx)
	require.NoError(t, err)
	assert.Len(t, chain2Accounts, 1)
	assert.Equal(t, uint64(2222), chain2Accounts[0].Amount)

	// Verify databases are actually different
	assert.NotEqual(t, chainDB1.DatabaseName(), chainDB2.DatabaseName(),
		"Chain databases should be separate")
}

// TestAccountsEmptyResults tests handling of queries with no results
func TestAccountsEmptyResults(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainID := "test-chain-accounts-empty"
	chain := helpers.CreateTestChain(chainID, "Test Chain Accounts Empty")
	helpers.SeedData(ctx, t, helpers.WithChains(chain))

	chainDB, err := helpers.NewChainDb(ctx, chainID)
	require.NoError(t, err)
	defer chainDB.Close()

	// Query for non-existent address
	var accounts []indexer.Account
	err = chainDB.Db.NewSelect().
		Model(&accounts).
		Where("address = ?", "0xNONEXISTENT000000000000000000000000000").
		Scan(ctx)
	require.NoError(t, err)
	assert.Empty(t, accounts, "Should return empty result for non-existent address")

	// Query for non-existent height
	err = chainDB.Db.NewSelect().
		Model(&accounts).
		Where("height = ?", 999999).
		Scan(ctx)
	require.NoError(t, err)
	assert.Empty(t, accounts, "Should return empty result for non-existent height")
}

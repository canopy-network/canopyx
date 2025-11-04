package db

import (
	"context"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/entities"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChainStore_InitializeDB tests database initialization.
func TestChainStore_InitializeDB(t *testing.T) {
	chainDB := createChainStore(t, 10001)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Verify all production tables exist
	productionTables := []string{
		"blocks", "txs", "block_summaries", "accounts", "events",
		"pools", "orders", "dex_prices", "dex_orders", "dex_deposits",
		"dex_withdrawals", "dex_pool_points_by_holder", "params",
		"validators", "validator_signing_info", "committees",
		"committee_validators", "poll_snapshots", "genesis",
	}

	for _, table := range productionTables {
		columns, err := chainDB.DescribeTable(ctx, table)
		require.NoError(t, err, "table %s should exist", table)
		assert.NotEmpty(t, columns, "table %s should have columns", table)
	}

	// Verify all staging tables exist
	stagingTables := []string{
		"blocks_staging", "txs_staging", "block_summaries_staging",
		"accounts_staging", "events_staging", "pools_staging",
		"orders_staging", "dex_prices_staging", "dex_orders_staging",
		"dex_deposits_staging", "dex_withdrawals_staging",
		"dex_pool_points_by_holder_staging", "params_staging",
		"validators_staging", "validator_signing_info_staging",
		"committees_staging", "committee_validators_staging",
		"poll_snapshots_staging",
	}

	for _, table := range stagingTables {
		columns, err := chainDB.DescribeTable(ctx, table)
		require.NoError(t, err, "staging table %s should exist", table)
		assert.NotEmpty(t, columns, "staging table %s should have columns", table)
	}
}

// TestChainStore_DatabaseName tests DatabaseName method.
func TestChainStore_DatabaseName(t *testing.T) {
	chainDB := createChainStore(t, 10002)

	dbName := chainDB.DatabaseName()
	assert.Equal(t, "chain_10002", dbName)
}

// TestChainStore_ChainKey tests ChainKey method.
func TestChainStore_ChainKey(t *testing.T) {
	chainDB := createChainStore(t, 10003)

	chainKey := chainDB.ChainKey()
	assert.Equal(t, "10003", chainKey)
}

// TestChainStore_InsertBlocksStaging tests inserting blocks to staging.
func TestChainStore_InsertBlocksStaging(t *testing.T) {
	chainDB := createChainStore(t, 20001)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	block := generateBlock(100)
	err := chainDB.InsertBlocksStaging(ctx, block)
	require.NoError(t, err)

	// Verify block is in staging (not production yet)
	var count uint64
	query := `SELECT count(*) FROM blocks_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

// TestChainStore_InsertTransactionsStaging tests inserting transactions to staging.
func TestChainStore_InsertTransactionsStaging(t *testing.T) {
	chainDB := createChainStore(t, 20002)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	txs := []*indexermodels.Transaction{
		generateTransaction(100, 0),
		generateTransaction(100, 1),
		generateTransaction(100, 2),
	}

	err := chainDB.InsertTransactionsStaging(ctx, txs)
	require.NoError(t, err)

	// Verify transactions are in staging
	var count uint64
	query := `SELECT count(*) FROM txs_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), count)
}

// TestChainStore_InsertAccountsStaging tests inserting accounts to staging.
func TestChainStore_InsertAccountsStaging(t *testing.T) {
	chainDB := createChainStore(t, 20003)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	accounts := []*indexermodels.Account{
		generateAccount("addr1", 100, 1000000),
		generateAccount("addr2", 100, 2000000),
		generateAccount("addr3", 100, 3000000),
	}

	err := chainDB.InsertAccountsStaging(ctx, accounts)
	require.NoError(t, err)

	// Verify accounts are in staging
	var count uint64
	query := `SELECT count(*) FROM accounts_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), count)
}

// TestChainStore_InsertEventsStaging tests inserting events to staging.
func TestChainStore_InsertEventsStaging(t *testing.T) {
	chainDB := createChainStore(t, 20004)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	events := []*indexermodels.Event{
		generateEvent(100, 0),
		generateEvent(100, 1),
		generateEvent(100, 2),
	}

	err := chainDB.InsertEventsStaging(ctx, events)
	require.NoError(t, err)

	// Verify events are in staging
	var count uint64
	query := `SELECT count(*) FROM events_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), count)
}

// TestChainStore_InsertBlockSummariesStaging tests inserting block summaries to staging.
func TestChainStore_InsertBlockSummariesStaging(t *testing.T) {
	chainDB := createChainStore(t, 20005)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	blockTime := time.Now().UTC()
	summary := generateBlockSummary(100, blockTime, 10)

	err := chainDB.InsertBlockSummariesStaging(ctx, summary)
	require.NoError(t, err)

	// Verify block summary is in staging
	var count uint64
	query := `SELECT count(*) FROM block_summaries_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

// TestChainStore_InsertPoolsStaging tests inserting pools to staging.
func TestChainStore_InsertPoolsStaging(t *testing.T) {
	chainDB := createChainStore(t, 20006)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pools := []*indexermodels.Pool{
		generatePool(1, 100),
		generatePool(2, 100),
	}

	err := chainDB.InsertPoolsStaging(ctx, pools)
	require.NoError(t, err)

	// Verify pools are in staging
	var count uint64
	query := `SELECT count(*) FROM pools_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), count)
}

// TestChainStore_InsertOrdersStaging tests inserting orders to staging.
func TestChainStore_InsertOrdersStaging(t *testing.T) {
	chainDB := createChainStore(t, 20007)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	orders := []*indexermodels.Order{
		generateOrder("order-1", 100),
		generateOrder("order-2", 100),
	}

	err := chainDB.InsertOrdersStaging(ctx, orders)
	require.NoError(t, err)

	// Verify orders are in staging
	var count uint64
	query := `SELECT count(*) FROM orders_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), count)
}

// TestChainStore_InsertDexPricesStaging tests inserting dex prices to staging.
func TestChainStore_InsertDexPricesStaging(t *testing.T) {
	chainDB := createChainStore(t, 20008)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prices := []*indexermodels.DexPrice{
		generateDexPrice(100),
		generateDexPrice(101),
	}

	err := chainDB.InsertDexPricesStaging(ctx, prices)
	require.NoError(t, err)

	// Verify prices are in staging
	var count uint64
	query := `SELECT count(*) FROM dex_prices_staging`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), count)
}

// TestChainStore_InsertDexOrdersStaging tests inserting dex orders to staging.
func TestChainStore_InsertDexOrdersStaging(t *testing.T) {
	chainDB := createChainStore(t, 20009)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	orders := []*indexermodels.DexOrder{
		generateDexOrder("dex-order-1", 100),
		generateDexOrder("dex-order-2", 100),
	}

	err := chainDB.InsertDexOrdersStaging(ctx, orders)
	require.NoError(t, err)

	// Verify dex orders are in staging
	var count uint64
	query := `SELECT count(*) FROM dex_orders_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), count)
}

// TestChainStore_InsertDexDepositsStaging tests inserting dex deposits to staging.
func TestChainStore_InsertDexDepositsStaging(t *testing.T) {
	chainDB := createChainStore(t, 20010)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deposits := []*indexermodels.DexDeposit{
		generateDexDeposit("deposit-1", 100),
		generateDexDeposit("deposit-2", 100),
	}

	err := chainDB.InsertDexDepositsStaging(ctx, deposits)
	require.NoError(t, err)

	// Verify deposits are in staging
	var count uint64
	query := `SELECT count(*) FROM dex_deposits_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), count)
}

// TestChainStore_InsertDexWithdrawalsStaging tests inserting dex withdrawals to staging.
func TestChainStore_InsertDexWithdrawalsStaging(t *testing.T) {
	chainDB := createChainStore(t, 20011)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	withdrawals := []*indexermodels.DexWithdrawal{
		generateDexWithdrawal("withdrawal-1", 100),
		generateDexWithdrawal("withdrawal-2", 100),
	}

	err := chainDB.InsertDexWithdrawalsStaging(ctx, withdrawals)
	require.NoError(t, err)

	// Verify withdrawals are in staging
	var count uint64
	query := `SELECT count(*) FROM dex_withdrawals_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), count)
}

// TestChainStore_InsertDexPoolPointsByHolderStaging tests inserting dex pool points to staging.
func TestChainStore_InsertDexPoolPointsByHolderStaging(t *testing.T) {
	chainDB := createChainStore(t, 20012)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	holders := []*indexermodels.DexPoolPointsByHolder{
		generateDexPoolPointsByHolder("holder-1", 100),
		generateDexPoolPointsByHolder("holder-2", 100),
	}

	err := chainDB.InsertDexPoolPointsByHolderStaging(ctx, holders)
	require.NoError(t, err)

	// Verify pool points are in staging
	var count uint64
	query := `SELECT count(*) FROM dex_pool_points_by_holder_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), count)
}

// TestChainStore_InsertParamsStaging tests inserting params to staging.
func TestChainStore_InsertParamsStaging(t *testing.T) {
	chainDB := createChainStore(t, 20013)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	params := generateParams(100)
	err := chainDB.InsertParamsStaging(ctx, params)
	require.NoError(t, err)

	// Verify params are in staging
	var count uint64
	query := `SELECT count(*) FROM params_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

// TestChainStore_InsertValidatorsStaging tests inserting validators to staging.
func TestChainStore_InsertValidatorsStaging(t *testing.T) {
	chainDB := createChainStore(t, 20014)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	validators := []*indexermodels.Validator{
		generateValidator("validator-1", 100),
		generateValidator("validator-2", 100),
	}

	err := chainDB.InsertValidatorsStaging(ctx, validators)
	require.NoError(t, err)

	// Verify validators are in staging
	var count uint64
	query := `SELECT count(*) FROM validators_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), count)
}

// TestChainStore_InsertValidatorSigningInfoStaging tests inserting validator signing info to staging.
func TestChainStore_InsertValidatorSigningInfoStaging(t *testing.T) {
	chainDB := createChainStore(t, 20015)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	signingInfos := []*indexermodels.ValidatorSigningInfo{
		generateValidatorSigningInfo("validator-1", 100),
		generateValidatorSigningInfo("validator-2", 100),
	}

	err := chainDB.InsertValidatorSigningInfoStaging(ctx, signingInfos)
	require.NoError(t, err)

	// Verify signing info are in staging
	var count uint64
	query := `SELECT count(*) FROM validator_signing_info_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), count)
}

// TestChainStore_InsertCommitteesStaging tests inserting committees to staging.
func TestChainStore_InsertCommitteesStaging(t *testing.T) {
	chainDB := createChainStore(t, 20016)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	committees := []*indexermodels.Committee{
		generateCommittee(1, 100),
		generateCommittee(2, 100),
	}

	err := chainDB.InsertCommitteesStaging(ctx, committees)
	require.NoError(t, err)

	// Verify committees are in staging
	var count uint64
	query := `SELECT count(*) FROM committees_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), count)
}

// TestChainStore_InsertCommitteeValidatorsStaging tests inserting committee validators to staging.
func TestChainStore_InsertCommitteeValidatorsStaging(t *testing.T) {
	chainDB := createChainStore(t, 20017)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cvs := []*indexermodels.CommitteeValidator{
		generateCommitteeValidator(1, "validator-1", 100),
		generateCommitteeValidator(1, "validator-2", 100),
	}

	err := chainDB.InsertCommitteeValidatorsStaging(ctx, cvs)
	require.NoError(t, err)

	// Verify committee validators are in staging
	var count uint64
	query := `SELECT count(*) FROM committee_validators_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), count)
}

// TestChainStore_InsertPollSnapshotsStaging tests inserting poll snapshots to staging.
func TestChainStore_InsertPollSnapshotsStaging(t *testing.T) {
	chainDB := createChainStore(t, 20018)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	snapshots := []*indexermodels.PollSnapshot{
		generatePollSnapshot("poll-1", "holder-1", 100),
		generatePollSnapshot("poll-1", "holder-2", 100),
	}

	err := chainDB.InsertPollSnapshotsStaging(ctx, snapshots)
	require.NoError(t, err)

	// Verify poll snapshots are in staging
	var count uint64
	query := `SELECT count(*) FROM poll_snapshots_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), count)
}

// TestChainStore_InsertGenesis tests inserting genesis data.
func TestChainStore_InsertGenesis(t *testing.T) {
	chainDB := createChainStore(t, 20019)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	genesisData := `{"chain_id":"test","initial_validators":[]}`
	err := chainDB.InsertGenesis(ctx, 1, genesisData, time.Now().UTC())
	require.NoError(t, err)

	// Verify genesis data exists
	data, err := chainDB.GetGenesisData(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, genesisData, data)
}

// TestChainStore_PromoteEntity tests promoting data from staging to production.
func TestChainStore_PromoteEntity(t *testing.T) {
	chainDB := createChainStore(t, 30001)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert block to staging
	block := generateBlock(100)
	err := chainDB.InsertBlocksStaging(ctx, block)
	require.NoError(t, err)

	// Promote to production
	err = chainDB.PromoteEntity(ctx, entities.Blocks, 100)
	require.NoError(t, err)

	// Verify block is now in production
	retrieved, err := chainDB.GetBlock(ctx, 100)
	require.NoError(t, err)
	assertBlockEqual(t, block, retrieved)
}

// TestChainStore_PromoteEntity_Idempotent tests promotion is idempotent.
func TestChainStore_PromoteEntity_Idempotent(t *testing.T) {
	chainDB := createChainStore(t, 30002)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert and promote block
	block := generateBlock(100)
	err := chainDB.InsertBlocksStaging(ctx, block)
	require.NoError(t, err)

	err = chainDB.PromoteEntity(ctx, entities.Blocks, 100)
	require.NoError(t, err)

	// Promote again - should not error
	err = chainDB.PromoteEntity(ctx, entities.Blocks, 100)
	require.NoError(t, err)

	// Verify block still exists and correct
	retrieved, err := chainDB.GetBlock(ctx, 100)
	require.NoError(t, err)
	assertBlockEqual(t, block, retrieved)
}

// TestChainStore_CleanEntityStaging tests cleaning staging data after promotion.
func TestChainStore_CleanEntityStaging(t *testing.T) {
	chainDB := createChainStore(t, 30003)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert block to staging
	block := generateBlock(100)
	err := chainDB.InsertBlocksStaging(ctx, block)
	require.NoError(t, err)

	// Promote to production
	err = chainDB.PromoteEntity(ctx, entities.Blocks, 100)
	require.NoError(t, err)

	// Clean staging
	err = chainDB.CleanEntityStaging(ctx, entities.Blocks, 100)
	require.NoError(t, err)

	// Verify staging is empty
	var count uint64
	query := `SELECT count(*) FROM blocks_staging WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), count)

	// Verify production data still exists
	retrieved, err := chainDB.GetBlock(ctx, 100)
	require.NoError(t, err)
	assertBlockEqual(t, block, retrieved)
}

// TestChainStore_TwoPhaseCommit_FullWorkflow tests the complete two-phase commit workflow.
func TestChainStore_TwoPhaseCommit_FullWorkflow(t *testing.T) {
	chainDB := createChainStore(t, 30004)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert multiple entity types to staging
	block := generateBlock(100)
	err := chainDB.InsertBlocksStaging(ctx, block)
	require.NoError(t, err)

	txs := []*indexermodels.Transaction{
		generateTransaction(100, 0),
		generateTransaction(100, 1),
	}
	err = chainDB.InsertTransactionsStaging(ctx, txs)
	require.NoError(t, err)

	accounts := []*indexermodels.Account{
		generateAccount("addr1", 100, 1000000),
	}
	err = chainDB.InsertAccountsStaging(ctx, accounts)
	require.NoError(t, err)

	// Promote all entities
	for _, entity := range []entities.Entity{entities.Blocks, entities.Transactions, entities.Accounts} {
		err = chainDB.PromoteEntity(ctx, entity, 100)
		require.NoError(t, err)
	}

	// Clean all staging
	for _, entity := range []entities.Entity{entities.Blocks, entities.Transactions, entities.Accounts} {
		err = chainDB.CleanEntityStaging(ctx, entity, 100)
		require.NoError(t, err)
	}

	// Verify production data exists and staging is clean
	retrievedBlock, err := chainDB.GetBlock(ctx, 100)
	require.NoError(t, err)
	assertBlockEqual(t, block, retrievedBlock)

	var txCount, accountCount, stagingBlockCount, stagingTxCount, stagingAccountCount uint64

	err = chainDB.Db.QueryRow(ctx, `SELECT count(*) FROM txs WHERE height = 100`).Scan(&txCount)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), txCount)

	err = chainDB.Db.QueryRow(ctx, `SELECT count(*) FROM accounts WHERE height = 100`).Scan(&accountCount)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), accountCount)

	err = chainDB.Db.QueryRow(ctx, `SELECT count(*) FROM blocks_staging WHERE height = 100`).Scan(&stagingBlockCount)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), stagingBlockCount)

	err = chainDB.Db.QueryRow(ctx, `SELECT count(*) FROM txs_staging WHERE height = 100`).Scan(&stagingTxCount)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), stagingTxCount)

	err = chainDB.Db.QueryRow(ctx, `SELECT count(*) FROM accounts_staging WHERE height = 100`).Scan(&stagingAccountCount)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), stagingAccountCount)
}

// TestChainStore_GetBlock tests retrieving a block.
func TestChainStore_GetBlock(t *testing.T) {
	chainDB := createChainStore(t, 40001)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	block := generateBlock(100)
	err := chainDB.InsertBlocksStaging(ctx, block)
	require.NoError(t, err)
	err = chainDB.PromoteEntity(ctx, entities.Blocks, 100)
	require.NoError(t, err)

	retrieved, err := chainDB.GetBlock(ctx, 100)
	require.NoError(t, err)
	assertBlockEqual(t, block, retrieved)
}

// TestChainStore_GetBlock_NotFound tests retrieving a non-existent block.
func TestChainStore_GetBlock_NotFound(t *testing.T) {
	chainDB := createChainStore(t, 40002)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := chainDB.GetBlock(ctx, 9999)
	require.Error(t, err)
}

// TestChainStore_HasBlock tests checking block existence.
func TestChainStore_HasBlock(t *testing.T) {
	chainDB := createChainStore(t, 40003)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Block doesn't exist yet
	exists, err := chainDB.HasBlock(ctx, 100)
	require.NoError(t, err)
	assert.False(t, exists)

	// Insert and promote block
	block := generateBlock(100)
	err = chainDB.InsertBlocksStaging(ctx, block)
	require.NoError(t, err)
	err = chainDB.PromoteEntity(ctx, entities.Blocks, 100)
	require.NoError(t, err)

	// Block should exist now
	exists, err = chainDB.HasBlock(ctx, 100)
	require.NoError(t, err)
	assert.True(t, exists)
}

// TestChainStore_GetBlockSummary tests retrieving block summary.
func TestChainStore_GetBlockSummary(t *testing.T) {
	chainDB := createChainStore(t, 40004)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	blockTime := time.Now().UTC()
	summary := generateBlockSummary(100, blockTime, 10)
	err := chainDB.InsertBlockSummariesStaging(ctx, summary)
	require.NoError(t, err)
	err = chainDB.PromoteEntity(ctx, entities.BlockSummaries, 100)
	require.NoError(t, err)

	retrievedSummary, err := chainDB.GetBlockSummary(ctx, 100)
	require.NoError(t, err)
	assert.Equal(t, uint64(100), retrievedSummary.Height)
	assert.Equal(t, uint32(10), retrievedSummary.NumTxs)
}

// TestChainStore_GetGenesisData tests retrieving genesis data.
func TestChainStore_GetGenesisData(t *testing.T) {
	chainDB := createChainStore(t, 40005)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	genesisData := `{"chain_id":"test","initial_validators":["val1","val2"]}`
	err := chainDB.InsertGenesis(ctx, 1, genesisData, time.Now().UTC())
	require.NoError(t, err)

	retrieved, err := chainDB.GetGenesisData(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, genesisData, retrieved)
}

// TestChainStore_HasGenesis tests checking genesis data existence.
func TestChainStore_HasGenesis(t *testing.T) {
	chainDB := createChainStore(t, 40006)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Genesis doesn't exist yet
	exists, err := chainDB.HasGenesis(ctx, 1)
	require.NoError(t, err)
	assert.False(t, exists)

	// Insert genesis
	genesisData := `{"chain_id":"test"}`
	err = chainDB.InsertGenesis(ctx, 1, genesisData, time.Now().UTC())
	require.NoError(t, err)

	// Genesis should exist now
	exists, err = chainDB.HasGenesis(ctx, 1)
	require.NoError(t, err)
	assert.True(t, exists)
}

// TestChainStore_GetEventsByTypeAndHeight tests retrieving events by type and height.
func TestChainStore_GetEventsByTypeAndHeight(t *testing.T) {
	chainDB := createChainStore(t, 40007)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert events of different types
	events := []*indexermodels.Event{
		{Height: 100, ChainID: 1, Address: "addr1", Reference: "tx1", EventType: "transfer", Msg: `{"amount":"100"}`, HeightTime: time.Now().UTC()},
		{Height: 100, ChainID: 1, Address: "addr2", Reference: "tx1", EventType: "stake", Msg: `{"amount":"1000"}`, HeightTime: time.Now().UTC()},
		{Height: 100, ChainID: 1, Address: "addr3", Reference: "tx2", EventType: "transfer", Msg: `{"amount":"200"}`, HeightTime: time.Now().UTC()},
	}

	err := chainDB.InsertEventsStaging(ctx, events)
	require.NoError(t, err)
	err = chainDB.PromoteEntity(ctx, entities.Events, 100)
	require.NoError(t, err)

	// Get only transfer events
	transfers, err := chainDB.GetEventsByTypeAndHeight(ctx, 100, "transfer")
	require.NoError(t, err)
	assert.Len(t, transfers, 2)
	for _, e := range transfers {
		assert.Equal(t, "transfer", e.EventType)
	}

	// Get stake events
	stakes, err := chainDB.GetEventsByTypeAndHeight(ctx, 100, "stake")
	require.NoError(t, err)
	assert.Len(t, stakes, 1)
	assert.Equal(t, "stake", stakes[0].EventType)
}

// TestChainStore_DeleteBlock tests deleting a block.
func TestChainStore_DeleteBlock(t *testing.T) {
	chainDB := createChainStore(t, 40008)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert and promote block
	block := generateBlock(100)
	err := chainDB.InsertBlocksStaging(ctx, block)
	require.NoError(t, err)
	err = chainDB.PromoteEntity(ctx, entities.Blocks, 100)
	require.NoError(t, err)

	// Verify block exists
	exists, err := chainDB.HasBlock(ctx, 100)
	require.NoError(t, err)
	assert.True(t, exists)

	// Delete block
	err = chainDB.DeleteBlock(ctx, 100)
	require.NoError(t, err)

	// Verify block is deleted (may take a moment for ClickHouse to process)
	time.Sleep(100 * time.Millisecond)
	var count uint64
	query := `SELECT count(*) FROM blocks FINAL WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), count)
}

// TestChainStore_DeleteTransactions tests deleting transactions.
func TestChainStore_DeleteTransactions(t *testing.T) {
	chainDB := createChainStore(t, 40009)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert and promote transactions
	txs := []*indexermodels.Transaction{
		generateTransaction(100, 0),
		generateTransaction(100, 1),
	}
	err := chainDB.InsertTransactionsStaging(ctx, txs)
	require.NoError(t, err)
	err = chainDB.PromoteEntity(ctx, entities.Transactions, 100)
	require.NoError(t, err)

	// Delete transactions
	err = chainDB.DeleteTransactions(ctx, 100)
	require.NoError(t, err)

	// Verify transactions are deleted
	time.Sleep(100 * time.Millisecond)
	var count uint64
	query := `SELECT count(*) FROM txs FINAL WHERE height = 100`
	err = chainDB.Db.QueryRow(ctx, query).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), count)
}

// TestChainStore_DescribeTable tests describing table schema.
func TestChainStore_DescribeTable(t *testing.T) {
	chainDB := createChainStore(t, 50001)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	columns, err := chainDB.DescribeTable(ctx, "blocks")
	require.NoError(t, err)
	assert.NotEmpty(t, columns)

	// Verify expected columns exist
	expectedColumns := []string{"height", "hash", "time", "parent_hash", "proposer_address", "size"}
	columnNames := make(map[string]bool)
	for _, col := range columns {
		columnNames[col.Name] = true
	}

	for _, expected := range expectedColumns {
		assert.True(t, columnNames[expected], "column %s should exist", expected)
	}
}

// TestChainStore_GetTableSchema tests retrieving table schema from system.columns.
func TestChainStore_GetTableSchema(t *testing.T) {
	chainDB := createChainStore(t, 50002)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	columns, err := chainDB.GetTableSchema(ctx, "blocks")
	require.NoError(t, err)
	assert.NotEmpty(t, columns)

	// Verify columns have types
	for _, col := range columns {
		assert.NotEmpty(t, col.Name)
		assert.NotEmpty(t, col.Type)
	}
}

// TestChainStore_GetTableDataPaginated tests paginated table data retrieval.
func TestChainStore_GetTableDataPaginated(t *testing.T) {
	chainDB := createChainStore(t, 50003)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert multiple blocks
	for height := uint64(100); height <= 110; height++ {
		block := generateBlock(height)
		err := chainDB.InsertBlocksStaging(ctx, block)
		require.NoError(t, err)
		err = chainDB.PromoteEntity(ctx, entities.Blocks, height)
		require.NoError(t, err)
	}

	// Get paginated data
	data, total, hasMore, err := chainDB.GetTableDataPaginated(ctx, "blocks", 5, 0, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(11), total)
	assert.True(t, hasMore)
	assert.Len(t, data, 5)
}

// TestChainStore_GetTableDataPaginated_WithHeightFilter tests pagination with height filtering.
func TestChainStore_GetTableDataPaginated_WithHeightFilter(t *testing.T) {
	chainDB := createChainStore(t, 50004)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert multiple blocks
	for height := uint64(100); height <= 110; height++ {
		block := generateBlock(height)
		err := chainDB.InsertBlocksStaging(ctx, block)
		require.NoError(t, err)
		err = chainDB.PromoteEntity(ctx, entities.Blocks, height)
		require.NoError(t, err)
	}

	// Get data with height filter
	fromHeight := uint64(105)
	toHeight := uint64(108)
	data, total, hasMore, err := chainDB.GetTableDataPaginated(ctx, "blocks", 10, 0, &fromHeight, &toHeight)
	require.NoError(t, err)
	assert.Equal(t, int64(4), total) // Heights 105, 106, 107, 108
	assert.False(t, hasMore)
	assert.Len(t, data, 4)
}

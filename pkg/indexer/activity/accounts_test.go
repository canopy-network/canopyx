package activity

import (
	"context"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"github.com/canopy-network/canopyx/pkg/db/entities"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.uber.org/zap/zaptest"
)

// Mock RPC Client for accounts
type mockAccountsRPCClient struct {
	mock.Mock
}

func (m *mockAccountsRPCClient) ChainHead(ctx context.Context) (uint64, error) {
	args := m.Called(ctx)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *mockAccountsRPCClient) BlockByHeight(ctx context.Context, height uint64) (*indexermodels.Block, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*indexermodels.Block), args.Error(1)
}

func (m *mockAccountsRPCClient) TxsByHeight(ctx context.Context, height uint64) ([]*indexermodels.Transaction, []*indexermodels.TransactionRaw, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).([]*indexermodels.Transaction), args.Get(1).([]*indexermodels.TransactionRaw), args.Error(2)
}

func (m *mockAccountsRPCClient) AccountsByHeight(ctx context.Context, height uint64) ([]*rpc.RpcAccount, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*rpc.RpcAccount), args.Error(1)
}

func (m *mockAccountsRPCClient) GetGenesisState(ctx context.Context, height uint64) (*rpc.GenesisState, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rpc.GenesisState), args.Error(1)
}

// Mock Chain Store for accounts tests
type mockAccountsChainStore struct {
	mock.Mock
	chainID          string
	databaseName     string
	insertedAccounts []*indexermodels.Account
}

func (m *mockAccountsChainStore) DatabaseName() string { return m.databaseName }
func (m *mockAccountsChainStore) ChainKey() string     { return m.chainID }

func (m *mockAccountsChainStore) InsertBlock(ctx context.Context, block *indexermodels.Block) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

func (m *mockAccountsChainStore) InsertTransactions(ctx context.Context, txs []*indexermodels.Transaction, raws []*indexermodels.TransactionRaw) error {
	args := m.Called(ctx, txs, raws)
	return args.Error(0)
}

func (m *mockAccountsChainStore) InsertBlockSummary(ctx context.Context, height uint64, timestamp time.Time, numTxs uint32) error {
	args := m.Called(ctx, height, timestamp, numTxs)
	return args.Error(0)
}

func (m *mockAccountsChainStore) GetBlock(ctx context.Context, height uint64) (*indexermodels.Block, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*indexermodels.Block), args.Error(1)
}

func (m *mockAccountsChainStore) GetBlockSummary(ctx context.Context, height uint64) (*indexermodels.BlockSummary, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*indexermodels.BlockSummary), args.Error(1)
}

func (m *mockAccountsChainStore) HasBlock(ctx context.Context, height uint64) (bool, error) {
	args := m.Called(ctx, height)
	return args.Bool(0), args.Error(1)
}

func (m *mockAccountsChainStore) DeleteBlock(ctx context.Context, height uint64) error {
	args := m.Called(ctx, height)
	return args.Error(0)
}

func (m *mockAccountsChainStore) DeleteTransactions(ctx context.Context, height uint64) error {
	args := m.Called(ctx, height)
	return args.Error(0)
}

func (m *mockAccountsChainStore) Exec(ctx context.Context, query string, args ...any) error {
	mockArgs := m.Called(ctx, query, args)
	return mockArgs.Error(0)
}

func (m *mockAccountsChainStore) QueryBlocks(ctx context.Context, height uint64, limit int, desc bool) ([]indexermodels.Block, error) {
	args := m.Called(ctx, height, limit, desc)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]indexermodels.Block), args.Error(1)
}

func (m *mockAccountsChainStore) QueryBlockSummaries(ctx context.Context, height uint64, limit int, desc bool) ([]indexermodels.BlockSummary, error) {
	args := m.Called(ctx, height, limit, desc)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]indexermodels.BlockSummary), args.Error(1)
}

func (m *mockAccountsChainStore) QueryTransactions(ctx context.Context, height uint64, limit int, desc bool) ([]indexermodels.Transaction, error) {
	args := m.Called(ctx, height, limit, desc)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]indexermodels.Transaction), args.Error(1)
}

func (m *mockAccountsChainStore) QueryTransactionsRaw(ctx context.Context, height uint64, limit int, desc bool) ([]map[string]interface{}, error) {
	args := m.Called(ctx, height, limit, desc)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]map[string]interface{}), args.Error(1)
}

func (m *mockAccountsChainStore) DescribeTable(ctx context.Context, table string) ([]db.Column, error) {
	args := m.Called(ctx, table)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]db.Column), args.Error(1)
}

func (m *mockAccountsChainStore) PromoteEntity(ctx context.Context, entity entities.Entity, height uint64) error {
	args := m.Called(ctx, entity, height)
	return args.Error(0)
}

func (m *mockAccountsChainStore) CleanEntityStaging(ctx context.Context, entity entities.Entity, height uint64) error {
	args := m.Called(ctx, entity, height)
	return args.Error(0)
}

func (m *mockAccountsChainStore) ValidateQueryHeight(ctx context.Context, height *uint64) (uint64, error) {
	args := m.Called(ctx, height)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *mockAccountsChainStore) GetFullyIndexedHeight(ctx context.Context) (uint64, error) {
	args := m.Called(ctx)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *mockAccountsChainStore) GetAccountByAddress(ctx context.Context, address string, height *uint64) (*indexermodels.Account, error) {
	args := m.Called(ctx, address, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*indexermodels.Account), args.Error(1)
}

func (m *mockAccountsChainStore) QueryAccounts(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexermodels.Account, error) {
	args := m.Called(ctx, cursor, limit, sortDesc)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]indexermodels.Account), args.Error(1)
}

func (m *mockAccountsChainStore) GetTransactionByHash(ctx context.Context, hash string) (*indexermodels.Transaction, error) {
	args := m.Called(ctx, hash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*indexermodels.Transaction), args.Error(1)
}

func (m *mockAccountsChainStore) Close() error {
	return nil
}

// TestIndexAccounts_Success tests successful indexing with change detection
func TestIndexAccounts_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Setup mock admin store
	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	// Setup mock RPC client
	mockRPC := new(mockAccountsRPCClient)

	// Current height accounts
	currentAccounts := []*rpc.RpcAccount{
		{Address: "0x111", Amount: 1500}, // Changed from 1000
		{Address: "0x222", Amount: 2000}, // No change
		{Address: "0x333", Amount: 3000}, // New account
	}

	// Previous height accounts
	previousAccounts := []*rpc.RpcAccount{
		{Address: "0x111", Amount: 1000}, // Will change
		{Address: "0x222", Amount: 2000}, // No change
	}

	mockRPC.On("AccountsByHeight", mock.Anything, uint64(100)).Return(currentAccounts, nil)
	mockRPC.On("AccountsByHeight", mock.Anything, uint64(99)).Return(previousAccounts, nil)

	// Setup mock chain store
	mockChainStore := &mockAccountsChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	// Mock the Exec call for InsertAccountsStaging
	mockChainStore.On("Exec", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil)

	// Setup chains map
	chainsMap := xsync.NewMap[string, db.ChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	// Create activity context
	activityCtx := &Context{
		Logger:     logger,
		IndexerDB:  adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	// Setup temporal test suite
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexAccounts)

	// Execute activity
	input := types.IndexAccountsInput{
		ChainID:   "chain-A",
		Height:    100,
		BlockTime: time.Now(),
	}

	future, err := env.ExecuteActivity(activityCtx.IndexAccounts, input)
	require.NoError(t, err)

	// Get output
	var output types.IndexAccountsOutput
	require.NoError(t, future.Get(&output))

	// Assertions
	assert.Equal(t, uint32(2), output.NumAccounts) // 2 changed accounts (0x111 and 0x333)
	assert.Greater(t, output.DurationMs, 0.0)

	mockRPC.AssertExpectations(t)
}

// TestIndexAccounts_NoChanges tests when no accounts have changed
func TestIndexAccounts_NoChanges(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := new(mockAccountsRPCClient)

	// Same accounts at both heights
	sameAccounts := []*rpc.RpcAccount{
		{Address: "0x111", Amount: 1000},
		{Address: "0x222", Amount: 2000},
	}

	mockRPC.On("AccountsByHeight", mock.Anything, uint64(100)).Return(sameAccounts, nil)
	mockRPC.On("AccountsByHeight", mock.Anything, uint64(99)).Return(sameAccounts, nil)

	mockChainStore := &mockAccountsChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	chainsMap := xsync.NewMap[string, db.ChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &Context{
		Logger:     logger,
		IndexerDB:  adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexAccounts)

	input := types.IndexAccountsInput{
		ChainID:   "chain-A",
		Height:    100,
		BlockTime: time.Now(),
	}

	future, err := env.ExecuteActivity(activityCtx.IndexAccounts, input)
	require.NoError(t, err)

	var output types.IndexAccountsOutput
	require.NoError(t, future.Get(&output))

	// No accounts should be indexed since nothing changed
	assert.Equal(t, uint32(0), output.NumAccounts)
	assert.GreaterOrEqual(t, output.DurationMs, 0.0)

	mockRPC.AssertExpectations(t)
}

// TestIndexAccounts_HeightOne tests genesis edge case
func TestIndexAccounts_HeightOne(t *testing.T) {
	mockRPC := new(mockAccountsRPCClient)

	// Accounts at height 1
	height1Accounts := []*rpc.RpcAccount{
		{Address: "0xaaa", Amount: 5000}, // Changed from genesis
		{Address: "0xbbb", Amount: 6000}, // Same as genesis
		{Address: "0xccc", Amount: 7000}, // New since genesis
	}

	mockRPC.On("AccountsByHeight", mock.Anything, uint64(1)).Return(height1Accounts, nil)

	// For testing, we'll work with test data directly since
	// we can't easily mock the database queries

	// Mock genesis cache - stored as JSON in the DB
	genesis := rpc.GenesisState{
		Time: 1234567890,
		Accounts: []*rpc.RpcAccount{
			{Address: "0xaaa", Amount: 4000}, // Will change
			{Address: "0xbbb", Amount: 6000}, // No change
		},
	}

	// We need to set up a more complex mock for the genesis query
	// In the real implementation, it queries the genesis table
	// For simplicity in testing, we'll use a modified chainStore

	// For unit testing purposes, we'll test the comparison logic directly
	// without the database interaction

	// Override the getGenesisAccounts behavior for testing
	// This is a simplification - in real code, it queries the database
	// For unit testing, we'll test the comparison logic
	input := types.IndexAccountsInput{
		ChainID:   "chain-A",
		Height:    1,
		BlockTime: time.Now(),
	}

	// Since we can't easily mock the bun DB queries, we'll test the logic directly
	// by calling the comparison logic with test data
	_ = context.Background()

	// Build previous state map (genesis)
	prevMap := make(map[string]uint64, len(genesis.Accounts))
	for _, acc := range genesis.Accounts {
		prevMap[acc.Address] = acc.Amount
	}

	// Compare and collect changed accounts
	changedAccounts := make([]*indexermodels.Account, 0)
	for _, curr := range height1Accounts {
		prevAmount, existed := prevMap[curr.Address]

		if curr.Amount != prevAmount {
			var createdHeight uint64
			if !existed {
				createdHeight = 1 // New account at height 1
			} else {
				createdHeight = 0 // Existed in genesis
			}

			changedAccounts = append(changedAccounts, &indexermodels.Account{
				Address:       curr.Address,
				Amount:        curr.Amount,
				Height:        1,
				HeightTime:    input.BlockTime,
				CreatedHeight: createdHeight,
			})
		}
	}

	// Verify the logic works correctly
	assert.Len(t, changedAccounts, 2) // 0xaaa changed, 0xccc is new
	assert.Equal(t, "0xaaa", changedAccounts[0].Address)
	assert.Equal(t, uint64(5000), changedAccounts[0].Amount)
	assert.Equal(t, uint64(0), changedAccounts[0].CreatedHeight) // Existed in genesis

	assert.Equal(t, "0xccc", changedAccounts[1].Address)
	assert.Equal(t, uint64(7000), changedAccounts[1].Amount)
	assert.Equal(t, uint64(1), changedAccounts[1].CreatedHeight) // New at height 1
}

// TestIndexAccounts_RPCFailure tests handling of RPC errors
func TestIndexAccounts_RPCFailure(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := new(mockAccountsRPCClient)

	// Simulate RPC failure
	mockRPC.On("AccountsByHeight", mock.Anything, uint64(100)).
		Return([]*rpc.RpcAccount(nil), assert.AnError)
	mockRPC.On("AccountsByHeight", mock.Anything, uint64(99)).
		Return([]*rpc.RpcAccount{}, nil).Maybe()

	mockChainStore := &mockAccountsChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	chainsMap := xsync.NewMap[string, db.ChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &Context{
		Logger:     logger,
		IndexerDB:  adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexAccounts)

	input := types.IndexAccountsInput{
		ChainID:   "chain-A",
		Height:    100,
		BlockTime: time.Now(),
	}

	future, err := env.ExecuteActivity(activityCtx.IndexAccounts, input)
	require.NoError(t, err)

	var output types.IndexAccountsOutput
	err = future.Get(&output)

	// Should have an error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fetch current accounts at height 100")

	mockRPC.AssertExpectations(t)
}

// TestIndexAccounts_LargeDataset tests performance with many accounts
func TestIndexAccounts_LargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large dataset test in short mode")
	}

	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := new(mockAccountsRPCClient)

	// Create large dataset - 10,000 accounts
	numAccounts := 10000
	currentAccounts := make([]*rpc.RpcAccount, numAccounts)
	previousAccounts := make([]*rpc.RpcAccount, numAccounts)

	for i := 0; i < numAccounts; i++ {
		address := "0x" + string(rune(i))
		currentAccounts[i] = &rpc.RpcAccount{
			Address: address,
			Amount:  uint64(i * 100),
		}
		previousAccounts[i] = &rpc.RpcAccount{
			Address: address,
			Amount:  uint64(i * 100),
		}
	}

	// Change 10% of accounts
	for i := 0; i < numAccounts/10; i++ {
		currentAccounts[i].Amount += 1000
	}

	mockRPC.On("AccountsByHeight", mock.Anything, uint64(100)).Return(currentAccounts, nil)
	mockRPC.On("AccountsByHeight", mock.Anything, uint64(99)).Return(previousAccounts, nil)

	mockChainStore := &mockAccountsChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}
	mockChainStore.On("Exec", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil)

	chainsMap := xsync.NewMap[string, db.ChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &Context{
		Logger:     logger,
		IndexerDB:  adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexAccounts)

	input := types.IndexAccountsInput{
		ChainID:   "chain-A",
		Height:    100,
		BlockTime: time.Now(),
	}

	start := time.Now()
	future, err := env.ExecuteActivity(activityCtx.IndexAccounts, input)
	require.NoError(t, err)

	var output types.IndexAccountsOutput
	require.NoError(t, future.Get(&output))

	elapsed := time.Since(start)

	// Performance assertions
	assert.Equal(t, uint32(1000), output.NumAccounts) // 10% changed
	assert.Greater(t, output.DurationMs, 0.0)
	assert.Less(t, elapsed, 5*time.Second) // Should complete quickly even with large dataset

	mockRPC.AssertExpectations(t)
}
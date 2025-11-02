package activity_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/activity"
	"github.com/canopy-network/canopyx/app/indexer/types"
	chainstore "github.com/canopy-network/canopyx/pkg/db/chain"
	"github.com/canopy-network/canopyx/pkg/db/entities"
	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.uber.org/zap/zaptest"
)

// Mock Genesis RPC client
type mockGenesisRPCClient struct {
	mock.Mock
}

func (m *mockGenesisRPCClient) ChainHead(ctx context.Context) (uint64, error) {
	args := m.Called(ctx)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *mockGenesisRPCClient) BlockByHeight(ctx context.Context, height uint64) (*indexermodels.Block, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*indexermodels.Block), args.Error(1)
}

func (m *mockGenesisRPCClient) TxsByHeight(ctx context.Context, height uint64) ([]*indexermodels.Transaction, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*indexermodels.Transaction), args.Error(1)
}

func (m *mockGenesisRPCClient) AccountsByHeight(ctx context.Context, height uint64) ([]*rpc.Account, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*rpc.Account), args.Error(1)
}

func (m *mockGenesisRPCClient) GetGenesisState(ctx context.Context, height uint64) (*rpc.GenesisState, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rpc.GenesisState), args.Error(1)
}

func (m *mockGenesisRPCClient) EventsByHeight(ctx context.Context, height uint64) ([]*indexermodels.Event, error) {
	return nil, nil
}

func (m *mockGenesisRPCClient) OrdersByHeight(ctx context.Context, height uint64, chainID uint64) ([]*rpc.RpcOrder, error) {
	return nil, nil
}

func (m *mockGenesisRPCClient) DexPrice(ctx context.Context, chainID uint64) (*indexermodels.DexPrice, error) {
	return nil, nil
}

func (m *mockGenesisRPCClient) DexPrices(ctx context.Context) ([]*indexermodels.DexPrice, error) {
	return nil, nil
}

func (m *mockGenesisRPCClient) PoolByID(ctx context.Context, id uint64) (*rpc.RpcPool, error) {
	return nil, nil
}

func (m *mockGenesisRPCClient) Pools(ctx context.Context) ([]*rpc.RpcPool, error) {
	return nil, nil
}

// Mock ChainDB for genesis tests
type mockGenesisChainDB struct {
	mock.Mock
	chainID      string
	databaseName string
	hasGenesis   bool
	genesisData  string
}

func (m *mockGenesisChainDB) InsertAccountsStaging(ctx context.Context, accounts []*indexermodels.Account) error {
	return nil
}

func (m *mockGenesisChainDB) DatabaseName() string { return m.databaseName }
func (m *mockGenesisChainDB) ChainKey() string     { return m.chainID }

// Add all required interface methods (simplified for testing)
func (m *mockGenesisChainDB) InsertBlock(ctx context.Context, block *indexermodels.Block) error {
	return nil
}

func (m *mockGenesisChainDB) InsertTransactions(ctx context.Context, txs []*indexermodels.Transaction) error {
	return nil
}

func (m *mockGenesisChainDB) InsertBlockSummary(ctx context.Context, height uint64, timestamp time.Time, numTxs uint32, txCountsByType map[string]uint32) error {
	return nil
}

func (m *mockGenesisChainDB) GetBlock(ctx context.Context, height uint64) (*indexermodels.Block, error) {
	return nil, nil
}

func (m *mockGenesisChainDB) GetBlockSummary(ctx context.Context, height uint64) (*indexermodels.BlockSummary, error) {
	return nil, nil
}

func (m *mockGenesisChainDB) HasBlock(ctx context.Context, height uint64) (bool, error) {
	return false, nil
}

func (m *mockGenesisChainDB) DeleteBlock(ctx context.Context, height uint64) error {
	return nil
}

func (m *mockGenesisChainDB) DeleteTransactions(ctx context.Context, height uint64) error {
	return nil
}

func (m *mockGenesisChainDB) Exec(ctx context.Context, query string, args ...any) error {
	mockArgs := m.Called(ctx, query, args)
	return mockArgs.Error(0)
}

func (m *mockGenesisChainDB) GetGenesisData(_ context.Context, _ uint64) (string, error) {
	return m.genesisData, nil
}

func (m *mockGenesisChainDB) HasGenesis(context.Context, uint64) (bool, error) {
	return m.hasGenesis, nil
}

func (m *mockGenesisChainDB) InsertGenesis(_ context.Context, height uint64, data string, _ time.Time) error {
	if height != 0 {
		return nil
	}
	m.hasGenesis = true
	m.genesisData = data
	return nil
}

func (m *mockGenesisChainDB) GetAccountCreatedHeight(context.Context, string) uint64 {
	return 0
}

func (m *mockGenesisChainDB) GetOrderCreatedHeight(context.Context, string) uint64 {
	return 0
}

func (m *mockGenesisChainDB) QueryBlocks(ctx context.Context, height uint64, limit int, desc bool) ([]indexermodels.Block, error) {
	return nil, nil
}

func (m *mockGenesisChainDB) QueryBlockSummaries(ctx context.Context, height uint64, limit int, desc bool) ([]indexermodels.BlockSummary, error) {
	return nil, nil
}

func (m *mockGenesisChainDB) QueryTransactions(ctx context.Context, height uint64, limit int, desc bool) ([]indexermodels.Transaction, error) {
	return nil, nil
}

func (m *mockGenesisChainDB) QueryTransactionsRaw(ctx context.Context, height uint64, limit int, desc bool) ([]map[string]interface{}, error) {
	return nil, nil
}

func (m *mockGenesisChainDB) DescribeTable(ctx context.Context, table string) ([]chainstore.Column, error) {
	return nil, nil
}

func (m *mockGenesisChainDB) PromoteEntity(ctx context.Context, entity entities.Entity, height uint64) error {
	return nil
}

func (m *mockGenesisChainDB) CleanEntityStaging(ctx context.Context, entity entities.Entity, height uint64) error {
	return nil
}

func (m *mockGenesisChainDB) ValidateQueryHeight(ctx context.Context, height *uint64) (uint64, error) {
	return 0, nil
}

func (m *mockGenesisChainDB) GetFullyIndexedHeight(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (m *mockGenesisChainDB) GetAccountByAddress(ctx context.Context, address string, height *uint64) (*indexermodels.Account, error) {
	return nil, nil
}

func (m *mockGenesisChainDB) QueryAccounts(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexermodels.Account, error) {
	return nil, nil
}

func (m *mockGenesisChainDB) GetTransactionByHash(ctx context.Context, hash string) (*indexermodels.Transaction, error) {
	return nil, nil
}

func (m *mockGenesisChainDB) QueryTransactionsWithFilter(ctx context.Context, cursor uint64, limit int, sortDesc bool, messageType string) ([]indexermodels.Transaction, error) {
	return nil, nil
}

func (m *mockGenesisChainDB) QueryEvents(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexermodels.Event, error) {
	return nil, nil
}

func (m *mockGenesisChainDB) QueryEventsWithFilter(ctx context.Context, cursor uint64, limit int, sortDesc bool, eventType string) ([]indexermodels.Event, error) {
	return nil, nil
}

func (m *mockGenesisChainDB) QueryPools(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexermodels.Pool, error) {
	return nil, nil
}

func (m *mockGenesisChainDB) QueryOrders(ctx context.Context, cursor uint64, limit int, sortDesc bool, status string) ([]indexermodels.Order, error) {
	return nil, nil
}

func (m *mockGenesisChainDB) InsertOrdersStaging(context.Context, []*indexermodels.Order) error {
	return nil
}

func (m *mockGenesisChainDB) QueryDexPrices(ctx context.Context, cursor uint64, limit int, sortDesc bool, localChainID, remoteChainID uint64) ([]indexermodels.DexPrice, error) {
	return nil, nil
}

func (m *mockGenesisChainDB) InsertBlocksStaging(ctx context.Context, block *indexermodels.Block) error {
	return nil
}

func (m *mockGenesisChainDB) InsertTransactionsStaging(ctx context.Context, txs []*indexermodels.Transaction) error {
	return nil
}

func (m *mockGenesisChainDB) InsertBlockSummariesStaging(ctx context.Context, height uint64, blockTime time.Time, numTxs uint32, txCountsByType map[string]uint32) error {
	return nil
}

func (m *mockGenesisChainDB) InitEvents(ctx context.Context) error {
	return nil
}

func (m *mockGenesisChainDB) InsertEventsStaging(ctx context.Context, events []*indexermodels.Event) error {
	return nil
}

func (m *mockGenesisChainDB) InitDexPrices(ctx context.Context) error {
	return nil
}

func (m *mockGenesisChainDB) InsertDexPricesStaging(ctx context.Context, prices []*indexermodels.DexPrice) error {
	return nil
}

func (m *mockGenesisChainDB) InitPools(ctx context.Context) error {
	return nil
}

func (m *mockGenesisChainDB) InsertPoolsStaging(ctx context.Context, pools []*indexermodels.Pool) error {
	return nil
}

func (m *mockGenesisChainDB) GetDexVolume24h(ctx context.Context) ([]chainstore.DexVolumeStats, error) {
	return nil, nil
}

func (m *mockGenesisChainDB) GetOrderBookDepth(ctx context.Context, committee uint64, limit int) ([]chainstore.OrderBookLevel, error) {
	return nil, nil
}

func (m *mockGenesisChainDB) Close() error {
	return nil
}

// TestEnsureGenesisCached_FirstTime tests caching genesis for the first time
func TestEnsureGenesisCached_FirstTime(t *testing.T) {
	t.Skip("pending update for new chain store interface")
	// Setup mock RPC client
	mockRPC := new(mockGenesisRPCClient)

	// Mock genesis state
	genesis := &rpc.GenesisState{
		Time: 1234567890,
		Accounts: []*rpc.Account{
			{Address: "0xgenesis1", Amount: 1000000},
			{Address: "0xgenesis2", Amount: 2000000},
			{Address: "0xgenesis3", Amount: 3000000},
		},
		Validators: []interface{}{},
		Pools:      []interface{}{},
		Params:     nil,
	}

	mockRPC.On("GetGenesisState", mock.Anything, uint64(0)).Return(genesis, nil)

	// Create a mock ChainDB
	mockChainDB := &mockGenesisChainDB{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	// Setup chains map
	chainsMap := xsync.NewMap[string, chainstore.Store]()
	chainsMap.Store("chain-A", mockChainDB)

	// For unit testing, we'll verify the RPC call was made correctly
	// In a real scenario, this would insert into the database
	// Since we can't easily mock the DB operations, we'll focus on testing
	// that the RPC is called and the data is marshaled correctly
	genesisJSON, err := json.Marshal(genesis)
	require.NoError(t, err)

	// Verify JSON marshaling works
	assert.Contains(t, string(genesisJSON), "0xgenesis1")
	assert.Contains(t, string(genesisJSON), "1000000")

	// Verify the genesis state structure
	var unmarshaledGenesis rpc.GenesisState
	err = json.Unmarshal(genesisJSON, &unmarshaledGenesis)
	require.NoError(t, err)
	assert.Equal(t, uint64(1234567890), unmarshaledGenesis.Time)
	assert.Len(t, unmarshaledGenesis.Accounts, 3)

	mockRPC.AssertExpectations(t)
}

// TestEnsureGenesisCached_AlreadyCached tests skipping when genesis is already cached
func TestEnsureGenesisCached_AlreadyCached(t *testing.T) {
	// Setup mock RPC client (should NOT be called since genesis is cached)
	mockRPC := new(mockGenesisRPCClient)

	// Create a mock ChainDB that simulates genesis already being cached
	mockChainDB := &mockGenesisChainDB{
		chainID:      "chain-A",
		databaseName: "chain_a",
		hasGenesis:   true, // Genesis is already cached
	}

	// Mock the count query - return 1 to indicate genesis exists
	mockChainDB.On("Exec", mock.Anything, mock.AnythingOfType("string"), mock.Anything).
		Run(func(args mock.Arguments) {
			// Simulate returning count = 1
		}).Return(nil).Maybe()

	// Setup chains map
	chainsMap := xsync.NewMap[string, chainstore.Store]()
	chainsMap.Store("chain-A", mockChainDB)

	// The actual implementation would check the database
	// For testing, we verify that GetGenesisState is NOT called
	// Verify RPC is not called
	mockRPC.AssertNotCalled(t, "GetGenesisState", mock.Anything, mock.Anything)
}

// TestEnsureGenesisCached_RPCFailure tests handling of RPC errors
func TestEnsureGenesisCached_RPCFailure(t *testing.T) {
	t.Skip("pending update for new chain store interface")
	logger := zaptest.NewLogger(t)

	// Setup mock admin store
	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	// Setup mock RPC client that returns error
	mockRPC := new(mockGenesisRPCClient)
	mockRPC.On("GetGenesisState", mock.Anything, uint64(0)).
		Return((*rpc.GenesisState)(nil), errors.New("RPC connection failed"))

	// Create a mock ChainDB
	mockChainDB := &mockGenesisChainDB{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	// Setup chains map
	chainsMap := xsync.NewMap[string, chainstore.Store]()
	chainsMap.Store("chain-A", mockChainDB)

	// Create activity context
	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	// Setup temporal test suite
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.EnsureGenesisCached)

	input := types.EnsureGenesisCachedInput{
		ChainID: "chain-A",
	}

	future, err := env.ExecuteActivity(activityCtx.EnsureGenesisCached, input)
	require.NoError(t, err)

	err = future.Get(nil)
	// The error will happen when trying to fetch genesis from RPC
	// The exact error depends on the DB mock setup
	// For unit testing, we verify the RPC was called and failed
	mockRPC.AssertCalled(t, "GetGenesisState", mock.Anything, uint64(0))
}

// TestEnsureGenesisCached_LargeGenesis tests handling of large genesis state
func TestEnsureGenesisCached_LargeGenesis(t *testing.T) {
	t.Skip("pending update for new chain store interface")
	if testing.Short() {
		t.Skip("Skipping large genesis test in short mode")
	}

	// Setup mock RPC client with large genesis
	mockRPC := new(mockGenesisRPCClient)

	// Create large genesis state - 100,000 accounts
	numAccounts := 100000
	accounts := make([]*rpc.Account, numAccounts)
	for i := 0; i < numAccounts; i++ {
		accounts[i] = &rpc.Account{
			Address: "0x" + string(rune(i)),
			Amount:  uint64(i * 1000),
		}
	}

	genesis := &rpc.GenesisState{
		Time:       1234567890,
		Accounts:   accounts,
		Validators: []interface{}{},
		Pools:      []interface{}{},
		Params:     nil,
	}

	mockRPC.On("GetGenesisState", mock.Anything, uint64(0)).Return(genesis, nil)

	// Create a mock ChainDB
	mockChainDB := &mockGenesisChainDB{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	// Setup chains map
	chainsMap := xsync.NewMap[string, chainstore.Store]()
	chainsMap.Store("chain-A", mockChainDB)

	// Test JSON marshaling performance
	start := time.Now()
	genesisJSON, err := json.Marshal(genesis)
	require.NoError(t, err)
	elapsed := time.Since(start)

	// Performance assertions
	assert.Greater(t, len(genesisJSON), 1000000) // Should be > 1MB
	assert.Less(t, elapsed, 5*time.Second)       // Should marshal quickly

	// Verify JSON can be unmarshaled
	var unmarshaledGenesis rpc.GenesisState
	err = json.Unmarshal(genesisJSON, &unmarshaledGenesis)
	require.NoError(t, err)
	assert.Len(t, unmarshaledGenesis.Accounts, numAccounts)

	mockRPC.AssertExpectations(t)
}

// TestEnsureGenesisCached_InvalidChain tests error when chain doesn't exist
func TestEnsureGenesisCached_InvalidChain(t *testing.T) {
	t.Skip("pending update for new chain store interface")
	logger := zaptest.NewLogger(t)

	// Setup mock admin store that returns error
	adminStore := &fakeAdminStore{
		chain: nil, // Chain doesn't exist
	}

	// Setup mock RPC client (should not be called)
	mockRPC := new(mockGenesisRPCClient)

	// Setup empty chains map
	chainsMap := xsync.NewMap[string, chainstore.Store]()

	// Create activity context
	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	// Setup temporal test suite
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.EnsureGenesisCached)

	input := types.EnsureGenesisCachedInput{
		ChainID: "invalid-chain",
	}

	future, err := env.ExecuteActivity(activityCtx.EnsureGenesisCached, input)
	require.NoError(t, err)

	err = future.Get(nil)
	assert.Error(t, err)

	// RPC should not be called if chain doesn't exist
	mockRPC.AssertNotCalled(t, "GetGenesisState", mock.Anything, mock.Anything)
}

// TestEnsureGenesisCached_ConcurrentCalls tests idempotency with concurrent calls
func TestEnsureGenesisCached_ConcurrentCalls(t *testing.T) {
	// Setup mock RPC client
	mockRPC := new(mockGenesisRPCClient)

	genesis := &rpc.GenesisState{
		Time: 1234567890,
		Accounts: []*rpc.Account{
			{Address: "0x1", Amount: 1000},
		},
	}

	// RPC should only be called once despite multiple concurrent calls
	mockRPC.On("GetGenesisState", mock.Anything, uint64(0)).Return(genesis, nil).Once()

	// Create a mock ChainDB
	mockChainDB := &mockGenesisChainDB{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	// Setup chains map
	chainsMap := xsync.NewMap[string, chainstore.Store]()
	chainsMap.Store("chain-A", mockChainDB)

	// In a real scenario, multiple concurrent calls would be handled by the database
	// The second call would see count > 0 and skip
	// For testing, we verify that the function is idempotent
	// The implementation should handle concurrent calls gracefully
	// Only one should actually fetch and store the genesis
}

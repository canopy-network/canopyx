package activity_test

import (
	"context"
	"github.com/canopy-network/canopyx/pkg/indexer/activity"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/db/entities"
	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.uber.org/zap/zaptest"
)

// TestIndexTransactions_MixedTypes tests indexing a block with multiple transaction types
func TestIndexTransactions_MixedTypes(t *testing.T) {
	logger := zaptest.NewLogger(t)
	blockTime := time.Now().UTC()

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-mixed",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	chainStore := &testChainStore{chainID: "chain-mixed", databaseName: "chain_mixed"}
	chainsMap := xsync.NewMap[string, db.ChainStore]()
	chainsMap.Store("chain-mixed", chainStore)

	// Create transactions of different types
	txs := []*indexermodels.Transaction{
		{TxHash: "tx1", MessageType: "send", Height: 100, HeightTime: blockTime},
		{TxHash: "tx2", MessageType: "send", Height: 100, HeightTime: blockTime},
		{TxHash: "tx3", MessageType: "delegate", Height: 100, HeightTime: blockTime},
		{TxHash: "tx4", MessageType: "vote", Height: 100, HeightTime: blockTime},
		{TxHash: "tx5", MessageType: "send", Height: 100, HeightTime: blockTime},
		{TxHash: "tx6", MessageType: "contract", Height: 100, HeightTime: blockTime},
	}

	rpcClient := &fakeRPCClient{
		block: &indexermodels.Block{Height: 100},
		txs:   txs,
	}

	activityCtx := &activity.Context{
		Logger:     logger,
		IndexerDB:  adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &fakeRPCFactory{client: rpcClient},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexTransactions)

	input := types.IndexTransactionsInput{
		ChainID:   "chain-mixed",
		Height:    100,
		BlockTime: blockTime,
	}

	future, err := env.ExecuteActivity(activityCtx.IndexTransactions, input)
	require.NoError(t, err)

	var output types.IndexTransactionsOutput
	require.NoError(t, future.Get(&output))

	// Verify total count
	assert.Equal(t, uint32(6), output.NumTxs)
	assert.GreaterOrEqual(t, output.DurationMs, 0.0)

	// Verify counts by type
	require.NotNil(t, output.TxCountsByType)
	assert.Equal(t, uint32(3), output.TxCountsByType["send"])
	assert.Equal(t, uint32(1), output.TxCountsByType["delegate"])
	assert.Equal(t, uint32(1), output.TxCountsByType["vote"])
	assert.Equal(t, uint32(1), output.TxCountsByType["contract"])

	// Verify transactions were inserted
	assert.Equal(t, 1, chainStore.insertTransactionCalls)
	require.Len(t, chainStore.lastTxs, 6)

	// Verify HeightTime was set on all transactions
	for _, tx := range chainStore.lastTxs {
		assert.Equal(t, blockTime, tx.HeightTime)
	}
}

// TestIndexTransactions_CountsByType verifies transaction type counting accuracy
func TestIndexTransactions_CountsByType(t *testing.T) {
	tests := []struct {
		name              string
		txs               []*indexermodels.Transaction
		expectedNumTxs    uint32
		expectedCountsLen int
		expectedCounts    map[string]uint32
	}{
		{
			name: "all send transactions",
			txs: []*indexermodels.Transaction{
				{TxHash: "tx1", MessageType: "send"},
				{TxHash: "tx2", MessageType: "send"},
				{TxHash: "tx3", MessageType: "send"},
			},
			expectedNumTxs:    3,
			expectedCountsLen: 1,
			expectedCounts: map[string]uint32{
				"send": 3,
			},
		},
		{
			name: "mixed transaction types",
			txs: []*indexermodels.Transaction{
				{TxHash: "tx1", MessageType: "send"},
				{TxHash: "tx2", MessageType: "delegate"},
				{TxHash: "tx3", MessageType: "undelegate"},
				{TxHash: "tx4", MessageType: "stake"},
				{TxHash: "tx5", MessageType: "unstake"},
			},
			expectedNumTxs:    5,
			expectedCountsLen: 5,
			expectedCounts: map[string]uint32{
				"send":       1,
				"delegate":   1,
				"undelegate": 1,
				"stake":      1,
				"unstake":    1,
			},
		},
		{
			name: "all 11 message types",
			txs: []*indexermodels.Transaction{
				{TxHash: "tx1", MessageType: "send"},
				{TxHash: "tx2", MessageType: "delegate"},
				{TxHash: "tx3", MessageType: "undelegate"},
				{TxHash: "tx4", MessageType: "stake"},
				{TxHash: "tx5", MessageType: "unstake"},
				{TxHash: "tx6", MessageType: "edit_stake"},
				{TxHash: "tx7", MessageType: "vote"},
				{TxHash: "tx8", MessageType: "proposal"},
				{TxHash: "tx9", MessageType: "contract"},
				{TxHash: "tx10", MessageType: "system"},
				{TxHash: "tx11", MessageType: "unknown"},
			},
			expectedNumTxs:    11,
			expectedCountsLen: 11,
			expectedCounts: map[string]uint32{
				"send":       1,
				"delegate":   1,
				"undelegate": 1,
				"stake":      1,
				"unstake":    1,
				"edit_stake": 1,
				"vote":       1,
				"proposal":   1,
				"contract":   1,
				"system":     1,
				"unknown":    1,
			},
		},
		{
			name: "duplicate types counted correctly",
			txs: []*indexermodels.Transaction{
				{TxHash: "tx1", MessageType: "vote"},
				{TxHash: "tx2", MessageType: "vote"},
				{TxHash: "tx3", MessageType: "vote"},
				{TxHash: "tx4", MessageType: "proposal"},
				{TxHash: "tx5", MessageType: "proposal"},
			},
			expectedNumTxs:    5,
			expectedCountsLen: 2,
			expectedCounts: map[string]uint32{
				"vote":     3,
				"proposal": 2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			blockTime := time.Now().UTC()

			adminStore := &fakeAdminStore{
				chain: &admin.Chain{
					ChainID:      "chain-test",
					RPCEndpoints: []string{"http://rpc.local"},
				},
			}

			chainStore := &testChainStore{chainID: "chain-test", databaseName: "chain_test"}
			chainsMap := xsync.NewMap[string, db.ChainStore]()
			chainsMap.Store("chain-test", chainStore)

			rpcClient := &fakeRPCClient{
				block: &indexermodels.Block{Height: 200},
				txs:   tt.txs,
			}

			activityCtx := &activity.Context{
				Logger:     logger,
				IndexerDB:  adminStore,
				ChainsDB:   chainsMap,
				RPCFactory: &fakeRPCFactory{client: rpcClient},
			}

			input := types.IndexTransactionsInput{
				ChainID:   "chain-test",
				Height:    200,
				BlockTime: blockTime,
			}

			output, err := activityCtx.IndexTransactions(context.Background(), input)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedNumTxs, output.NumTxs)
			assert.Len(t, output.TxCountsByType, tt.expectedCountsLen)

			for msgType, expectedCount := range tt.expectedCounts {
				actualCount, exists := output.TxCountsByType[msgType]
				assert.True(t, exists, "expected message type %s not found in counts", msgType)
				assert.Equal(t, expectedCount, actualCount, "incorrect count for message type %s", msgType)
			}
		})
	}
}

// TestIndexTransactions_EmptyBlock tests indexing a block with zero transactions
func TestIndexTransactions_EmptyBlock(t *testing.T) {
	logger := zaptest.NewLogger(t)
	blockTime := time.Now().UTC()

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-empty",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	chainStore := &testChainStore{chainID: "chain-empty", databaseName: "chain_empty"}
	chainsMap := xsync.NewMap[string, db.ChainStore]()
	chainsMap.Store("chain-empty", chainStore)

	// Empty transaction list
	rpcClient := &fakeRPCClient{
		block: &indexermodels.Block{Height: 300},
		txs:   []*indexermodels.Transaction{},
	}

	activityCtx := &activity.Context{
		Logger:     logger,
		IndexerDB:  adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &fakeRPCFactory{client: rpcClient},
	}

	input := types.IndexTransactionsInput{
		ChainID:   "chain-empty",
		Height:    300,
		BlockTime: blockTime,
	}

	output, err := activityCtx.IndexTransactions(context.Background(), input)
	require.NoError(t, err)

	assert.Equal(t, uint32(0), output.NumTxs)
	assert.Empty(t, output.TxCountsByType)
	assert.GreaterOrEqual(t, output.DurationMs, 0.0)

	// InsertTransactions should still be called (with empty slice)
	assert.Equal(t, 1, chainStore.insertTransactionCalls)
	assert.Empty(t, chainStore.lastTxs)
}

// TestIndexTransactions_SingleType tests a block with only one transaction type
func TestIndexTransactions_SingleType(t *testing.T) {
	logger := zaptest.NewLogger(t)
	blockTime := time.Now().UTC()

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-single",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	chainStore := &testChainStore{chainID: "chain-single", databaseName: "chain_single"}
	chainsMap := xsync.NewMap[string, db.ChainStore]()
	chainsMap.Store("chain-single", chainStore)

	// Only contract transactions
	txs := []*indexermodels.Transaction{
		{TxHash: "contract1", MessageType: "contract", Height: 400},
		{TxHash: "contract2", MessageType: "contract", Height: 400},
		{TxHash: "contract3", MessageType: "contract", Height: 400},
		{TxHash: "contract4", MessageType: "contract", Height: 400},
	}

	rpcClient := &fakeRPCClient{
		block: &indexermodels.Block{Height: 400},
		txs:   txs,
	}

	activityCtx := &activity.Context{
		Logger:     logger,
		IndexerDB:  adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &fakeRPCFactory{client: rpcClient},
	}

	input := types.IndexTransactionsInput{
		ChainID:   "chain-single",
		Height:    400,
		BlockTime: blockTime,
	}

	output, err := activityCtx.IndexTransactions(context.Background(), input)
	require.NoError(t, err)

	assert.Equal(t, uint32(4), output.NumTxs)
	assert.Len(t, output.TxCountsByType, 1)
	assert.Equal(t, uint32(4), output.TxCountsByType["contract"])
}

// TestIndexTransactions_AllTypes tests a block with all 11 message types
func TestIndexTransactions_AllTypes(t *testing.T) {
	logger := zaptest.NewLogger(t)
	blockTime := time.Now().UTC()

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-all-types",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	chainStore := &testChainStore{chainID: "chain-all-types", databaseName: "chain_all_types"}
	chainsMap := xsync.NewMap[string, db.ChainStore]()
	chainsMap.Store("chain-all-types", chainStore)

	// Create one transaction of each type (11 total)
	txs := []*indexermodels.Transaction{
		{TxHash: "send1", MessageType: "send", Height: 500},
		{TxHash: "delegate1", MessageType: "delegate", Height: 500},
		{TxHash: "undelegate1", MessageType: "undelegate", Height: 500},
		{TxHash: "stake1", MessageType: "stake", Height: 500},
		{TxHash: "unstake1", MessageType: "unstake", Height: 500},
		{TxHash: "edit_stake1", MessageType: "edit_stake", Height: 500},
		{TxHash: "vote1", MessageType: "vote", Height: 500},
		{TxHash: "proposal1", MessageType: "proposal", Height: 500},
		{TxHash: "contract1", MessageType: "contract", Height: 500},
		{TxHash: "system1", MessageType: "system", Height: 500},
		{TxHash: "unknown1", MessageType: "unknown", Height: 500},
	}

	rpcClient := &fakeRPCClient{
		block: &indexermodels.Block{Height: 500},
		txs:   txs,
	}

	activityCtx := &activity.Context{
		Logger:     logger,
		IndexerDB:  adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &fakeRPCFactory{client: rpcClient},
	}

	input := types.IndexTransactionsInput{
		ChainID:   "chain-all-types",
		Height:    500,
		BlockTime: blockTime,
	}

	output, err := activityCtx.IndexTransactions(context.Background(), input)
	require.NoError(t, err)

	assert.Equal(t, uint32(11), output.NumTxs)
	assert.Len(t, output.TxCountsByType, 11, "expected all 11 message types")

	// Verify each type has exactly 1 transaction
	expectedTypes := []string{
		"send", "delegate", "undelegate", "stake", "unstake",
		"edit_stake", "vote", "proposal", "contract", "system", "unknown",
	}

	for _, msgType := range expectedTypes {
		count, exists := output.TxCountsByType[msgType]
		assert.True(t, exists, "message type %s not found in counts", msgType)
		assert.Equal(t, uint32(1), count, "expected exactly 1 transaction of type %s", msgType)
	}

	// Verify all transactions were inserted
	assert.Equal(t, 1, chainStore.insertTransactionCalls)
	require.Len(t, chainStore.lastTxs, 11)
}

// TestIndexTransactions_HeightTimePopulation verifies HeightTime is set correctly
func TestIndexTransactions_HeightTimePopulation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	blockTime := time.Date(2024, 1, 15, 12, 30, 45, 0, time.UTC)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-time",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	chainStore := &testChainStore{chainID: "chain-time", databaseName: "chain_time"}
	chainsMap := xsync.NewMap[string, db.ChainStore]()
	chainsMap.Store("chain-time", chainStore)

	txs := []*indexermodels.Transaction{
		{TxHash: "tx1", MessageType: "send", Height: 600},
		{TxHash: "tx2", MessageType: "vote", Height: 600},
	}

	rpcClient := &fakeRPCClient{
		block: &indexermodels.Block{Height: 600},
		txs:   txs,
	}

	activityCtx := &activity.Context{
		Logger:     logger,
		IndexerDB:  adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &fakeRPCFactory{client: rpcClient},
	}

	input := types.IndexTransactionsInput{
		ChainID:   "chain-time",
		Height:    600,
		BlockTime: blockTime,
	}

	output, err := activityCtx.IndexTransactions(context.Background(), input)
	require.NoError(t, err)

	assert.Equal(t, uint32(2), output.NumTxs)

	// Verify all transactions have the correct HeightTime
	for i, tx := range chainStore.lastTxs {
		assert.Equal(t, blockTime, tx.HeightTime,
			"transaction %d has incorrect HeightTime", i)
	}
}

// testChainStore is a test implementation of db.ChainStore that tracks transaction counts
type testChainStore struct {
	chainID                 string
	databaseName            string
	insertBlockCalls        int
	insertTransactionCalls  int
	insertBlockSummaryCalls int
	lastBlock               *indexermodels.Block
	lastBlockSummary        *indexermodels.BlockSummary
	lastTxs                 []*indexermodels.Transaction
	lastTxCountsByType      map[string]uint32
	hasBlock                bool
	deletedBlocks           []uint64
	deletedTransactions     []uint64
	insertedAccounts        []*indexermodels.Account
	accountCreatedHeights   map[string]uint64
	genesisJSON             string
}

func (f *testChainStore) DatabaseName() string { return f.databaseName }
func (f *testChainStore) ChainKey() string     { return f.chainID }

func (f *testChainStore) InsertBlock(_ context.Context, block *indexermodels.Block) error {
	f.insertBlockCalls++
	f.lastBlock = block
	return nil
}

func (f *testChainStore) InsertTransactions(_ context.Context, txs []*indexermodels.Transaction) error {
	f.insertTransactionCalls++
	f.lastTxs = txs
	return nil
}

func (f *testChainStore) InsertBlockSummary(_ context.Context, height uint64, blockTime time.Time, numTxs uint32, txCountsByType map[string]uint32) error {
	f.insertBlockSummaryCalls++
	f.lastBlockSummary = &indexermodels.BlockSummary{
		Height:         height,
		HeightTime:     blockTime,
		NumTxs:         numTxs,
		TxCountsByType: txCountsByType,
	}
	f.lastTxCountsByType = txCountsByType
	return nil
}

func (f *testChainStore) GetBlock(_ context.Context, height uint64) (*indexermodels.Block, error) {
	if f.lastBlock != nil && f.lastBlock.Height == height {
		return f.lastBlock, nil
	}
	return nil, nil
}

func (f *testChainStore) GetBlockSummary(_ context.Context, height uint64) (*indexermodels.BlockSummary, error) {
	if f.lastBlockSummary != nil && f.lastBlockSummary.Height == height {
		return f.lastBlockSummary, nil
	}
	return nil, nil
}

func (f *testChainStore) HasBlock(_ context.Context, _ uint64) (bool, error) {
	return f.hasBlock, nil
}

func (f *testChainStore) DeleteBlock(_ context.Context, height uint64) error {
	f.deletedBlocks = append(f.deletedBlocks, height)
	f.hasBlock = false
	return nil
}

func (f *testChainStore) DeleteTransactions(_ context.Context, height uint64) error {
	f.deletedTransactions = append(f.deletedTransactions, height)
	return nil
}

func (f *testChainStore) Exec(_ context.Context, query string, args ...any) error {
	return nil
}

func (f *testChainStore) InsertAccountsStaging(_ context.Context, accounts []*indexermodels.Account) error {
	f.insertedAccounts = append(f.insertedAccounts, accounts...)
	return nil
}

func (f *testChainStore) GetGenesisData(_ context.Context, _ uint64) (string, error) {
	return f.genesisJSON, nil
}

func (f *testChainStore) GetAccountCreatedHeight(_ context.Context, address string) uint64 {
	if f.accountCreatedHeights == nil {
		return 0
	}
	return f.accountCreatedHeights[address]
}

func (*testChainStore) QueryBlocks(context.Context, uint64, int, bool) ([]indexermodels.Block, error) {
	return nil, nil
}

func (*testChainStore) QueryBlockSummaries(context.Context, uint64, int, bool) ([]indexermodels.BlockSummary, error) {
	return nil, nil
}

func (*testChainStore) QueryTransactions(context.Context, uint64, int, bool) ([]indexermodels.Transaction, error) {
	return nil, nil
}

func (*testChainStore) QueryTransactionsRaw(context.Context, uint64, int, bool) ([]map[string]interface{}, error) {
	return nil, nil
}

func (*testChainStore) DescribeTable(context.Context, string) ([]db.Column, error) {
	return nil, nil
}

func (*testChainStore) PromoteEntity(context.Context, entities.Entity, uint64) error {
	return nil
}

func (*testChainStore) CleanEntityStaging(context.Context, entities.Entity, uint64) error {
	return nil
}

func (*testChainStore) ValidateQueryHeight(context.Context, *uint64) (uint64, error) {
	return 0, nil
}

func (*testChainStore) GetFullyIndexedHeight(context.Context) (uint64, error) {
	return 0, nil
}

func (*testChainStore) GetAccountByAddress(context.Context, string, *uint64) (*indexermodels.Account, error) {
	return nil, nil
}

func (*testChainStore) QueryAccounts(context.Context, uint64, int, bool) ([]indexermodels.Account, error) {
	return nil, nil
}

func (*testChainStore) GetTransactionByHash(context.Context, string) (*indexermodels.Transaction, error) {
	return nil, nil
}

func (*testChainStore) QueryTransactionsWithFilter(context.Context, uint64, int, bool, string) ([]indexermodels.Transaction, error) {
	return nil, nil
}

func (*testChainStore) QueryEvents(context.Context, uint64, int, bool) ([]indexermodels.Event, error) {
	return nil, nil
}

func (*testChainStore) QueryEventsWithFilter(context.Context, uint64, int, bool, string) ([]indexermodels.Event, error) {
	return nil, nil
}

func (*testChainStore) QueryPools(context.Context, uint64, int, bool) ([]indexermodels.Pool, error) {
	return nil, nil
}

func (*testChainStore) QueryOrders(context.Context, uint64, int, bool, string) ([]indexermodels.Order, error) {
	return nil, nil
}

func (*testChainStore) QueryDexPrices(context.Context, uint64, int, bool, uint64, uint64) ([]indexermodels.DexPrice, error) {
	return nil, nil
}

func (*testChainStore) InsertBlocksStaging(context.Context, *indexermodels.Block) error {
	return nil
}

func (*testChainStore) InsertTransactionsStaging(context.Context, []*indexermodels.Transaction) error {
	return nil
}

func (*testChainStore) InsertBlockSummariesStaging(context.Context, uint64, time.Time, uint32, map[string]uint32) error {
	return nil
}

func (*testChainStore) InitEvents(context.Context) error {
	return nil
}

func (*testChainStore) InsertEventsStaging(context.Context, []*indexermodels.Event) error {
	return nil
}

func (*testChainStore) InitDexPrices(context.Context) error {
	return nil
}

func (*testChainStore) InsertDexPricesStaging(context.Context, []*indexermodels.DexPrice) error {
	return nil
}

func (*testChainStore) InitPools(context.Context) error {
	return nil
}

func (*testChainStore) InsertPoolsStaging(context.Context, []*indexermodels.Pool) error {
	return nil
}

func (*testChainStore) GetDexVolume24h(context.Context) ([]db.DexVolumeStats, error) {
	return nil, nil
}

func (*testChainStore) GetOrderBookDepth(context.Context, uint64, int) ([]db.OrderBookLevel, error) {
	return nil, nil
}

func (*testChainStore) Close() error { return nil }

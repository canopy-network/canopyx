package activity

import (
	"context"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/db/entities"
	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.uber.org/zap/zaptest"
)

func TestIndexTransactionsInsertsAllTxs(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
			Paused:       0,
			Deleted:      0,
		},
	}
	chainStore := &fakeChainStore{chainID: "chain-A", databaseName: "chain_a"}
	chainsMap := xsync.NewMap[string, db.ChainStore]()
	chainsMap.Store("chain-A", chainStore)

	rpcClient := &fakeRPCClient{
		block: &indexermodels.Block{Height: 10},
		txs: []*indexermodels.Transaction{
			{TxHash: "tx1"}, {TxHash: "tx2"},
		},
	}
	activityCtx := &Context{
		Logger:     logger,
		IndexerDB:  adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &fakeRPCFactory{client: rpcClient},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	env.RegisterActivity(activityCtx.IndexTransactions)
	future, err := env.ExecuteActivity(activityCtx.IndexTransactions, types.IndexTransactionsInput{
		ChainID:   "chain-A",
		Height:    10,
		BlockTime: time.Now().UTC(),
	})
	require.NoError(t, err)

	var output types.IndexTransactionsOutput
	require.NoError(t, future.Get(&output))
	require.Equal(t, uint32(2), output.NumTxs)
	require.GreaterOrEqual(t, output.DurationMs, 0.0)
	require.Equal(t, 1, chainStore.insertTransactionCalls)
	require.Len(t, chainStore.lastTxs, 2)
}

func TestFetchBlockFromRPC(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}
	chainsMap := xsync.NewMap[string, db.ChainStore]()
	chainsMap.Store("chain-A", &fakeChainStore{chainID: "chain-A", databaseName: "chain_a"})

	rpcClient := &fakeRPCClient{
		block: &indexermodels.Block{Height: 42, Hash: "abc123"},
	}
	activityCtx := &Context{
		Logger:     logger,
		IndexerDB:  adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &fakeRPCFactory{client: rpcClient},
	}

	// Call the activity directly (not through Temporal test framework)
	// This avoids serialization issues with interface{} types
	input := types.IndexBlockInput{
		ChainID: "chain-A",
		Height:  42,
	}

	output, err := activityCtx.FetchBlockFromRPC(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output.Block)
	require.GreaterOrEqual(t, output.DurationMs, 0.0)

	// Verify the block has correct values
	require.Equal(t, uint64(42), output.Block.Height)
	require.Equal(t, "abc123", output.Block.Hash)
}

func TestSaveBlock(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}
	chainStore := &fakeChainStore{chainID: "chain-A", databaseName: "chain_a"}
	chainsMap := xsync.NewMap[string, db.ChainStore]()
	chainsMap.Store("chain-A", chainStore)

	activityCtx := &Context{
		Logger:     logger,
		IndexerDB:  adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &fakeRPCFactory{client: &fakeRPCClient{}},
	}

	// Call the activity directly (not through Temporal test framework)
	block := &indexermodels.Block{Height: 42, Hash: "xyz789"}
	input := types.SaveBlockInput{
		ChainID: "chain-A",
		Height:  42,
		Block:   block, // Now using concrete type *indexermodels.Block
	}

	output, err := activityCtx.SaveBlock(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, uint64(42), output.Height)
	require.GreaterOrEqual(t, output.DurationMs, 0.0)
	require.Equal(t, 1, chainStore.insertBlockCalls)
	require.NotNil(t, chainStore.lastBlock)
	require.Equal(t, uint64(42), chainStore.lastBlock.Height)
	require.Equal(t, "xyz789", chainStore.lastBlock.Hash)
}

func TestIndexBlockPersistsBlockWithoutSummary(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}
	chainStore := &fakeChainStore{chainID: "chain-A", databaseName: "chain_a"}
	chainsMap := xsync.NewMap[string, db.ChainStore]()
	chainsMap.Store("chain-A", chainStore)

	rpcClient := &fakeRPCClient{
		block: &indexermodels.Block{Height: 42},
	}
	ctx := &Context{
		Logger:     logger,
		IndexerDB:  adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &fakeRPCFactory{client: rpcClient},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	input := types.IndexBlockInput{
		ChainID: "chain-A",
		Height:  42,
	}

	env.RegisterActivity(ctx.IndexBlock)
	future, err := env.ExecuteActivity(ctx.IndexBlock, input)
	require.NoError(t, err)

	var output types.IndexBlockOutput
	require.NoError(t, future.Get(&output))
	require.Equal(t, uint64(42), output.Height)
	require.GreaterOrEqual(t, output.DurationMs, 0.0)
	require.Equal(t, 1, chainStore.insertBlockCalls)
	require.NotNil(t, chainStore.lastBlock)
}

func TestSaveBlockSummary(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}
	chainStore := &fakeChainStore{chainID: "chain-A", databaseName: "chain_a"}
	chainsMap := xsync.NewMap[string, db.ChainStore]()
	chainsMap.Store("chain-A", chainStore)

	ctx := &Context{
		Logger:     logger,
		IndexerDB:  adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &fakeRPCFactory{client: &fakeRPCClient{}},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	input := types.SaveBlockSummaryInput{
		ChainID: "chain-A",
		Height:  42,
		Summaries: types.BlockSummaries{
			NumTxs: 7,
		},
	}

	env.RegisterActivity(ctx.SaveBlockSummary)
	future, err := env.ExecuteActivity(ctx.SaveBlockSummary, input)
	require.NoError(t, err)

	var output types.SaveBlockSummaryOutput
	require.NoError(t, future.Get(&output))
	require.GreaterOrEqual(t, output.DurationMs, 0.0)
	require.Equal(t, 1, chainStore.insertBlockSummaryCalls)
	require.NotNil(t, chainStore.lastBlockSummary)
	require.Equal(t, uint64(42), chainStore.lastBlockSummary.Height)
	require.Equal(t, uint32(7), chainStore.lastBlockSummary.NumTxs)
}

func TestPrepareIndexBlockSkipsWhenExists(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adminStore := &fakeAdminStore{chain: &admin.Chain{ChainID: "chain-A", RPCEndpoints: []string{"http://localhost"}}}
	chainStore := &fakeChainStore{chainID: "chain-A", databaseName: "chain_a", hasBlock: true}
	chainsMap := xsync.NewMap[string, db.ChainStore]()
	chainsMap.Store("chain-A", chainStore)

	activityCtx := &Context{
		Logger:     logger,
		IndexerDB:  adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &fakeRPCFactory{client: &fakeRPCClient{}},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.PrepareIndexBlock)

	var output types.PrepareIndexBlockOutput
	future, err := env.ExecuteActivity(activityCtx.PrepareIndexBlock, types.IndexBlockInput{ChainID: "chain-A", Height: 10})
	require.NoError(t, err)
	require.NoError(t, future.Get(&output))
	require.True(t, output.Skip)
	require.GreaterOrEqual(t, output.DurationMs, 0.0)
	require.Empty(t, chainStore.deletedBlocks)
	require.Empty(t, chainStore.deletedTransactions)
}

func TestPrepareIndexBlockDeletesOnReindex(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adminStore := &fakeAdminStore{chain: &admin.Chain{ChainID: "chain-A", RPCEndpoints: []string{"http://localhost"}}}
	chainStore := &fakeChainStore{chainID: "chain-A", databaseName: "chain_a", hasBlock: true}
	chainsMap := xsync.NewMap[string, db.ChainStore]()
	chainsMap.Store("chain-A", chainStore)

	activityCtx := &Context{
		Logger:     logger,
		IndexerDB:  adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &fakeRPCFactory{client: &fakeRPCClient{}},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.PrepareIndexBlock)

	var output types.PrepareIndexBlockOutput
	future, err := env.ExecuteActivity(activityCtx.PrepareIndexBlock, types.IndexBlockInput{ChainID: "chain-A", Height: 42, Reindex: true})
	require.NoError(t, err)
	require.NoError(t, future.Get(&output))
	require.False(t, output.Skip)
	require.GreaterOrEqual(t, output.DurationMs, 0.0)
	require.Contains(t, chainStore.deletedBlocks, uint64(42))
	require.Contains(t, chainStore.deletedTransactions, uint64(42))
}

type fakeAdminStore struct {
	chain           *admin.Chain
	recordedChainID string
	recordedHeight  uint64
}

func (f *fakeAdminStore) GetChain(context.Context, string) (*admin.Chain, error) {
	return f.chain, nil
}

func (f *fakeAdminStore) RecordIndexed(_ context.Context, chainID string, height uint64, indexingTimeMs float64, indexingDetail string) error {
	f.recordedChainID = chainID
	f.recordedHeight = height
	return nil
}

func (f *fakeAdminStore) ListChain(context.Context) ([]admin.Chain, error) {
	if f.chain == nil {
		return nil, nil
	}
	return []admin.Chain{*f.chain}, nil
}

func (f *fakeAdminStore) LastIndexed(context.Context, string) (uint64, error) {
	return 0, nil
}

func (f *fakeAdminStore) FindGaps(context.Context, string) ([]db.Gap, error) {
	return nil, nil
}

func (f *fakeAdminStore) UpdateRPCHealth(context.Context, string, string, string) error {
	return nil
}

type fakeChainStore struct {
	chainID                 string
	databaseName            string
	insertBlockCalls        int
	insertTransactionCalls  int
	insertBlockSummaryCalls int
	lastBlock               *indexermodels.Block
	lastBlockSummary        *indexermodels.BlockSummary
	lastTxs                 []*indexermodels.Transaction
	execCalls               []string
	hasBlock                bool
	deletedBlocks           []uint64
	deletedTransactions     []uint64
}

func (f *fakeChainStore) DatabaseName() string { return f.databaseName }
func (f *fakeChainStore) ChainKey() string     { return f.chainID }

func (f *fakeChainStore) InsertBlock(_ context.Context, block *indexermodels.Block) error {
	f.insertBlockCalls++
	f.lastBlock = block
	return nil
}

func (f *fakeChainStore) InsertTransactions(_ context.Context, txs []*indexermodels.Transaction) error {
	f.insertTransactionCalls++
	f.lastTxs = txs
	return nil
}

func (f *fakeChainStore) InsertBlockSummary(_ context.Context, height uint64, _ time.Time, numTxs uint32, txCountsByType map[string]uint32) error {
	f.insertBlockSummaryCalls++
	f.lastBlockSummary = &indexermodels.BlockSummary{
		Height:         height,
		NumTxs:         numTxs,
		TxCountsByType: txCountsByType,
	}
	return nil
}

func (f *fakeChainStore) GetBlock(_ context.Context, height uint64) (*indexermodels.Block, error) {
	if f.lastBlock != nil && f.lastBlock.Height == height {
		return f.lastBlock, nil
	}
	return nil, nil
}

func (f *fakeChainStore) GetBlockSummary(_ context.Context, height uint64) (*indexermodels.BlockSummary, error) {
	if f.lastBlockSummary != nil && f.lastBlockSummary.Height == height {
		return f.lastBlockSummary, nil
	}
	return nil, nil
}

func (f *fakeChainStore) HasBlock(_ context.Context, _ uint64) (bool, error) {
	return f.hasBlock, nil
}

func (f *fakeChainStore) DeleteBlock(_ context.Context, height uint64) error {
	f.deletedBlocks = append(f.deletedBlocks, height)
	f.hasBlock = false
	return nil
}

func (f *fakeChainStore) DeleteTransactions(_ context.Context, height uint64) error {
	f.deletedTransactions = append(f.deletedTransactions, height)
	return nil
}

func (f *fakeChainStore) Exec(_ context.Context, query string, args ...any) error {
	f.execCalls = append(f.execCalls, query)
	return nil
}

func (*fakeChainStore) QueryBlocks(context.Context, uint64, int, bool) ([]indexermodels.Block, error) {
	return nil, nil
}

func (*fakeChainStore) QueryBlockSummaries(context.Context, uint64, int, bool) ([]indexermodels.BlockSummary, error) {
	return nil, nil
}

func (*fakeChainStore) QueryTransactions(context.Context, uint64, int, bool) ([]indexermodels.Transaction, error) {
	return nil, nil
}

func (*fakeChainStore) QueryTransactionsRaw(context.Context, uint64, int, bool) ([]map[string]interface{}, error) {
	return nil, nil
}

func (*fakeChainStore) DescribeTable(context.Context, string) ([]db.Column, error) {
	return nil, nil
}

func (*fakeChainStore) PromoteEntity(context.Context, entities.Entity, uint64) error {
	return nil
}

func (*fakeChainStore) CleanEntityStaging(context.Context, entities.Entity, uint64) error {
	return nil
}

func (*fakeChainStore) ValidateQueryHeight(context.Context, *uint64) (uint64, error) {
	return 0, nil
}

func (*fakeChainStore) GetFullyIndexedHeight(context.Context) (uint64, error) {
	return 0, nil
}

func (*fakeChainStore) GetAccountByAddress(context.Context, string, *uint64) (*indexermodels.Account, error) {
	return nil, nil
}

func (*fakeChainStore) QueryAccounts(context.Context, uint64, int, bool) ([]indexermodels.Account, error) {
	return nil, nil
}

func (*fakeChainStore) GetTransactionByHash(context.Context, string) (*indexermodels.Transaction, error) {
	return nil, nil
}

func (*fakeChainStore) QueryTransactionsWithFilter(context.Context, uint64, int, bool, string) ([]indexermodels.Transaction, error) {
	return nil, nil
}

func (*fakeChainStore) Close() error { return nil }

func (*fakeChainStore) InsertBlocksStaging(context.Context, *indexermodels.Block) error {
	return nil
}

func (*fakeChainStore) InsertTransactionsStaging(context.Context, []*indexermodels.Transaction) error {
	return nil
}

func (*fakeChainStore) InsertBlockSummariesStaging(context.Context, uint64, time.Time, uint32, map[string]uint32) error {
	return nil
}

func (*fakeChainStore) InitAccounts(context.Context) error {
	return nil
}

func (*fakeChainStore) InsertAccountsStaging(context.Context, []*indexermodels.Account) error {
	return nil
}

func (*fakeChainStore) InitEvents(context.Context) error {
	return nil
}

func (*fakeChainStore) InsertEventsStaging(context.Context, []*indexermodels.Event) error {
	return nil
}

func (*fakeChainStore) QueryEvents(context.Context, uint64, int, bool) ([]indexermodels.Event, error) {
	return nil, nil
}

func (*fakeChainStore) QueryEventsWithFilter(context.Context, uint64, int, bool, string) ([]indexermodels.Event, error) {
	return nil, nil
}

func (*fakeChainStore) InitPools(context.Context) error {
	return nil
}

func (*fakeChainStore) InsertPoolsStaging(context.Context, []*indexermodels.Pool) error {
	return nil
}

func (*fakeChainStore) QueryPools(context.Context, uint64, int, bool) ([]indexermodels.Pool, error) {
	return nil, nil
}

func (*fakeChainStore) InitOrders(context.Context) error {
	return nil
}

func (*fakeChainStore) InsertOrdersStaging(context.Context, []*indexermodels.Order) error {
	return nil
}

func (*fakeChainStore) QueryOrders(context.Context, uint64, int, bool, string) ([]indexermodels.Order, error) {
	return nil, nil
}

func (*fakeChainStore) InitDexPrices(context.Context) error {
	return nil
}

func (*fakeChainStore) InsertDexPricesStaging(context.Context, []*indexermodels.DexPrice) error {
	return nil
}

func (*fakeChainStore) QueryDexPrices(context.Context, uint64, int, bool, uint64, uint64) ([]indexermodels.DexPrice, error) {
	return nil, nil
}

func (*fakeChainStore) GetDexVolume24h(context.Context) ([]db.DexVolumeStats, error) {
	return nil, nil
}

func (*fakeChainStore) GetOrderBookDepth(context.Context, uint64, int) ([]db.OrderBookLevel, error) {
	return nil, nil
}

type fakeRPCFactory struct {
	client rpc.Client
}

func (f *fakeRPCFactory) NewClient(_ []string) rpc.Client {
	return f.client
}

type fakeRPCClient struct {
	block *indexermodels.Block
	txs   []*indexermodels.Transaction
}

func (f *fakeRPCClient) ChainHead(context.Context) (uint64, error) {
	return 0, nil
}

func (f *fakeRPCClient) BlockByHeight(context.Context, uint64) (*indexermodels.Block, error) {
	return f.block, nil
}

func (f *fakeRPCClient) TxsByHeight(context.Context, uint64) ([]*indexermodels.Transaction, error) {
	return f.txs, nil
}

func (f *fakeRPCClient) AccountsByHeight(context.Context, uint64) ([]*rpc.RpcAccount, error) {
	return nil, nil
}

func (f *fakeRPCClient) GetGenesisState(context.Context, uint64) (*rpc.GenesisState, error) {
	return nil, nil
}

func (f *fakeRPCClient) EventsByHeight(context.Context, uint64) ([]*indexermodels.Event, error) {
	return nil, nil
}

func (f *fakeRPCClient) OrdersByHeight(context.Context, uint64, uint64) ([]*rpc.RpcOrder, error) {
	return nil, nil
}

func (f *fakeRPCClient) DexPrice(context.Context, uint64) (*indexermodels.DexPrice, error) {
	return nil, nil
}

func (f *fakeRPCClient) DexPrices(context.Context) ([]*indexermodels.DexPrice, error) {
	return nil, nil
}

func (f *fakeRPCClient) PoolByID(context.Context, uint64) (*rpc.RpcPool, error) {
	return nil, nil
}

func (f *fakeRPCClient) Pools(context.Context) ([]*rpc.RpcPool, error) {
	return nil, nil
}

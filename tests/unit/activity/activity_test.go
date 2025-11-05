package activity_test

import (
	"context"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/activity"
	"github.com/canopy-network/canopyx/app/indexer/types"
	adminstore "github.com/canopy-network/canopyx/pkg/db/admin"
	chainstore "github.com/canopy-network/canopyx/pkg/db/chain"
	"github.com/canopy-network/canopyx/pkg/db/entities"
	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.uber.org/zap/zaptest"
)

func TestIndexTransactionsInsertsAllTxs(t *testing.T) {
	t.Skip("pending update for new chain store interface")
	logger := zaptest.NewLogger(t)
	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      1,
			RPCEndpoints: []string{"http://rpc.local"},
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
			Paused:       0,
			Deleted:      0,
		},
	}
	chainStore := &fakeChainStore{chainID: "chain-A", databaseName: "chain_a"}
	chainsMap := xsync.NewMap[string, chainstore.Store]()
	chainsMap.Store("chain-A", chainStore)

	rpcClient := &fakeRPCClient{
		block: &rpc.BlockByHeight{
			BlockHeader: rpc.BlockHeader{Height: 10},
		},
		txs: []*rpc.Transaction{
			{Hash: "tx1"}, {Hash: "tx2"},
		},
	}
	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		ChainDB:    chainsMap,
		RPCFactory: &fakeRPCFactory{client: rpcClient},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	env.RegisterActivity(activityCtx.IndexTransactions)
	future, err := env.ExecuteActivity(activityCtx.IndexTransactions, types.IndexTransactionsInput{
		ChainID:   1,
		Height:    10,
		BlockTime: time.Now().UTC(),
	})
	require.NoError(t, err)

	var output types.ActivityIndexTransactionsOutput
	require.NoError(t, future.Get(&output))
	require.Equal(t, uint32(2), output.NumTxs)
	require.GreaterOrEqual(t, output.DurationMs, 0.0)
	require.Equal(t, 1, chainStore.insertTransactionsStagingCalls)
	require.Len(t, chainStore.lastTxs, 2)
}

func TestFetchBlockFromRPC(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      1,
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}
	chainsMap := xsync.NewMap[string, chainstore.Store]()
	chainsMap.Store("chain-A", &fakeChainStore{chainID: "chain-A", databaseName: "chain_a"})

	rpcClient := &fakeRPCClient{
		block: &rpc.BlockByHeight{
			BlockHeader: rpc.BlockHeader{Height: 42, Hash: "abc123"},
		},
	}
	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		ChainDB:    chainsMap,
		RPCFactory: &fakeRPCFactory{client: rpcClient},
	}

	// Call the activity directly (not through Temporal test framework)
	// This avoids serialization issues with interface{} types
	input := types.WorkflowIndexBlockInput{
		ChainID: 1,
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
	t.Skip("pending update for new chain store interface")
	logger := zaptest.NewLogger(t)
	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      1,
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}
	chainStore := &fakeChainStore{chainID: "chain-A", databaseName: "chain_a"}
	chainsMap := xsync.NewMap[string, chainstore.Store]()
	chainsMap.Store("chain-A", chainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		ChainDB:    chainsMap,
		RPCFactory: &fakeRPCFactory{client: &fakeRPCClient{}},
	}

	// Call the activity directly (not through Temporal test framework)
	block := &indexermodels.Block{Height: 42, Hash: "xyz789"}
	input := types.ActivitySaveBlockInput{
		ChainID: 1,
		Height:  42,
		Block:   block, // Now using concrete type *indexermodels.Block
	}

	output, err := activityCtx.SaveBlock(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, uint64(42), output.Height)
	require.GreaterOrEqual(t, output.DurationMs, 0.0)
	require.Equal(t, 1, chainStore.insertBlocksStagingCalls)
	require.NotNil(t, chainStore.lastBlock)
	require.Equal(t, uint64(42), chainStore.lastBlock.Height)
	require.Equal(t, "xyz789", chainStore.lastBlock.Hash)
}

func TestIndexBlockPersistsBlockWithoutSummary(t *testing.T) {
	t.Skip("pending update for new chain store interface")
	logger := zaptest.NewLogger(t)
	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      1,
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}
	chainStore := &fakeChainStore{chainID: "chain-A", databaseName: "chain_a"}
	chainsMap := xsync.NewMap[string, chainstore.Store]()
	chainsMap.Store("chain-A", chainStore)

	rpcClient := &fakeRPCClient{
		block: &rpc.BlockByHeight{
			BlockHeader: rpc.BlockHeader{Height: 42},
		},
	}
	ctx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		ChainDB:    chainsMap,
		RPCFactory: &fakeRPCFactory{client: rpcClient},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	input := types.WorkflowIndexBlockInput{
		ChainID: 1,
		Height:  42,
	}

	env.RegisterActivity(ctx.IndexBlock)
	future, err := env.ExecuteActivity(ctx.IndexBlock, input)
	require.NoError(t, err)

	var output types.ActivitySaveBlockOutput
	require.NoError(t, future.Get(&output))
	require.Equal(t, uint64(42), output.Height)
	require.GreaterOrEqual(t, output.DurationMs, 0.0)
	require.Equal(t, 1, chainStore.insertBlocksStagingCalls)
	require.NotNil(t, chainStore.lastBlock)
}

func TestSaveBlockSummary(t *testing.T) {
	t.Skip("pending update for new chain store interface")
	logger := zaptest.NewLogger(t)
	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      1,
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}
	chainStore := &fakeChainStore{chainID: "chain-A", databaseName: "chain_a"}
	chainsMap := xsync.NewMap[string, chainstore.Store]()
	chainsMap.Store("chain-A", chainStore)

	ctx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		ChainDB:    chainsMap,
		RPCFactory: &fakeRPCFactory{client: &fakeRPCClient{}},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	input := types.ActivitySaveBlockSummaryInput{
		ChainID: 1,
		Height:  42,
		Summaries: types.BlockSummaries{
			NumTxs: 7,
		},
	}

	env.RegisterActivity(ctx.SaveBlockSummary)
	future, err := env.ExecuteActivity(ctx.SaveBlockSummary, input)
	require.NoError(t, err)

	var output types.ActivitySaveBlockSummaryOutput
	require.NoError(t, future.Get(&output))
	require.GreaterOrEqual(t, output.DurationMs, 0.0)
	require.Equal(t, 1, chainStore.insertBlockSummariesStagingCalls)
	require.NotNil(t, chainStore.lastBlockSummary)
	require.Equal(t, uint64(42), chainStore.lastBlockSummary.Height)
	require.Equal(t, uint32(7), chainStore.lastBlockSummary.NumTxs)
}

func TestPrepareIndexBlockSkipsWhenExists(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adminStore := &fakeAdminStore{chain: &admin.Chain{ChainID: 1, RPCEndpoints: []string{"http://localhost"}}}
	chainStore := &fakeChainStore{chainID: "chain-A", databaseName: "chain_a", hasBlock: true}
	chainsMap := xsync.NewMap[string, chainstore.Store]()
	chainsMap.Store("chain-A", chainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		ChainDB:    chainsMap,
		RPCFactory: &fakeRPCFactory{client: &fakeRPCClient{}},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.PrepareIndexBlock)

	var output types.ActivityPrepareIndexBlockOutput
	future, err := env.ExecuteActivity(activityCtx.PrepareIndexBlock, types.WorkflowIndexBlockInput{ChainID: 1, Height: 10})
	require.NoError(t, err)
	require.NoError(t, future.Get(&output))
	require.True(t, output.Skip)
	require.GreaterOrEqual(t, output.DurationMs, 0.0)
	require.Empty(t, chainStore.deletedBlocks)
	require.Empty(t, chainStore.deletedTransactions)
}

func TestPrepareIndexBlockDeletesOnReindex(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adminStore := &fakeAdminStore{chain: &admin.Chain{ChainID: 1, RPCEndpoints: []string{"http://localhost"}}}
	chainStore := &fakeChainStore{chainID: "chain-A", databaseName: "chain_a", hasBlock: true}
	chainsMap := xsync.NewMap[string, chainstore.Store]()
	chainsMap.Store("chain-A", chainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		ChainDB:    chainsMap,
		RPCFactory: &fakeRPCFactory{client: &fakeRPCClient{}},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.PrepareIndexBlock)

	var output types.ActivityPrepareIndexBlockOutput
	future, err := env.ExecuteActivity(activityCtx.PrepareIndexBlock, types.WorkflowIndexBlockInput{ChainID: 1, Height: 42, Reindex: true})
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

func (f *fakeAdminStore) GetChain(context.Context, uint64) (*admin.Chain, error) {
	return f.chain, nil
}

func (f *fakeAdminStore) RecordIndexed(_ context.Context, chainID uint64, height uint64, blockTime time.Time, indexingTimeMs float64, indexingDetail string) error {
	f.recordedHeight = height
	return nil
}

func (f *fakeAdminStore) ListChain(context.Context) ([]admin.Chain, error) {
	if f.chain == nil {
		return nil, nil
	}
	return []admin.Chain{*f.chain}, nil
}

func (f *fakeAdminStore) LastIndexed(context.Context, uint64) (uint64, error) {
	return 0, nil
}

func (f *fakeAdminStore) FindGaps(context.Context, uint64) ([]adminstore.Gap, error) {
	return nil, nil
}

func (f *fakeAdminStore) UpdateRPCHealth(context.Context, uint64, string, string) error {
	return nil
}

func (f *fakeAdminStore) IndexProgressHistory(context.Context, uint64, int, int) ([]admin.ProgressPoint, error) {
	return nil, nil
}

type fakeChainStore struct {
	chainID                          string
	databaseName                     string
	insertBlockCalls                 int
	insertTransactionCalls           int
	insertBlockSummaryCalls          int
	insertBlocksStagingCalls         int
	insertTransactionsStagingCalls   int
	insertBlockSummariesStagingCalls int
	lastBlock                        *indexermodels.Block
	lastBlockSummary                 *indexermodels.BlockSummary
	lastTxs                          []*indexermodels.Transaction
	execCalls                        []string
	hasBlock                         bool
	deletedBlocks                    []uint64
	deletedTransactions              []uint64
	insertedAccounts                 []*indexermodels.Account
	accountCreatedHeights            map[string]uint64
	genesisJSON                      string
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

func (*fakeChainStore) DescribeTable(context.Context, string) ([]chainstore.Column, error) {
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

func (f *fakeChainStore) InsertBlocksStaging(_ context.Context, block *indexermodels.Block) error {
	f.insertBlocksStagingCalls++
	f.lastBlock = block
	return nil
}

func (f *fakeChainStore) InsertTransactionsStaging(_ context.Context, txs []*indexermodels.Transaction) error {
	f.insertTransactionsStagingCalls++
	f.lastTxs = txs
	return nil
}

func (f *fakeChainStore) InsertBlockSummariesStaging(_ context.Context, summary *indexermodels.BlockSummary) error {
	f.insertBlockSummariesStagingCalls++
	f.lastBlockSummary = summary
	return nil
}

func (*fakeChainStore) InitAccounts(context.Context) error {
	return nil
}

func (f *fakeChainStore) InsertAccountsStaging(_ context.Context, accounts []*indexermodels.Account) error {
	f.insertedAccounts = append(f.insertedAccounts, accounts...)
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

func (f *fakeChainStore) GetGenesisData(_ context.Context, _ uint64) (string, error) {
	return f.genesisJSON, nil
}

func (f *fakeChainStore) HasGenesis(context.Context, uint64) (bool, error) {
	return f.genesisJSON != "", nil
}

func (f *fakeChainStore) InsertGenesis(_ context.Context, height uint64, data string, _ time.Time) error {
	if height != 0 {
		return nil
	}
	f.genesisJSON = data
	return nil
}

func (f *fakeChainStore) GetAccountCreatedHeight(_ context.Context, address string) uint64 {
	if f.accountCreatedHeights == nil {
		return 0
	}
	return f.accountCreatedHeights[address]
}

func (*fakeChainStore) GetOrderCreatedHeight(context.Context, string) uint64 {
	return 0
}

func (*fakeChainStore) GetEventsByTypeAndHeight(context.Context, uint64, bool, ...string) ([]*indexermodels.Event, error) {
	return nil, nil
}

func (*fakeChainStore) InitializeDB(context.Context) error {
	return nil
}

func (*fakeChainStore) Select(context.Context, interface{}, string, ...any) error {
	return nil
}

func (*fakeChainStore) GetTableSchema(context.Context, string) ([]chainstore.Column, error) {
	return nil, nil
}

func (*fakeChainStore) GetTableDataPaginated(context.Context, string, int, int, *uint64, *uint64) ([]map[string]interface{}, int64, bool, error) {
	return nil, 0, false, nil
}

func (*fakeChainStore) InsertDexOrdersStaging(context.Context, []*indexermodels.DexOrder) error {
	return nil
}

func (*fakeChainStore) InsertDexDepositsStaging(context.Context, []*indexermodels.DexDeposit) error {
	return nil
}

func (*fakeChainStore) InsertDexWithdrawalsStaging(context.Context, []*indexermodels.DexWithdrawal) error {
	return nil
}

func (*fakeChainStore) InsertPoolPointsByHolderStaging(context.Context, []*indexermodels.PoolPointsByHolder) error {
	return nil
}

func (*fakeChainStore) InsertParamsStaging(context.Context, *indexermodels.Params) error {
	return nil
}

func (*fakeChainStore) InsertValidatorsStaging(context.Context, []*indexermodels.Validator) error {
	return nil
}

func (*fakeChainStore) InsertValidatorSigningInfoStaging(context.Context, []*indexermodels.ValidatorSigningInfo) error {
	return nil
}

func (*fakeChainStore) InsertCommitteesStaging(context.Context, []*indexermodels.Committee) error {
	return nil
}

func (*fakeChainStore) InsertCommitteeValidatorsStaging(context.Context, []*indexermodels.CommitteeValidator) error {
	return nil
}

func (*fakeChainStore) InsertPollSnapshotsStaging(context.Context, []*indexermodels.PollSnapshot) error {
	return nil
}

type fakeRPCFactory struct {
	client rpc.Client
}

func (f *fakeRPCFactory) NewClient(_ []string) rpc.Client {
	return f.client
}

type fakeRPCClient struct {
	block *rpc.BlockByHeight
	txs   []*rpc.Transaction
}

func (f *fakeRPCClient) ChainHead(context.Context) (uint64, error) {
	return 0, nil
}

func (f *fakeRPCClient) BlockByHeight(context.Context, uint64) (*rpc.BlockByHeight, error) {
	return f.block, nil
}

func (f *fakeRPCClient) TxsByHeight(context.Context, uint64) ([]*rpc.Transaction, error) {
	return f.txs, nil
}

func (f *fakeRPCClient) AccountsByHeight(context.Context, uint64) ([]*rpc.Account, error) {
	return nil, nil
}

func (f *fakeRPCClient) GetGenesisState(context.Context, uint64) (*rpc.GenesisState, error) {
	return nil, nil
}

func (f *fakeRPCClient) EventsByHeight(context.Context, uint64) ([]*rpc.RpcEvent, error) {
	return nil, nil
}

func (f *fakeRPCClient) OrdersByHeight(context.Context, uint64, uint64) ([]*rpc.RpcOrder, error) {
	return nil, nil
}

func (f *fakeRPCClient) DexPrice(context.Context, uint64, uint64) (*rpc.RpcDexPrice, error) {
	return nil, nil
}

func (f *fakeRPCClient) DexPrices(context.Context, uint64) ([]*rpc.RpcDexPrice, error) {
	return nil, nil
}

func (f *fakeRPCClient) PoolByID(context.Context, uint64) (*rpc.RpcPool, error) {
	return nil, nil
}

func (f *fakeRPCClient) Pools(context.Context) ([]*rpc.RpcPool, error) {
	return nil, nil
}

func (f *fakeRPCClient) AllParams(context.Context, uint64) (*rpc.RpcAllParams, error) {
	return nil, nil
}

func (f *fakeRPCClient) FeeParams(context.Context, uint64) (*rpc.FeeParams, error) {
	return nil, nil
}

func (f *fakeRPCClient) ConParams(context.Context, uint64) (*rpc.ConsensusParams, error) {
	return nil, nil
}

func (f *fakeRPCClient) ValParams(context.Context, uint64) (*rpc.ValidatorParams, error) {
	return nil, nil
}

func (f *fakeRPCClient) GovParams(context.Context, uint64) (*rpc.GovParams, error) {
	return nil, nil
}

func (f *fakeRPCClient) Validators(context.Context, uint64) ([]*rpc.RpcValidator, error) {
	return nil, nil
}

func (f *fakeRPCClient) NonSigners(context.Context, uint64) ([]*rpc.RpcNonSigner, error) {
	return nil, nil
}

func (f *fakeRPCClient) CommitteeData(context.Context, uint64, uint64) (*rpc.RpcCommitteeData, error) {
	return nil, nil
}

func (f *fakeRPCClient) CommitteesData(context.Context, uint64) ([]*rpc.RpcCommitteeData, error) {
	return nil, nil
}

func (f *fakeRPCClient) SubsidizedCommittees(context.Context, uint64) ([]uint64, error) {
	return nil, nil
}

func (f *fakeRPCClient) RetiredCommittees(context.Context, uint64) ([]uint64, error) {
	return nil, nil
}

func (f *fakeRPCClient) Poll(context.Context) (rpc.RpcPoll, error) {
	return rpc.RpcPoll{}, nil
}

func (f *fakeRPCClient) State(context.Context) (*rpc.StateResponse, error) {
	return nil, nil
}

func (f *fakeRPCClient) DexBatchByHeight(context.Context, uint64, uint64) (*rpc.RpcDexBatch, error) {
	return nil, nil
}

func (f *fakeRPCClient) NextDexBatchByHeight(context.Context, uint64, uint64) (*rpc.RpcDexBatch, error) {
	return nil, nil
}

func (f *fakeRPCClient) AllDexBatchesByHeight(context.Context, uint64) ([]*rpc.RpcDexBatch, error) {
	return nil, nil
}

func (f *fakeRPCClient) AllNextDexBatchesByHeight(context.Context, uint64) ([]*rpc.RpcDexBatch, error) {
	return nil, nil
}

func (f *fakeRPCClient) StateByHeight(context.Context, *uint64) (*rpc.StateResponse, error) {
	return nil, nil
}

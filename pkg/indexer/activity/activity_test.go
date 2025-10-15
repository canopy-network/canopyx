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
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
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
		raws: []*indexermodels.TransactionRaw{
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
	future, err := env.ExecuteActivity(activityCtx.IndexTransactions, types.IndexBlockInput{
		ChainID: "chain-A",
		Height:  10,
	})
	require.NoError(t, err)

	var output types.IndexTransactionsOutput
	require.NoError(t, future.Get(&output))
	require.Equal(t, uint32(2), output.NumTxs)
	require.GreaterOrEqual(t, output.DurationMs, 0.0)
	require.Equal(t, 1, chainStore.insertTransactionCalls)
	require.Len(t, chainStore.lastTxs, 2)
}

func TestIndexBlockPersistsBlockWithSummary(t *testing.T) {
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
		BlockSummaries: &types.BlockSummaries{
			NumTxs: 7,
		},
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
	require.Equal(t, uint32(7), chainStore.lastBlock.NumTxs)
}

func TestIndexBlockFailsOnMissingBlockSummaries(t *testing.T) {
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

	env.RegisterActivity(ctx.IndexBlock)
	_, err := env.ExecuteActivity(ctx.IndexBlock, types.IndexBlockInput{
		ChainID: "chain-A",
		Height:  42,
	})
	require.Error(t, err)
	appErr := &temporal.ApplicationError{}
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, "block_summaries_not_found", appErr.Type())
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
	chainID                string
	databaseName           string
	insertBlockCalls       int
	insertTransactionCalls int
	lastBlock              *indexermodels.Block
	lastTxs                []*indexermodels.Transaction
	lastRaws               []*indexermodels.TransactionRaw
	execCalls              []string
	hasBlock               bool
	deletedBlocks          []uint64
	deletedTransactions    []uint64
}

func (f *fakeChainStore) DatabaseName() string { return f.databaseName }
func (f *fakeChainStore) ChainKey() string     { return f.chainID }

func (f *fakeChainStore) InsertBlock(_ context.Context, block *indexermodels.Block) error {
	f.insertBlockCalls++
	f.lastBlock = block
	return nil
}

func (f *fakeChainStore) InsertTransactions(_ context.Context, txs []*indexermodels.Transaction, raws []*indexermodels.TransactionRaw) error {
	f.insertTransactionCalls++
	f.lastTxs = txs
	f.lastRaws = raws
	return nil
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

func (*fakeChainStore) QueryBlocks(context.Context, uint64, int) ([]indexermodels.BlockRow, error) {
	return nil, nil
}

func (*fakeChainStore) QueryTransactions(context.Context, uint64, int) ([]indexermodels.TransactionRow, error) {
	return nil, nil
}

func (*fakeChainStore) Close() error { return nil }

type fakeRPCFactory struct {
	client rpc.Client
}

func (f *fakeRPCFactory) NewClient(_ []string) rpc.Client {
	return f.client
}

type fakeRPCClient struct {
	block *indexermodels.Block
	txs   []*indexermodels.Transaction
	raws  []*indexermodels.TransactionRaw
}

func (f *fakeRPCClient) ChainHead(context.Context) (uint64, error) {
	return 0, nil
}

func (f *fakeRPCClient) BlockByHeight(context.Context, uint64) (*indexermodels.Block, error) {
	return f.block, nil
}

func (f *fakeRPCClient) TxsByHeight(context.Context, uint64) ([]*indexermodels.Transaction, []*indexermodels.TransactionRaw, error) {
	return f.txs, f.raws, nil
}

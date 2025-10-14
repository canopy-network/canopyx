package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/indexer/activity"
	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.uber.org/zap/zaptest"
)

func TestIndexBlockWorkflowHappyPath(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	logger := zaptest.NewLogger(t)
	adminStore := &wfFakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		},
	}
	chainStore := &wfFakeChainStore{chainID: "chain-A", databaseName: "chain_a"}
	chainsMap := xsync.NewMap[string, db.ChainStore]()
	chainsMap.Store("chain-A", chainStore)

	rpcClient := &wfFakeRPCClient{
		block: &indexermodels.Block{Height: 21},
		txs: []*indexermodels.Transaction{
			{TxHash: "tx1"},
		},
		raws: []*indexermodels.TransactionRaw{
			{TxHash: "tx1"},
		},
	}

	activityCtx := &activity.Context{
		Logger:     logger,
		IndexerDB:  adminStore,
		ChainsDB:   chainsMap,
		RPCFactory: &wfFakeRPCFactory{client: rpcClient},
	}

	wfCtx := Context{
		TemporalClient:  &temporal.Client{IndexerQueue: "index:%s"},
		ActivityContext: activityCtx,
	}

	env.RegisterWorkflow(wfCtx.IndexBlockWorkflow)
	env.RegisterActivity(activityCtx.PrepareIndexBlock)
	env.RegisterActivity(activityCtx.IndexTransactions)
	env.RegisterActivity(activityCtx.IndexBlock)
	env.RegisterActivity(activityCtx.RecordIndexed)

	input := types.IndexBlockInput{ChainID: "chain-A", Height: 21}
	env.ExecuteWorkflow(wfCtx.IndexBlockWorkflow, input)

	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, 1, chainStore.insertTransactionCalls)
	require.Equal(t, 1, chainStore.insertBlockCalls)
	require.NotNil(t, chainStore.lastBlock)
	require.Equal(t, uint64(21), chainStore.lastBlock.Height)
	require.Equal(t, uint32(1), chainStore.lastBlock.NumTxs)
	require.Equal(t, "chain-A", adminStore.recordedChainID)
	require.Equal(t, uint64(21), adminStore.recordedHeight)
}

func TestPriorityKeyForHeight(t *testing.T) {
	require.Equal(t, PriorityHigh, calculateBlockPriority(10_000, 10_000, false))
	require.Equal(t, PriorityHigh, calculateBlockPriority(10_000, 9_950, false))
	require.Equal(t, PriorityMedium, calculateBlockPriority(10_000, 5_500, false))
	require.Equal(t, PriorityLow, calculateBlockPriority(10_000, 1_000, false))
}

type wfFakeAdminStore struct {
	chain           *admin.Chain
	recordedChainID string
	recordedHeight  uint64
}

func (f *wfFakeAdminStore) GetChain(context.Context, string) (*admin.Chain, error) {
	return f.chain, nil
}

func (f *wfFakeAdminStore) RecordIndexed(_ context.Context, chainID string, height uint64) error {
	f.recordedChainID = chainID
	f.recordedHeight = height
	return nil
}

func (f *wfFakeAdminStore) ListChain(context.Context) ([]admin.Chain, error) {
	if f.chain == nil {
		return nil, nil
	}
	return []admin.Chain{*f.chain}, nil
}

func (f *wfFakeAdminStore) LastIndexed(context.Context, string) (uint64, error) {
	return 0, nil
}

func (f *wfFakeAdminStore) FindGaps(context.Context, string) ([]db.Gap, error) {
	return nil, nil
}

type wfFakeChainStore struct {
	chainID                string
	databaseName           string
	insertBlockCalls       int
	insertTransactionCalls int
	lastBlock              *indexermodels.Block
	hasBlock               bool
	deletedBlocks          []uint64
	deletedTransactions    []uint64
}

func (f *wfFakeChainStore) DatabaseName() string { return f.databaseName }
func (f *wfFakeChainStore) ChainKey() string     { return f.chainID }

func (f *wfFakeChainStore) InsertBlock(_ context.Context, block *indexermodels.Block) error {
	f.insertBlockCalls++
	f.lastBlock = block
	return nil
}

func (f *wfFakeChainStore) InsertTransactions(_ context.Context, _ []*indexermodels.Transaction, _ []*indexermodels.TransactionRaw) error {
	f.insertTransactionCalls++
	return nil
}

func (f *wfFakeChainStore) HasBlock(_ context.Context, _ uint64) (bool, error) {
	return f.hasBlock, nil
}

func (f *wfFakeChainStore) DeleteBlock(_ context.Context, height uint64) error {
	f.deletedBlocks = append(f.deletedBlocks, height)
	f.hasBlock = false
	return nil
}

func (f *wfFakeChainStore) DeleteTransactions(_ context.Context, height uint64) error {
	f.deletedTransactions = append(f.deletedTransactions, height)
	return nil
}

func (*wfFakeChainStore) Exec(context.Context, string, ...any) error { return nil }
func (*wfFakeChainStore) Close() error                               { return nil }

type wfFakeRPCFactory struct {
	client rpc.Client
}

func (f *wfFakeRPCFactory) NewClient(_ []string) rpc.Client {
	return f.client
}

type wfFakeRPCClient struct {
	block *indexermodels.Block
	txs   []*indexermodels.Transaction
	raws  []*indexermodels.TransactionRaw
}

func (f *wfFakeRPCClient) ChainHead(context.Context) (uint64, error) { return 0, nil }

func (f *wfFakeRPCClient) BlockByHeight(context.Context, uint64) (*indexermodels.Block, error) {
	return f.block, nil
}

func (f *wfFakeRPCClient) TxsByHeight(context.Context, uint64) ([]*indexermodels.Transaction, []*indexermodels.TransactionRaw, error) {
	return f.txs, f.raws, nil
}

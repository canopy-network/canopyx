package activity

import (
	"context"
	"testing"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.uber.org/zap/zaptest"
)

func TestComputeTxsAllChainsExecutesQueriesForActiveChains(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adminStore := &reporterFakeAdminStore{
		chains: []admin.Chain{
			{ChainID: "chain-A", Paused: 0},
			{ChainID: "chain-B", Paused: 1},
		},
	}
	chainA := &reporterFakeChainStore{chainID: "chain-A", databaseName: "chain_a"}
	chainsMap := xsync.NewMap[string, db.ChainStore]()
	chainsMap.Store("chain-A", chainA)

	ctx := &Context{
		Logger:    logger,
		IndexerDB: adminStore,
		ReportsDB: &reporterFakeReportsStore{name: "reports_db"},
		ChainsDB:  chainsMap,
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()

	env.RegisterActivity(ctx.ComputeTxsAllChains)
	_, err := env.ExecuteActivity(ctx.ComputeTxsAllChains)
	require.NoError(t, err)

	// Only active chain should run three queries (hourly, daily, rolling 24h)
	require.Equal(t, 3, len(chainA.execQueries))
}

type reporterFakeAdminStore struct {
	chains []admin.Chain
}

func (f *reporterFakeAdminStore) GetChain(context.Context, string) (*admin.Chain, error) {
	if len(f.chains) == 0 {
		return nil, nil
	}
	return &f.chains[0], nil
}

func (f *reporterFakeAdminStore) RecordIndexed(context.Context, string, uint64, float64, string) error {
	return nil
}

func (f *reporterFakeAdminStore) ListChain(context.Context) ([]admin.Chain, error) {
	return f.chains, nil
}

func (f *reporterFakeAdminStore) LastIndexed(context.Context, string) (uint64, error) {
	return 0, nil
}

func (f *reporterFakeAdminStore) FindGaps(context.Context, string) ([]db.Gap, error) {
	return nil, nil
}

func (f *reporterFakeAdminStore) UpdateRPCHealth(context.Context, string, string, string) error {
	return nil
}

type reporterFakeChainStore struct {
	chainID      string
	databaseName string
	execQueries  []string
}

func (f *reporterFakeChainStore) DatabaseName() string { return f.databaseName }
func (f *reporterFakeChainStore) ChainKey() string     { return f.chainID }

func (f *reporterFakeChainStore) InsertBlock(context.Context, *indexermodels.Block) error {
	return nil
}

func (f *reporterFakeChainStore) InsertTransactions(context.Context, []*indexermodels.Transaction) error {
	return nil
}

func (f *reporterFakeChainStore) HasBlock(context.Context, uint64) (bool, error) {
	return false, nil
}

func (f *reporterFakeChainStore) DeleteBlock(context.Context, uint64) error {
	return nil
}

func (f *reporterFakeChainStore) DeleteTransactions(context.Context, uint64) error {
	return nil
}

func (f *reporterFakeChainStore) Exec(_ context.Context, query string, _ ...any) error {
	f.execQueries = append(f.execQueries, query)
	return nil
}

func (*reporterFakeChainStore) InsertBlockSummary(context.Context, uint64, uint32) error {
	return nil
}

func (*reporterFakeChainStore) GetBlockSummary(context.Context, uint64) (*indexermodels.BlockSummary, error) {
	return nil, nil
}

func (*reporterFakeChainStore) QueryBlocks(context.Context, uint64, int) ([]indexermodels.Block, error) {
	return nil, nil
}

func (*reporterFakeChainStore) QueryBlockSummaries(context.Context, uint64, int) ([]indexermodels.BlockSummary, error) {
	return nil, nil
}

func (*reporterFakeChainStore) QueryTransactions(context.Context, uint64, int) ([]indexermodels.Transaction, error) {
	return nil, nil
}

func (*reporterFakeChainStore) QueryTransactionsRaw(context.Context, uint64, int) ([]map[string]interface{}, error) {
	return nil, nil
}

func (*reporterFakeChainStore) DescribeTable(context.Context, string) ([]db.Column, error) {
	return nil, nil
}

func (*reporterFakeChainStore) Close() error { return nil }

type reporterFakeReportsStore struct{ name string }

func (f *reporterFakeReportsStore) DatabaseName() string { return f.name }
func (f *reporterFakeReportsStore) Close() error         { return nil }

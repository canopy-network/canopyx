package activity_test

import (
	"context"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/db/entities"
	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/reporter/activity"
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

	ctx := &activity.Context{
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

func (*reporterFakeChainStore) InsertBlockSummary(context.Context, uint64, time.Time, uint32, map[string]uint32) error {
	return nil
}

func (*reporterFakeChainStore) GetBlockSummary(context.Context, uint64) (*indexermodels.BlockSummary, error) {
	return nil, nil
}

func (*reporterFakeChainStore) QueryBlocks(context.Context, uint64, int, bool) ([]indexermodels.Block, error) {
	return nil, nil
}

func (*reporterFakeChainStore) QueryBlockSummaries(context.Context, uint64, int, bool) ([]indexermodels.BlockSummary, error) {
	return nil, nil
}

func (*reporterFakeChainStore) QueryTransactions(context.Context, uint64, int, bool) ([]indexermodels.Transaction, error) {
	return nil, nil
}

func (*reporterFakeChainStore) QueryTransactionsRaw(context.Context, uint64, int, bool) ([]map[string]interface{}, error) {
	return nil, nil
}

func (*reporterFakeChainStore) DescribeTable(context.Context, string) ([]db.Column, error) {
	return nil, nil
}

// Entity staging and promotion methods
func (*reporterFakeChainStore) CleanEntityStaging(context.Context, entities.Entity, uint64) error {
	return nil
}

func (*reporterFakeChainStore) PromoteEntity(context.Context, entities.Entity, uint64) error {
	return nil
}

// Height validation methods
func (*reporterFakeChainStore) ValidateQueryHeight(context.Context, *uint64) (uint64, error) {
	return 0, nil
}

func (*reporterFakeChainStore) GetFullyIndexedHeight(context.Context) (uint64, error) {
	return 0, nil
}

// Account methods
func (*reporterFakeChainStore) QueryAccounts(context.Context, uint64, int, bool) ([]indexermodels.Account, error) {
	return nil, nil
}

func (*reporterFakeChainStore) GetAccountByAddress(context.Context, string, *uint64) (*indexermodels.Account, error) {
	return nil, nil
}

// Event methods
func (*reporterFakeChainStore) QueryEvents(context.Context, uint64, int, bool) ([]indexermodels.Event, error) {
	return nil, nil
}

func (*reporterFakeChainStore) QueryEventsWithFilter(context.Context, uint64, int, bool, string) ([]indexermodels.Event, error) {
	return nil, nil
}

func (*reporterFakeChainStore) InsertEventsStaging(context.Context, []*indexermodels.Event) error {
	return nil
}

func (*reporterFakeChainStore) InitEvents(context.Context) error {
	return nil
}

// Pool methods
func (*reporterFakeChainStore) QueryPools(context.Context, uint64, int, bool) ([]indexermodels.Pool, error) {
	return nil, nil
}

func (*reporterFakeChainStore) InsertPoolsStaging(context.Context, []*indexermodels.Pool) error {
	return nil
}

func (*reporterFakeChainStore) InitPools(context.Context) error {
	return nil
}

// Order methods
func (*reporterFakeChainStore) QueryOrders(context.Context, uint64, int, bool, string) ([]indexermodels.Order, error) {
	return nil, nil
}

// DexPrice methods
func (*reporterFakeChainStore) QueryDexPrices(context.Context, uint64, int, bool, uint64, uint64) ([]indexermodels.DexPrice, error) {
	return nil, nil
}

func (*reporterFakeChainStore) InsertDexPricesStaging(context.Context, []*indexermodels.DexPrice) error {
	return nil
}

func (*reporterFakeChainStore) InitDexPrices(context.Context) error {
	return nil
}

// Analytics methods
func (*reporterFakeChainStore) GetDexVolume24h(context.Context) ([]db.DexVolumeStats, error) {
	return nil, nil
}

func (*reporterFakeChainStore) GetOrderBookDepth(context.Context, uint64, int) ([]db.OrderBookLevel, error) {
	return nil, nil
}

// Block and transaction methods
func (*reporterFakeChainStore) GetBlock(context.Context, uint64) (*indexermodels.Block, error) {
	return nil, nil
}

func (*reporterFakeChainStore) GetTransactionByHash(context.Context, string) (*indexermodels.Transaction, error) {
	return nil, nil
}

func (*reporterFakeChainStore) QueryTransactionsWithFilter(context.Context, uint64, int, bool, string) ([]indexermodels.Transaction, error) {
	return nil, nil
}

func (*reporterFakeChainStore) InsertBlocksStaging(context.Context, *indexermodels.Block) error {
	return nil
}

func (*reporterFakeChainStore) InsertTransactionsStaging(context.Context, []*indexermodels.Transaction) error {
	return nil
}

func (*reporterFakeChainStore) InsertBlockSummariesStaging(context.Context, uint64, time.Time, uint32, map[string]uint32) error {
	return nil
}

func (*reporterFakeChainStore) Close() error { return nil }

type reporterFakeReportsStore struct{ name string }

func (f *reporterFakeReportsStore) DatabaseName() string { return f.name }
func (f *reporterFakeReportsStore) Close() error         { return nil }

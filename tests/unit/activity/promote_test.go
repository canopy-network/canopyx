package activity_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/canopy-network/canopyx/app/indexer/activity"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	chainstore "github.com/canopy-network/canopyx/pkg/db/chain"
	"github.com/canopy-network/canopyx/pkg/db/entities"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// MockChainStore is a mock implementation of chain.Store for testing
type MockChainStore struct {
	mock.Mock
}

func (m *MockChainStore) DatabaseName() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockChainStore) ChainKey() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockChainStore) InsertBlock(ctx context.Context, block *indexer.Block) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

func (m *MockChainStore) GetBlock(ctx context.Context, height uint64) (*indexer.Block, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*indexer.Block), args.Error(1)
}

func (m *MockChainStore) InsertTransactions(ctx context.Context, txs []*indexer.Transaction) error {
	args := m.Called(ctx, txs)
	return args.Error(0)
}

func (m *MockChainStore) InsertBlockSummary(ctx context.Context, height uint64, blockTime time.Time, numTxs uint32, txCountsByType map[string]uint32) error {
	args := m.Called(ctx, height, blockTime, numTxs, txCountsByType)
	return args.Error(0)
}

func (m *MockChainStore) GetBlockSummary(ctx context.Context, height uint64) (*indexer.BlockSummary, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*indexer.BlockSummary), args.Error(1)
}

func (m *MockChainStore) HasBlock(ctx context.Context, height uint64) (bool, error) {
	args := m.Called(ctx, height)
	return args.Bool(0), args.Error(1)
}

func (m *MockChainStore) DeleteBlock(ctx context.Context, height uint64) error {
	args := m.Called(ctx, height)
	return args.Error(0)
}

func (m *MockChainStore) DeleteTransactions(ctx context.Context, height uint64) error {
	args := m.Called(ctx, height)
	return args.Error(0)
}

func (m *MockChainStore) Exec(ctx context.Context, query string, args ...any) error {
	callArgs := m.Called(ctx, query, args)
	return callArgs.Error(0)
}

func (m *MockChainStore) InsertAccountsStaging(ctx context.Context, accounts []*indexer.Account) error {
	args := m.Called(ctx, accounts)
	return args.Error(0)
}

func (m *MockChainStore) GetGenesisData(ctx context.Context, height uint64) (string, error) {
	args := m.Called(ctx, height)
	return args.String(0), args.Error(1)
}

func (m *MockChainStore) GetAccountCreatedHeight(ctx context.Context, address string) uint64 {
	args := m.Called(ctx, address)
	if args.Get(0) == nil {
		return 0
	}
	return args.Get(0).(uint64)
}

func (m *MockChainStore) GetOrderCreatedHeight(ctx context.Context, orderID string) uint64 {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return 0
	}
	return args.Get(0).(uint64)
}

func (m *MockChainStore) HasGenesis(context.Context, uint64) (bool, error) {
	return true, nil
}

func (m *MockChainStore) InsertGenesis(context.Context, uint64, string, time.Time) error {
	return nil
}

func (m *MockChainStore) QueryBlocks(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.Block, error) {
	args := m.Called(ctx, cursor, limit, sortDesc)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]indexer.Block), args.Error(1)
}

func (m *MockChainStore) QueryBlockSummaries(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.BlockSummary, error) {
	args := m.Called(ctx, cursor, limit, sortDesc)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]indexer.BlockSummary), args.Error(1)
}

func (m *MockChainStore) QueryTransactions(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.Transaction, error) {
	args := m.Called(ctx, cursor, limit, sortDesc)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]indexer.Transaction), args.Error(1)
}

func (m *MockChainStore) QueryTransactionsRaw(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]map[string]interface{}, error) {
	args := m.Called(ctx, cursor, limit, sortDesc)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]map[string]interface{}), args.Error(1)
}

func (m *MockChainStore) DescribeTable(ctx context.Context, tableName string) ([]chainstore.Column, error) {
	args := m.Called(ctx, tableName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]chainstore.Column), args.Error(1)
}

func (m *MockChainStore) PromoteEntity(ctx context.Context, entity entities.Entity, height uint64) error {
	args := m.Called(ctx, entity, height)
	return args.Error(0)
}

func (m *MockChainStore) CleanEntityStaging(ctx context.Context, entity entities.Entity, height uint64) error {
	args := m.Called(ctx, entity, height)
	return args.Error(0)
}

func (m *MockChainStore) ValidateQueryHeight(ctx context.Context, requestedHeight *uint64) (uint64, error) {
	args := m.Called(ctx, requestedHeight)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockChainStore) GetFullyIndexedHeight(ctx context.Context) (uint64, error) {
	args := m.Called(ctx)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockChainStore) GetAccountByAddress(ctx context.Context, address string, height *uint64) (*indexer.Account, error) {
	args := m.Called(ctx, address, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*indexer.Account), args.Error(1)
}

func (m *MockChainStore) QueryAccounts(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.Account, error) {
	args := m.Called(ctx, cursor, limit, sortDesc)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]indexer.Account), args.Error(1)
}

func (m *MockChainStore) GetTransactionByHash(ctx context.Context, hash string) (*indexer.Transaction, error) {
	args := m.Called(ctx, hash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*indexer.Transaction), args.Error(1)
}

func (m *MockChainStore) QueryTransactionsWithFilter(ctx context.Context, cursor uint64, limit int, sortDesc bool, messageType string) ([]indexer.Transaction, error) {
	args := m.Called(ctx, cursor, limit, sortDesc, messageType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]indexer.Transaction), args.Error(1)
}

func (m *MockChainStore) QueryEvents(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.Event, error) {
	return nil, nil
}

func (m *MockChainStore) QueryEventsWithFilter(ctx context.Context, cursor uint64, limit int, sortDesc bool, eventType string) ([]indexer.Event, error) {
	return nil, nil
}

func (m *MockChainStore) QueryPools(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.Pool, error) {
	return nil, nil
}

func (m *MockChainStore) QueryOrders(ctx context.Context, cursor uint64, limit int, sortDesc bool, status string) ([]indexer.Order, error) {
	return nil, nil
}

func (m *MockChainStore) InsertOrdersStaging(context.Context, []*indexer.Order) error {
	return nil
}

func (m *MockChainStore) QueryDexPrices(ctx context.Context, cursor uint64, limit int, sortDesc bool, localChainID, remoteChainID uint64) ([]indexer.DexPrice, error) {
	return nil, nil
}

func (m *MockChainStore) InsertBlocksStaging(ctx context.Context, block *indexer.Block) error {
	return nil
}

func (m *MockChainStore) InsertTransactionsStaging(ctx context.Context, txs []*indexer.Transaction) error {
	return nil
}

func (m *MockChainStore) InsertBlockSummariesStaging(ctx context.Context, height uint64, blockTime time.Time, numTxs uint32, txCountsByType map[string]uint32) error {
	return nil
}

func (m *MockChainStore) InitEvents(ctx context.Context) error {
	return nil
}

func (m *MockChainStore) InsertEventsStaging(ctx context.Context, events []*indexer.Event) error {
	return nil
}

func (m *MockChainStore) InitDexPrices(ctx context.Context) error {
	return nil
}

func (m *MockChainStore) InsertDexPricesStaging(ctx context.Context, prices []*indexer.DexPrice) error {
	return nil
}

func (m *MockChainStore) InitPools(ctx context.Context) error {
	return nil
}

func (m *MockChainStore) InsertPoolsStaging(ctx context.Context, pools []*indexer.Pool) error {
	return nil
}

func (m *MockChainStore) GetDexVolume24h(ctx context.Context) ([]chainstore.DexVolumeStats, error) {
	return nil, nil
}

func (m *MockChainStore) GetOrderBookDepth(ctx context.Context, committee uint64, limit int) ([]chainstore.OrderBookLevel, error) {
	return nil, nil
}

func (m *MockChainStore) Close() error {
	args := m.Called()
	return args.Error(0)
}

// TestPromoteData_Success tests successful data promotion
func TestPromoteData_Success(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	// Setup mocks
	mockChainStore := new(MockChainStore)
	chainsDB := xsync.NewMap[string, chainstore.Store]()
	chainsDB.Store("test-chain", mockChainStore)

	// Create activity context
	activityCtx := &activity.Context{
		Logger:   logger,
		ChainsDB: chainsDB,
	}

	// Setup expectations
	mockChainStore.On("PromoteEntity", ctx, entities.Blocks, uint64(1000)).Return(nil)

	// Test input
	input := types.PromoteDataInput{
		ChainID: "test-chain",
		Entity:  "blocks",
		Height:  1000,
	}

	// Execute activity
	output, err := activityCtx.PromoteData(ctx, input)

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, "blocks", output.Entity)
	assert.Equal(t, uint64(1000), output.Height)
	assert.Greater(t, output.DurationMs, float64(0))

	mockChainStore.AssertExpectations(t)
}

// TestPromoteData_InvalidEntity tests promotion with invalid entity name
func TestPromoteData_InvalidEntity(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	// Create activity context
	activityCtx := &activity.Context{
		Logger:   logger,
		ChainsDB: xsync.NewMap[string, chainstore.Store](),
	}

	// Test input with invalid entity
	input := types.PromoteDataInput{
		ChainID: "test-chain",
		Entity:  "invalid_entity",
		Height:  1000,
	}

	// Execute activity
	_, err := activityCtx.PromoteData(ctx, input)

	// Assertions
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid entity")
}

// TestPromoteData_DatabaseError tests promotion when database operation fails
func TestPromoteData_DatabaseError(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	// Setup mocks
	mockChainStore := new(MockChainStore)
	chainsDB := xsync.NewMap[string, chainstore.Store]()
	chainsDB.Store("test-chain", mockChainStore)

	// Create activity context
	activityCtx := &activity.Context{
		Logger:   logger,
		ChainsDB: chainsDB,
	}

	// Setup expectations - promotion fails
	dbError := errors.New("database connection failed")
	mockChainStore.On("PromoteEntity", ctx, entities.Blocks, uint64(1000)).Return(dbError)

	// Test input
	input := types.PromoteDataInput{
		ChainID: "test-chain",
		Entity:  "blocks",
		Height:  1000,
	}

	// Execute activity
	_, err := activityCtx.PromoteData(ctx, input)

	// Assertions
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database connection failed")

	mockChainStore.AssertExpectations(t)
}

// TestPromoteData_ChainNotFound tests when chain database is not found
func TestPromoteData_ChainNotFound(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	// Create activity context with empty chains map
	activityCtx := &activity.Context{
		Logger:   logger,
		ChainsDB: xsync.NewMap[string, chainstore.Store](),
	}

	// Test input
	input := types.PromoteDataInput{
		ChainID: "unknown-chain",
		Entity:  "blocks",
		Height:  1000,
	}

	// Execute activity
	_, err := activityCtx.PromoteData(ctx, input)

	// Assertions
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get chain db")
}

// TestCleanPromotedData_Success tests successful staging cleanup
func TestCleanPromotedData_Success(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	// Setup mocks
	mockChainStore := new(MockChainStore)
	chainsDB := xsync.NewMap[string, chainstore.Store]()
	chainsDB.Store("test-chain", mockChainStore)

	// Create activity context
	activityCtx := &activity.Context{
		Logger:   logger,
		ChainsDB: chainsDB,
	}

	// Setup expectations
	mockChainStore.On("CleanEntityStaging", ctx, entities.Blocks, uint64(1000)).Return(nil)

	// Test input
	input := types.CleanPromotedDataInput{
		ChainID: "test-chain",
		Entity:  "blocks",
		Height:  1000,
	}

	// Execute activity
	output, err := activityCtx.CleanPromotedData(ctx, input)

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, "blocks", output.Entity)
	assert.Equal(t, uint64(1000), output.Height)
	assert.Greater(t, output.DurationMs, float64(0))

	mockChainStore.AssertExpectations(t)
}

// TestCleanPromotedData_InvalidEntity tests cleanup with invalid entity name
func TestCleanPromotedData_InvalidEntity(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	// Create activity context
	activityCtx := &activity.Context{
		Logger:   logger,
		ChainsDB: xsync.NewMap[string, chainstore.Store](),
	}

	// Test input with invalid entity
	input := types.CleanPromotedDataInput{
		ChainID: "test-chain",
		Entity:  "invalid_entity",
		Height:  1000,
	}

	// Execute activity
	_, err := activityCtx.CleanPromotedData(ctx, input)

	// Assertions
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid entity")
}

// TestCleanPromotedData_NonCriticalFailure tests that cleanup continues on failure
func TestCleanPromotedData_NonCriticalFailure(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	// Setup mocks
	mockChainStore := new(MockChainStore)
	chainsDB := xsync.NewMap[string, chainstore.Store]()
	chainsDB.Store("test-chain", mockChainStore)

	// Create activity context
	activityCtx := &activity.Context{
		Logger:   logger,
		ChainsDB: chainsDB,
	}

	// Setup expectations - cleanup fails
	cleanupError := errors.New("cleanup failed")
	mockChainStore.On("CleanEntityStaging", ctx, entities.Blocks, uint64(1000)).Return(cleanupError)

	// Test input
	input := types.CleanPromotedDataInput{
		ChainID: "test-chain",
		Entity:  "blocks",
		Height:  1000,
	}

	// Execute activity
	output, err := activityCtx.CleanPromotedData(ctx, input)

	// Assertions - should not error, just log warning
	assert.NoError(t, err, "Cleanup failure should not return error (non-critical)")
	assert.Equal(t, "blocks", output.Entity)
	assert.Equal(t, uint64(1000), output.Height)
	assert.Greater(t, output.DurationMs, float64(0))

	mockChainStore.AssertExpectations(t)
}

// TestPromoteData_AllEntities tests promotion for all defined entities
func TestPromoteData_AllEntities(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	// Test each entity
	for _, entity := range entities.All() {
		t.Run(fmt.Sprintf("Promote_%s", entity), func(t *testing.T) {
			// Setup mocks
			mockChainStore := new(MockChainStore)
			chainsDB := xsync.NewMap[string, chainstore.Store]()
			chainsDB.Store("test-chain", mockChainStore)

			// Create activity context
			activityCtx := &activity.Context{
				Logger:   logger,
				ChainsDB: chainsDB,
			}

			// Setup expectations
			mockChainStore.On("PromoteEntity", ctx, entity, uint64(2000)).Return(nil)

			// Test input
			input := types.PromoteDataInput{
				ChainID: "test-chain",
				Entity:  entity.String(),
				Height:  2000,
			}

			// Execute activity
			output, err := activityCtx.PromoteData(ctx, input)

			// Assertions
			assert.NoError(t, err)
			assert.Equal(t, entity.String(), output.Entity)
			assert.Equal(t, uint64(2000), output.Height)

			mockChainStore.AssertExpectations(t)
		})
	}
}

// TestCleanPromotedData_AllEntities tests cleanup for all defined entities
func TestCleanPromotedData_AllEntities(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	// Test each entity
	for _, entity := range entities.All() {
		t.Run(fmt.Sprintf("Clean_%s", entity), func(t *testing.T) {
			// Setup mocks
			mockChainStore := new(MockChainStore)
			chainsDB := xsync.NewMap[string, chainstore.Store]()
			chainsDB.Store("test-chain", mockChainStore)

			// Create activity context
			activityCtx := &activity.Context{
				Logger:   logger,
				ChainsDB: chainsDB,
			}

			// Setup expectations
			mockChainStore.On("CleanEntityStaging", ctx, entity, uint64(3000)).Return(nil)

			// Test input
			input := types.CleanPromotedDataInput{
				ChainID: "test-chain",
				Entity:  entity.String(),
				Height:  3000,
			}

			// Execute activity
			output, err := activityCtx.CleanPromotedData(ctx, input)

			// Assertions
			assert.NoError(t, err)
			assert.Equal(t, entity.String(), output.Entity)
			assert.Equal(t, uint64(3000), output.Height)

			mockChainStore.AssertExpectations(t)
		})
	}
}

// Benchmark tests

func BenchmarkPromoteData(b *testing.B) {
	ctx := context.Background()
	logger := zaptest.NewLogger(b)

	// Setup mocks
	mockChainStore := new(MockChainStore)
	chainsDB := xsync.NewMap[string, chainstore.Store]()
	chainsDB.Store("test-chain", mockChainStore)

	// Create activity context
	activityCtx := &activity.Context{
		Logger:   logger,
		ChainsDB: chainsDB,
	}

	// Setup expectations
	mockChainStore.On("PromoteEntity", ctx, entities.Blocks, mock.AnythingOfType("uint64")).Return(nil)

	input := types.PromoteDataInput{
		ChainID: "test-chain",
		Entity:  "blocks",
		Height:  1000,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		input.Height = uint64(1000 + i)
		_, err := activityCtx.PromoteData(ctx, input)
		require.NoError(b, err)
	}
}

func BenchmarkCleanPromotedData(b *testing.B) {
	ctx := context.Background()
	logger := zaptest.NewLogger(b)

	// Setup mocks
	mockChainStore := new(MockChainStore)
	chainsDB := xsync.NewMap[string, chainstore.Store]()
	chainsDB.Store("test-chain", mockChainStore)

	// Create activity context
	activityCtx := &activity.Context{
		Logger:   logger,
		ChainsDB: chainsDB,
	}

	// Setup expectations
	mockChainStore.On("CleanEntityStaging", ctx, entities.Blocks, mock.AnythingOfType("uint64")).Return(nil)

	input := types.CleanPromotedDataInput{
		ChainID: "test-chain",
		Entity:  "blocks",
		Height:  1000,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		input.Height = uint64(1000 + i)
		_, err := activityCtx.CleanPromotedData(ctx, input)
		require.NoError(b, err)
	}
}

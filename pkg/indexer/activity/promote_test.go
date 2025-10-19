package activity

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/db/entities"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/indexer/types"
)

// MockChainStore is a mock implementation of db.ChainStore for testing
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

func (m *MockChainStore) InsertTransactions(ctx context.Context, txs []*indexer.Transaction, raw []*indexer.TransactionRaw) error {
	args := m.Called(ctx, txs, raw)
	return args.Error(0)
}

func (m *MockChainStore) InsertBlockSummary(ctx context.Context, height uint64, blockTime time.Time, numTxs uint32) error {
	args := m.Called(ctx, height, blockTime, numTxs)
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

func (m *MockChainStore) DescribeTable(ctx context.Context, tableName string) ([]db.Column, error) {
	args := m.Called(ctx, tableName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]db.Column), args.Error(1)
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
	chainsDB := xsync.NewMap[string, db.ChainStore]()
	chainsDB.Store("test-chain", mockChainStore)

	// Create activity context
	activityCtx := &Context{
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
	activityCtx := &Context{
		Logger:   logger,
		ChainsDB: xsync.NewMap[string, db.ChainStore](),
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
	chainsDB := xsync.NewMap[string, db.ChainStore]()
	chainsDB.Store("test-chain", mockChainStore)

	// Create activity context
	activityCtx := &Context{
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
	activityCtx := &Context{
		Logger:   logger,
		ChainsDB: xsync.NewMap[string, db.ChainStore](),
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
	chainsDB := xsync.NewMap[string, db.ChainStore]()
	chainsDB.Store("test-chain", mockChainStore)

	// Create activity context
	activityCtx := &Context{
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
	activityCtx := &Context{
		Logger:   logger,
		ChainsDB: xsync.NewMap[string, db.ChainStore](),
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
	chainsDB := xsync.NewMap[string, db.ChainStore]()
	chainsDB.Store("test-chain", mockChainStore)

	// Create activity context
	activityCtx := &Context{
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
			chainsDB := xsync.NewMap[string, db.ChainStore]()
			chainsDB.Store("test-chain", mockChainStore)

			// Create activity context
			activityCtx := &Context{
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
			chainsDB := xsync.NewMap[string, db.ChainStore]()
			chainsDB.Store("test-chain", mockChainStore)

			// Create activity context
			activityCtx := &Context{
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
	chainsDB := xsync.NewMap[string, db.ChainStore]()
	chainsDB.Store("test-chain", mockChainStore)

	// Create activity context
	activityCtx := &Context{
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
	chainsDB := xsync.NewMap[string, db.ChainStore]()
	chainsDB.Store("test-chain", mockChainStore)

	// Create activity context
	activityCtx := &Context{
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
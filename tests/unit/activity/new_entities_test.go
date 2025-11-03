package activity_test

import (
	"context"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/activity"
	"github.com/canopy-network/canopyx/app/indexer/types"
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

// ======================
// IndexCommittees Tests
// ======================

func TestIndexCommittees_Success_CommitteesChanged(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := &mockNewEntitiesRPCClient{}

	// Current committees at height 100
	currentCommittees := []*rpc.RpcCommittee{
		{
			ChainID:                1,
			LastRootHeightUpdated:  95,
			LastChainHeightUpdated: 200,
			NumberOfSamples:        100,
		},
		{
			ChainID:                2,
			LastRootHeightUpdated:  98,
			LastChainHeightUpdated: 150,
			NumberOfSamples:        75,
		},
	}

	// Previous committees at height 99
	previousCommittees := []*rpc.RpcCommittee{
		{
			ChainID:                1,
			LastRootHeightUpdated:  90, // Changed
			LastChainHeightUpdated: 190,
			NumberOfSamples:        95,
		},
	}

	mockRPC.On("CommitteesData", mock.Anything, uint64(100)).Return(currentCommittees, nil)
	mockRPC.On("CommitteesData", mock.Anything, uint64(99)).Return(previousCommittees, nil)
	mockRPC.On("SubsidizedCommittees", mock.Anything, uint64(100)).Return([]uint64{1}, nil)
	mockRPC.On("SubsidizedCommittees", mock.Anything, uint64(99)).Return([]uint64{}, nil)
	mockRPC.On("RetiredCommittees", mock.Anything, uint64(100)).Return([]uint64{}, nil)
	mockRPC.On("RetiredCommittees", mock.Anything, uint64(99)).Return([]uint64{}, nil)

	mockChainStore := &mockNewEntitiesChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	chainsMap := xsync.NewMap[string, *mockNewEntitiesChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexCommittees)

	input := types.IndexCommitteesInput{
		ChainID:   "chain-A",
		Height:    100,
		BlockTime: time.Now().UTC(),
	}

	future, err := env.ExecuteActivity(activityCtx.IndexCommittees, input)
	require.NoError(t, err)

	var output types.IndexCommitteesOutput
	require.NoError(t, future.Get(&output))

	// 2 committees changed (committee 1 changed, committee 2 is new)
	assert.Equal(t, uint32(2), output.NumCommittees)
	assert.Greater(t, output.DurationMs, 0.0)
	require.Len(t, mockChainStore.insertedCommittees, 2)

	// Verify committee 1 is marked as subsidized
	var committee1 *indexermodels.Committee
	for _, c := range mockChainStore.insertedCommittees {
		if c.ChainID == 1 {
			committee1 = c
			break
		}
	}
	require.NotNil(t, committee1)
	assert.True(t, committee1.Subsidized)

	mockRPC.AssertExpectations(t)
}

func TestIndexCommittees_GenesisBlock(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := &mockNewEntitiesRPCClient{}

	genesisCommittees := []*rpc.RpcCommittee{
		{
			ChainID:                1,
			LastRootHeightUpdated:  0,
			LastChainHeightUpdated: 0,
			NumberOfSamples:        10,
		},
	}

	mockRPC.On("CommitteesData", mock.Anything, uint64(1)).Return(genesisCommittees, nil)
	mockRPC.On("SubsidizedCommittees", mock.Anything, uint64(1)).Return([]uint64{}, nil)
	mockRPC.On("RetiredCommittees", mock.Anything, uint64(1)).Return([]uint64{}, nil)

	mockChainStore := &mockNewEntitiesChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	chainsMap := xsync.NewMap[string, *mockNewEntitiesChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexCommittees)

	input := types.IndexCommitteesInput{
		ChainID:   "chain-A",
		Height:    1,
		BlockTime: time.Now().UTC(),
	}

	future, err := env.ExecuteActivity(activityCtx.IndexCommittees, input)
	require.NoError(t, err)

	var output types.IndexCommitteesOutput
	require.NoError(t, future.Get(&output))

	// All committees inserted at genesis
	assert.Equal(t, uint32(1), output.NumCommittees)
	assert.Equal(t, uint64(1), mockChainStore.insertedCommittees[0].Height)

	mockRPC.AssertExpectations(t)
}

// ======================
// IndexPoll Tests
// ======================

func TestIndexPoll_Success_WithProposals(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := &mockNewEntitiesRPCClient{}

	pollResults := map[string]*rpc.RpcPollResult{
		"proposal123": {
			ProposalURL: "https://gov.example.com/proposal123",
			Accounts: rpc.RpcPollCategory{
				ApproveTokens:     1000000,
				RejectTokens:      500000,
				TotalVotedTokens:  1500000,
				TotalTokens:       2000000,
				ApprovePercentage: 66.67,
				RejectPercentage:  33.33,
				VotedPercentage:   75.0,
			},
			Validators: rpc.RpcPollCategory{
				ApproveTokens:     5000000,
				RejectTokens:      2000000,
				TotalVotedTokens:  7000000,
				TotalTokens:       10000000,
				ApprovePercentage: 71.43,
				RejectPercentage:  28.57,
				VotedPercentage:   70.0,
			},
		},
		"proposal456": {
			ProposalURL: "https://gov.example.com/proposal456",
			Accounts: rpc.RpcPollCategory{
				ApproveTokens:     2000000,
				RejectTokens:      1000000,
				TotalVotedTokens:  3000000,
				TotalTokens:       5000000,
				ApprovePercentage: 66.67,
				RejectPercentage:  33.33,
				VotedPercentage:   60.0,
			},
			Validators: rpc.RpcPollCategory{
				ApproveTokens:     8000000,
				RejectTokens:      3000000,
				TotalVotedTokens:  11000000,
				TotalTokens:       15000000,
				ApprovePercentage: 72.73,
				RejectPercentage:  27.27,
				VotedPercentage:   73.33,
			},
		},
	}

	mockRPC.On("Poll", mock.Anything).Return(pollResults, nil)

	mockChainStore := &mockNewEntitiesChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	chainsMap := xsync.NewMap[string, *mockNewEntitiesChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexPoll)

	input := types.IndexPollInput{
		ChainID:   "chain-A",
		Height:    100,
		BlockTime: time.Now().UTC(),
	}

	future, err := env.ExecuteActivity(activityCtx.IndexPoll, input)
	require.NoError(t, err)

	var output types.IndexPollOutput
	require.NoError(t, future.Get(&output))

	assert.Equal(t, uint32(2), output.NumProposals)
	assert.Greater(t, output.DurationMs, 0.0)
	require.Len(t, mockChainStore.insertedPollSnapshots, 2)

	// Verify proposal data
	var proposal123 *indexermodels.PollSnapshot
	for _, p := range mockChainStore.insertedPollSnapshots {
		if p.ProposalHash == "proposal123" {
			proposal123 = p
			break
		}
	}
	require.NotNil(t, proposal123)
	assert.Equal(t, "https://gov.example.com/proposal123", proposal123.ProposalURL)
	assert.Equal(t, uint64(1000000), proposal123.AccountsApproveTokens)
	assert.Equal(t, 66.67, proposal123.AccountsApprovePercentage)

	mockRPC.AssertExpectations(t)
}

func TestIndexPoll_EmptyPoll(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := &mockNewEntitiesRPCClient{}
	mockRPC.On("Poll", mock.Anything).Return(map[string]*rpc.RpcPollResult{}, nil)

	mockChainStore := &mockNewEntitiesChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	chainsMap := xsync.NewMap[string, *mockNewEntitiesChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexPoll)

	input := types.IndexPollInput{
		ChainID:   "chain-A",
		Height:    100,
		BlockTime: time.Now().UTC(),
	}

	future, err := env.ExecuteActivity(activityCtx.IndexPoll, input)
	require.NoError(t, err)

	var output types.IndexPollOutput
	require.NoError(t, future.Get(&output))

	// No active proposals
	assert.Equal(t, uint32(0), output.NumProposals)
	assert.Empty(t, mockChainStore.insertedPollSnapshots)

	mockRPC.AssertExpectations(t)
}

// ======================
// IndexDexBatch Tests
// ======================

func TestIndexDexBatch_Success_WithOrders(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := &mockNewEntitiesRPCClient{}

	currentBatch := &rpc.RpcDexBatch{
		LockedHeight: 95,
		Orders: []*rpc.RpcDexOrder{
			{
				OrderID:         "order1",
				Address:         "0xaddr1",
				AmountForSale:   1000,
				RequestedAmount: 500,
			},
		},
		Deposits: []*rpc.RpcDexDeposit{
			{
				OrderID: "deposit1",
				Address: "0xaddr2",
				Amount:  2000,
			},
		},
		Withdrawals: []*rpc.RpcDexWithdrawal{
			{
				OrderID: "withdrawal1",
				Address: "0xaddr3",
				Percent: 50,
			},
		},
	}

	nextBatch := &rpc.RpcDexBatch{
		LockedHeight: 0,
		Orders: []*rpc.RpcDexOrder{
			{
				OrderID:         "order2",
				Address:         "0xaddr4",
				AmountForSale:   3000,
				RequestedAmount: 1500,
			},
		},
		Deposits:    []*rpc.RpcDexDeposit{},
		Withdrawals: []*rpc.RpcWithdrawal{},
	}

	mockRPC.On("DexBatchByHeight", mock.Anything, uint64(100), uint64(1)).Return(currentBatch, nil)
	mockRPC.On("NextDexBatchByHeight", mock.Anything, uint64(100), uint64(1)).Return(nextBatch, nil)

	mockChainStore := &mockNewEntitiesChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
		eventsByType: map[string][]*indexermodels.Event{},
	}

	chainsMap := xsync.NewMap[string, *mockNewEntitiesChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexDexBatch)

	input := types.IndexDexBatchInput{
		ChainID:   "chain-A",
		Height:    100,
		Committee: 1,
		BlockTime: time.Now().UTC(),
	}

	future, err := env.ExecuteActivity(activityCtx.IndexDexBatch, input)
	require.NoError(t, err)

	var output types.IndexDexBatchOutput
	require.NoError(t, future.Get(&output))

	assert.Equal(t, uint32(2), output.NumOrders)      // 1 locked + 1 future
	assert.Equal(t, uint32(1), output.NumDeposits)    // 1 deposit
	assert.Equal(t, uint32(1), output.NumWithdrawals) // 1 withdrawal
	assert.Greater(t, output.DurationMs, 0.0)

	mockRPC.AssertExpectations(t)
}

func TestIndexDexBatch_NoBatch(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := &mockNewEntitiesRPCClient{}
	mockRPC.On("DexBatchByHeight", mock.Anything, uint64(100), uint64(1)).Return((*rpc.RpcDexBatch)(nil), nil)
	mockRPC.On("NextDexBatchByHeight", mock.Anything, uint64(100), uint64(1)).Return((*rpc.RpcDexBatch)(nil), nil)

	mockChainStore := &mockNewEntitiesChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
		eventsByType: map[string][]*indexermodels.Event{},
	}

	chainsMap := xsync.NewMap[string, *mockNewEntitiesChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexDexBatch)

	input := types.IndexDexBatchInput{
		ChainID:   "chain-A",
		Height:    100,
		Committee: 1,
		BlockTime: time.Now().UTC(),
	}

	future, err := env.ExecuteActivity(activityCtx.IndexDexBatch, input)
	require.NoError(t, err)

	var output types.IndexDexBatchOutput
	require.NoError(t, future.Get(&output))

	// No batch means no entities
	assert.Equal(t, uint32(0), output.NumOrders)
	assert.Equal(t, uint32(0), output.NumDeposits)
	assert.Equal(t, uint32(0), output.NumWithdrawals)

	mockRPC.AssertExpectations(t)
}

// ======================
// IndexDexPoolPoints Tests
// ======================

func TestIndexDexPoolPoints_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := &mockNewEntitiesRPCClient{}

	pools := []*rpc.RpcPool{
		{
			ID:              1,
			ChainID:         2,
			Amount:          10000,
			TotalPoolPoints: 1000,
			Points: []*rpc.RpcPoolPoint{
				{Address: "0xholder1", Points: 600}, // 60% of pool
				{Address: "0xholder2", Points: 400}, // 40% of pool
			},
		},
		{
			ID:              2,
			ChainID:         3,
			Amount:          5000,
			TotalPoolPoints: 500,
			Points: []*rpc.RpcPoolPoint{
				{Address: "0xholder3", Points: 500}, // 100% of pool
			},
		},
	}

	mockRPC.On("Pools", mock.Anything).Return(pools, nil)

	mockChainStore := &mockNewEntitiesChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	chainsMap := xsync.NewMap[string, *mockNewEntitiesChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexDexPoolPoints)

	input := types.IndexDexPoolPointsInput{
		ChainID:   "chain-A",
		Height:    100,
		BlockTime: time.Now().UTC(),
	}

	future, err := env.ExecuteActivity(activityCtx.IndexDexPoolPoints, input)
	require.NoError(t, err)

	var output types.IndexDexPoolPointsOutput
	require.NoError(t, future.Get(&output))

	assert.Equal(t, uint32(3), output.NumHolders) // 3 holders across 2 pools
	assert.Greater(t, output.DurationMs, 0.0)
	require.Len(t, mockChainStore.insertedPoolPointsByHolder, 3)

	// Verify liquidity calculations
	var holder1 *indexermodels.DexPoolPointsByHolder
	for _, h := range mockChainStore.insertedPoolPointsByHolder {
		if h.Address == "0xholder1" {
			holder1 = h
			break
		}
	}
	require.NotNil(t, holder1)
	// holder1 has 600 points out of 1000 total, pool has 10000 amount
	// liquidity = 10000 * 600 / 1000 = 6000
	assert.Equal(t, uint64(6000), holder1.LiquidityPoolPoints)

	mockRPC.AssertExpectations(t)
}

func TestIndexDexPoolPoints_EmptyPools(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := &mockNewEntitiesRPCClient{}
	mockRPC.On("Pools", mock.Anything).Return([]*rpc.RpcPool{}, nil)

	mockChainStore := &mockNewEntitiesChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	chainsMap := xsync.NewMap[string, *mockNewEntitiesChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexDexPoolPoints)

	input := types.IndexDexPoolPointsInput{
		ChainID:   "chain-A",
		Height:    100,
		BlockTime: time.Now().UTC(),
	}

	future, err := env.ExecuteActivity(activityCtx.IndexDexPoolPoints, input)
	require.NoError(t, err)

	var output types.IndexDexPoolPointsOutput
	require.NoError(t, future.Get(&output))

	assert.Equal(t, uint32(0), output.NumHolders)
	assert.Empty(t, mockChainStore.insertedPoolPointsByHolder)

	mockRPC.AssertExpectations(t)
}

// ======================
// Mock Implementations
// ======================

type mockNewEntitiesRPCClient struct {
	mock.Mock
}

func (m *mockNewEntitiesRPCClient) ChainHead(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (m *mockNewEntitiesRPCClient) BlockByHeight(ctx context.Context, height uint64) (*indexermodels.Block, error) {
	return nil, nil
}

func (m *mockNewEntitiesRPCClient) TxsByHeight(ctx context.Context, height uint64) ([]*indexermodels.Transaction, error) {
	return nil, nil
}

func (m *mockNewEntitiesRPCClient) AccountsByHeight(ctx context.Context, height uint64) ([]*rpc.Account, error) {
	return nil, nil
}

func (m *mockNewEntitiesRPCClient) GetGenesisState(ctx context.Context, height uint64) (*rpc.GenesisState, error) {
	return nil, nil
}

func (m *mockNewEntitiesRPCClient) EventsByHeight(ctx context.Context, height uint64) ([]*indexermodels.Event, error) {
	return nil, nil
}

func (m *mockNewEntitiesRPCClient) OrdersByHeight(ctx context.Context, height uint64, chainID uint64) ([]*rpc.RpcOrder, error) {
	return nil, nil
}

func (m *mockNewEntitiesRPCClient) DexPrice(ctx context.Context, chainID uint64) (*indexermodels.DexPrice, error) {
	return nil, nil
}

func (m *mockNewEntitiesRPCClient) DexPrices(ctx context.Context) ([]*indexermodels.DexPrice, error) {
	return nil, nil
}

func (m *mockNewEntitiesRPCClient) PoolByID(ctx context.Context, id uint64) (*rpc.RpcPool, error) {
	return nil, nil
}

func (m *mockNewEntitiesRPCClient) Pools(ctx context.Context) ([]*rpc.RpcPool, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*rpc.RpcPool), args.Error(1)
}

func (m *mockNewEntitiesRPCClient) AllParams(ctx context.Context, height uint64) (*rpc.RpcAllParams, error) {
	return nil, nil
}

func (m *mockNewEntitiesRPCClient) Validators(ctx context.Context, height uint64) ([]*rpc.RpcValidator, error) {
	return nil, nil
}

func (m *mockNewEntitiesRPCClient) NonSigners(ctx context.Context, height uint64) ([]*rpc.RpcNonSigner, error) {
	return nil, nil
}

func (m *mockNewEntitiesRPCClient) ValParams(ctx context.Context, height uint64) (*rpc.ValidatorParams, error) {
	return nil, nil
}

func (m *mockNewEntitiesRPCClient) CommitteesData(ctx context.Context, height uint64) ([]*rpc.RpcCommittee, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*rpc.RpcCommittee), args.Error(1)
}

func (m *mockNewEntitiesRPCClient) SubsidizedCommittees(ctx context.Context, height uint64) ([]uint64, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]uint64), args.Error(1)
}

func (m *mockNewEntitiesRPCClient) RetiredCommittees(ctx context.Context, height uint64) ([]uint64, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]uint64), args.Error(1)
}

func (m *mockNewEntitiesRPCClient) Poll(ctx context.Context) (map[string]*rpc.RpcPollResult, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]*rpc.RpcPollResult), args.Error(1)
}

func (m *mockNewEntitiesRPCClient) DexBatchByHeight(ctx context.Context, height uint64, committee uint64) (*rpc.RpcDexBatch, error) {
	args := m.Called(ctx, height, committee)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rpc.RpcDexBatch), args.Error(1)
}

func (m *mockNewEntitiesRPCClient) NextDexBatchByHeight(ctx context.Context, height uint64, committee uint64) (*rpc.RpcDexBatch, error) {
	args := m.Called(ctx, height, committee)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rpc.RpcDexBatch), args.Error(1)
}

type mockNewEntitiesChainStore struct {
	chainID                    string
	databaseName               string
	insertedCommittees         []*indexermodels.Committee
	insertedPollSnapshots      []*indexermodels.PollSnapshot
	insertedDexOrders          []*indexermodels.DexOrder
	insertedDexDeposits        []*indexermodels.DexDeposit
	insertedDexWithdrawals     []*indexermodels.DexWithdrawal
	insertedPoolPointsByHolder []*indexermodels.DexPoolPointsByHolder
	eventsByType               map[string][]*indexermodels.Event
}

func (m *mockNewEntitiesChainStore) DatabaseName() string { return m.databaseName }
func (m *mockNewEntitiesChainStore) ChainKey() string     { return m.chainID }

func (m *mockNewEntitiesChainStore) InsertCommitteesStaging(ctx context.Context, committees []*indexermodels.Committee) error {
	m.insertedCommittees = append(m.insertedCommittees, committees...)
	return nil
}

func (m *mockNewEntitiesChainStore) InsertPollSnapshotsStaging(ctx context.Context, snapshots []*indexermodels.PollSnapshot) error {
	m.insertedPollSnapshots = append(m.insertedPollSnapshots, snapshots...)
	return nil
}

func (m *mockNewEntitiesChainStore) InsertDexOrdersStaging(ctx context.Context, orders []*indexermodels.DexOrder) error {
	m.insertedDexOrders = append(m.insertedDexOrders, orders...)
	return nil
}

func (m *mockNewEntitiesChainStore) InsertDexDepositsStaging(ctx context.Context, deposits []*indexermodels.DexDeposit) error {
	m.insertedDexDeposits = append(m.insertedDexDeposits, deposits...)
	return nil
}

func (m *mockNewEntitiesChainStore) InsertDexWithdrawalsStaging(ctx context.Context, withdrawals []*indexermodels.DexWithdrawal) error {
	m.insertedDexWithdrawals = append(m.insertedDexWithdrawals, withdrawals...)
	return nil
}

func (m *mockNewEntitiesChainStore) InsertDexPoolPointsByHolderStaging(ctx context.Context, holders []*indexermodels.DexPoolPointsByHolder) error {
	m.insertedPoolPointsByHolder = append(m.insertedPoolPointsByHolder, holders...)
	return nil
}

func (m *mockNewEntitiesChainStore) GetEventsByTypeAndHeight(ctx context.Context, height uint64, eventTypes ...string) ([]*indexermodels.Event, error) {
	var result []*indexermodels.Event
	for _, eventType := range eventTypes {
		if events, ok := m.eventsByType[eventType]; ok {
			result = append(result, events...)
		}
	}
	return result, nil
}

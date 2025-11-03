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

// TestIndexParams_Success_ParamsChanged tests successful indexing when params changed
func TestIndexParams_Success_ParamsChanged(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Setup mock admin store
	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	// Setup mock RPC client
	mockRPC := &mockParamsRPCClient{}

	// Current params (height 100)
	currentParams := &rpc.RpcAllParams{
		ConsensusParams: &rpc.RpcConsensusParams{
			BlockSize:       1000,
			ProtocolVersion: 2,
			RootChainID:     1,
			Retired:         false,
		},
		ValidatorParams: &rpc.ValidatorParams{
			UnstakingBlocks:           21000,
			MaxPauseBlocks:            1000,
			DoubleSignSlashPercentage: 10,
			NonSignSlashPercentage:    5,
			MaxNonSign:                100,
			NonSignWindow:             10000,
			MaxCommittees:             50,
			MaxCommitteeSize:          100,
			EarlyWithdrawalPenalty:    10,
			DelegateUnstakingBlocks:   21000,
			MinimumOrderSize:          1000,
			StakePercentForSubsidized: 50,
			MaxSlashPerCommittee:      10,
			DelegateRewardPercentage:  10,
			BuyDeadlineBlocks:         1000,
			LockOrderFeeMultiplier:    2,
		},
		FeeParams: &rpc.RpcFeeParams{
			SendFee:                 100,
			StakeFee:                200,
			EditStakeFee:            150,
			UnstakeFee:              100,
			PauseFee:                50,
			UnpauseFee:              50,
			ChangeParameterFee:      1000,
			DaoTransferFee:          500,
			CertificateResultsFee:   300,
			SubsidyFee:              200,
			CreateOrderFee:          100,
			EditOrderFee:            50,
			DeleteOrderFee:          50,
			DexLimitOrderFee:        100,
			DexLiquidityDepositFee:  200,
			DexLiquidityWithdrawFee: 200,
		},
		GovParams: &rpc.RpcGovParams{
			DaoRewardPercentage: 10,
		},
	}

	// Previous params (height 99) - different BlockSize
	previousParams := &rpc.RpcAllParams{
		ConsensusParams: &rpc.RpcConsensusParams{
			BlockSize:       800, // Changed!
			ProtocolVersion: 2,
			RootChainID:     1,
			Retired:         false,
		},
		ValidatorParams: currentParams.ValidatorParams,
		FeeParams:       currentParams.FeeParams,
		GovParams:       currentParams.GovParams,
	}

	mockRPC.On("AllParams", mock.Anything, uint64(100)).Return(currentParams, nil)
	mockRPC.On("AllParams", mock.Anything, uint64(99)).Return(previousParams, nil)

	// Setup mock chain store
	mockChainStore := &mockParamsChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	// Setup chains map
	chainsMap := xsync.NewMap[string, *mockParamsChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	// Create activity context
	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	// Setup temporal test suite
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexParams)

	// Execute activity
	input := types.IndexParamsInput{
		ChainID:   "chain-A",
		Height:    100,
		BlockTime: time.Now().UTC(),
	}

	future, err := env.ExecuteActivity(activityCtx.IndexParams, input)
	require.NoError(t, err)

	// Get output
	var output types.IndexParamsOutput
	require.NoError(t, future.Get(&output))

	// Assertions
	assert.True(t, output.ParamsChanged, "Params should have changed")
	assert.Greater(t, output.DurationMs, 0.0)
	require.Len(t, mockChainStore.insertedParams, 1)
	assert.Equal(t, uint64(1000), mockChainStore.insertedParams[0].BlockSize)

	mockRPC.AssertExpectations(t)
}

// TestIndexParams_Success_NoChanges tests when params haven't changed
func TestIndexParams_Success_NoChanges(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := &mockParamsRPCClient{}

	// Identical params at both heights
	sameParams := &rpc.RpcAllParams{
		ConsensusParams: &rpc.RpcConsensusParams{
			BlockSize:       1000,
			ProtocolVersion: 2,
			RootChainID:     1,
			Retired:         false,
		},
		ValidatorParams: &rpc.ValidatorParams{
			UnstakingBlocks: 21000,
			MaxPauseBlocks:  1000,
		},
		FeeParams: &rpc.RpcFeeParams{
			SendFee:  100,
			StakeFee: 200,
		},
		GovParams: &rpc.RpcGovParams{
			DaoRewardPercentage: 10,
		},
	}

	mockRPC.On("AllParams", mock.Anything, uint64(100)).Return(sameParams, nil)
	mockRPC.On("AllParams", mock.Anything, uint64(99)).Return(sameParams, nil)

	mockChainStore := &mockParamsChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	chainsMap := xsync.NewMap[string, *mockParamsChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexParams)

	input := types.IndexParamsInput{
		ChainID:   "chain-A",
		Height:    100,
		BlockTime: time.Now().UTC(),
	}

	future, err := env.ExecuteActivity(activityCtx.IndexParams, input)
	require.NoError(t, err)

	var output types.IndexParamsOutput
	require.NoError(t, future.Get(&output))

	// Params should not have changed
	assert.False(t, output.ParamsChanged)
	assert.GreaterOrEqual(t, output.DurationMs, 0.0)
	assert.Empty(t, mockChainStore.insertedParams, "No params should be inserted when unchanged")

	mockRPC.AssertExpectations(t)
}

// TestIndexParams_GenesisBlock tests height 1 always inserts params
func TestIndexParams_GenesisBlock(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := &mockParamsRPCClient{}

	genesisParams := &rpc.RpcAllParams{
		ConsensusParams: &rpc.RpcConsensusParams{
			BlockSize:       1000,
			ProtocolVersion: 1,
			RootChainID:     1,
			Retired:         false,
		},
		ValidatorParams: &rpc.ValidatorParams{
			UnstakingBlocks: 21000,
		},
		FeeParams: &rpc.RpcFeeParams{
			SendFee: 100,
		},
		GovParams: &rpc.RpcGovParams{
			DaoRewardPercentage: 10,
		},
	}

	mockRPC.On("AllParams", mock.Anything, uint64(1)).Return(genesisParams, nil)

	mockChainStore := &mockParamsChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	chainsMap := xsync.NewMap[string, *mockParamsChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexParams)

	input := types.IndexParamsInput{
		ChainID:   "chain-A",
		Height:    1,
		BlockTime: time.Now().UTC(),
	}

	future, err := env.ExecuteActivity(activityCtx.IndexParams, input)
	require.NoError(t, err)

	var output types.IndexParamsOutput
	require.NoError(t, future.Get(&output))

	// Genesis block should always insert params
	assert.True(t, output.ParamsChanged)
	assert.Greater(t, output.DurationMs, 0.0)
	require.Len(t, mockChainStore.insertedParams, 1)
	assert.Equal(t, uint64(1), mockChainStore.insertedParams[0].Height)

	mockRPC.AssertExpectations(t)
}

// TestIndexParams_RPCError tests handling of RPC errors
func TestIndexParams_RPCError(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := &mockParamsRPCClient{}
	mockRPC.On("AllParams", mock.Anything, uint64(100)).Return((*rpc.RpcAllParams)(nil), assert.AnError)

	mockChainStore := &mockParamsChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	chainsMap := xsync.NewMap[string, *mockParamsChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexParams)

	input := types.IndexParamsInput{
		ChainID:   "chain-A",
		Height:    100,
		BlockTime: time.Now().UTC(),
	}

	future, execErr := env.ExecuteActivity(activityCtx.IndexParams, input)

	var output types.IndexParamsOutput
	var actErr error
	if execErr != nil {
		actErr = execErr
	} else {
		actErr = future.Get(&output)
	}

	// Should have an error
	assert.Error(t, actErr)
	assert.Contains(t, actErr.Error(), "fetch params at height 100")

	mockRPC.AssertExpectations(t)
}

// Mock implementations

type mockParamsRPCClient struct {
	mock.Mock
}

func (m *mockParamsRPCClient) ChainHead(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (m *mockParamsRPCClient) BlockByHeight(ctx context.Context, height uint64) (*indexermodels.Block, error) {
	return nil, nil
}

func (m *mockParamsRPCClient) TxsByHeight(ctx context.Context, height uint64) ([]*indexermodels.Transaction, error) {
	return nil, nil
}

func (m *mockParamsRPCClient) AccountsByHeight(ctx context.Context, height uint64) ([]*rpc.Account, error) {
	return nil, nil
}

func (m *mockParamsRPCClient) GetGenesisState(ctx context.Context, height uint64) (*rpc.GenesisState, error) {
	return nil, nil
}

func (m *mockParamsRPCClient) EventsByHeight(ctx context.Context, height uint64) ([]*indexermodels.Event, error) {
	return nil, nil
}

func (m *mockParamsRPCClient) OrdersByHeight(ctx context.Context, height uint64, chainID uint64) ([]*rpc.RpcOrder, error) {
	return nil, nil
}

func (m *mockParamsRPCClient) DexPrice(ctx context.Context, chainID uint64) (*indexermodels.DexPrice, error) {
	return nil, nil
}

func (m *mockParamsRPCClient) DexPrices(ctx context.Context) ([]*indexermodels.DexPrice, error) {
	return nil, nil
}

func (m *mockParamsRPCClient) PoolByID(ctx context.Context, id uint64) (*rpc.RpcPool, error) {
	return nil, nil
}

func (m *mockParamsRPCClient) Pools(ctx context.Context) ([]*rpc.RpcPool, error) {
	return nil, nil
}

func (m *mockParamsRPCClient) AllParams(ctx context.Context, height uint64) (*rpc.RpcAllParams, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rpc.RpcAllParams), args.Error(1)
}

func (m *mockParamsRPCClient) Validators(ctx context.Context, height uint64) ([]*rpc.RpcValidator, error) {
	return nil, nil
}

func (m *mockParamsRPCClient) NonSigners(ctx context.Context, height uint64) ([]*rpc.RpcNonSigner, error) {
	return nil, nil
}

func (m *mockParamsRPCClient) ValParams(ctx context.Context, height uint64) (*rpc.ValidatorParams, error) {
	return nil, nil
}

func (m *mockParamsRPCClient) CommitteesData(ctx context.Context, height uint64) ([]*rpc.RpcCommittee, error) {
	return nil, nil
}

func (m *mockParamsRPCClient) SubsidizedCommittees(ctx context.Context, height uint64) ([]uint64, error) {
	return nil, nil
}

func (m *mockParamsRPCClient) RetiredCommittees(ctx context.Context, height uint64) ([]uint64, error) {
	return nil, nil
}

func (m *mockParamsRPCClient) Poll(ctx context.Context) (map[string]*rpc.RpcPollResult, error) {
	return nil, nil
}

func (m *mockParamsRPCClient) DexBatchByHeight(ctx context.Context, height uint64, committee uint64) (*rpc.RpcDexBatch, error) {
	return nil, nil
}

func (m *mockParamsRPCClient) NextDexBatchByHeight(ctx context.Context, height uint64, committee uint64) (*rpc.RpcDexBatch, error) {
	return nil, nil
}

type mockParamsChainStore struct {
	chainID        string
	databaseName   string
	insertedParams []*indexermodels.Params
}

func (m *mockParamsChainStore) DatabaseName() string { return m.databaseName }
func (m *mockParamsChainStore) ChainKey() string     { return m.chainID }

func (m *mockParamsChainStore) InsertParamsStaging(ctx context.Context, params *indexermodels.Params) error {
	m.insertedParams = append(m.insertedParams, params)
	return nil
}

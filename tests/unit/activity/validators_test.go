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

// TestIndexValidators_Success_ValidatorsChanged tests successful indexing with validator changes
func TestIndexValidators_Success_ValidatorsChanged(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := &mockValidatorsRPCClient{}

	// Current validators at height 100
	currentValidators := []*rpc.RpcValidator{
		{
			Address:         "0x111",
			PublicKey:       "pk111",
			NetAddress:      "tcp://1.1.1.1:26656",
			StakedAmount:    2000, // Changed from 1000
			Output:          "0xout111",
			Committees:      []uint64{1, 2},
			MaxPausedHeight: 0,
			UnstakingHeight: 0,
			Delegate:        "0xdelegate111",
			Compound:        true,
		},
		{
			Address:         "0x222",
			PublicKey:       "pk222",
			NetAddress:      "tcp://2.2.2.2:26656",
			StakedAmount:    1500,
			Output:          "0xout222",
			Committees:      []uint64{1},
			MaxPausedHeight: 0,
			UnstakingHeight: 0,
			Delegate:        "",
			Compound:        false,
		},
		{
			Address:         "0x333", // New validator
			PublicKey:       "pk333",
			NetAddress:      "tcp://3.3.3.3:26656",
			StakedAmount:    3000,
			Output:          "0xout333",
			Committees:      []uint64{2},
			MaxPausedHeight: 0,
			UnstakingHeight: 0,
			Delegate:        "",
			Compound:        false,
		},
	}

	// Previous validators at height 99
	previousValidators := []*rpc.RpcValidator{
		{
			Address:         "0x111",
			PublicKey:       "pk111",
			NetAddress:      "tcp://1.1.1.1:26656",
			StakedAmount:    1000, // Will change
			Output:          "0xout111",
			Committees:      []uint64{1, 2},
			MaxPausedHeight: 0,
			UnstakingHeight: 0,
			Delegate:        "0xdelegate111",
			Compound:        true,
		},
		{
			Address:         "0x222",
			PublicKey:       "pk222",
			NetAddress:      "tcp://2.2.2.2:26656",
			StakedAmount:    1500, // No change
			Output:          "0xout222",
			Committees:      []uint64{1},
			MaxPausedHeight: 0,
			UnstakingHeight: 0,
			Delegate:        "",
			Compound:        false,
		},
	}

	// Non-signers data
	currentNonSigners := []*rpc.RpcNonSigner{
		{Address: "0x111", Counter: 5},
	}
	previousNonSigners := []*rpc.RpcNonSigner{
		{Address: "0x111", Counter: 3}, // Counter increased
	}

	mockRPC.On("Validators", mock.Anything, uint64(100)).Return(currentValidators, nil)
	mockRPC.On("Validators", mock.Anything, uint64(99)).Return(previousValidators, nil)
	mockRPC.On("NonSigners", mock.Anything, uint64(100)).Return(currentNonSigners, nil)
	mockRPC.On("NonSigners", mock.Anything, uint64(99)).Return(previousNonSigners, nil)
	mockRPC.On("ValParams", mock.Anything, uint64(100)).Return(&rpc.RpcValidatorParams{
		NonSignWindow: 10000,
	}, nil)

	mockChainStore := &mockValidatorsChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	chainsMap := xsync.NewMap[string, *mockValidatorsChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexValidators)

	input := types.IndexValidatorsInput{
		ChainID:   "chain-A",
		Height:    100,
		BlockTime: time.Now().UTC(),
	}

	future, err := env.ExecuteActivity(activityCtx.IndexValidators, input)
	require.NoError(t, err)

	var output types.IndexValidatorsOutput
	require.NoError(t, future.Get(&output))

	// Assertions - 2 validators changed (0x111 stake changed, 0x333 is new)
	assert.Equal(t, uint32(2), output.NumValidators)
	assert.Equal(t, uint32(1), output.NumSigningInfos) // 0x111 counter changed
	assert.Greater(t, output.DurationMs, 0.0)

	// Verify inserted data
	require.Len(t, mockChainStore.insertedValidators, 2)
	require.Len(t, mockChainStore.insertedSigningInfos, 1)

	// Check validator 0x111 was updated
	found111 := false
	for _, v := range mockChainStore.insertedValidators {
		if v.Address == "0x111" {
			found111 = true
			assert.Equal(t, uint64(2000), v.StakedAmount)
			assert.Equal(t, uint64(100), v.Height)
		}
	}
	assert.True(t, found111, "Validator 0x111 should be in inserted validators")

	// Check signing info was updated
	assert.Equal(t, "0x111", mockChainStore.insertedSigningInfos[0].Address)
	assert.Equal(t, uint64(5), mockChainStore.insertedSigningInfos[0].MissedBlocksCount)

	// Check committee-validator junction records
	assert.GreaterOrEqual(t, len(mockChainStore.insertedCommitteeValidators), 2)

	mockRPC.AssertExpectations(t)
}

// TestIndexValidators_GenesisBlock tests height 1 always inserts all validators
func TestIndexValidators_GenesisBlock(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := &mockValidatorsRPCClient{}

	genesisValidators := []*rpc.RpcValidator{
		{
			Address:         "0xgenesis1",
			PublicKey:       "pkgenesis1",
			NetAddress:      "tcp://1.1.1.1:26656",
			StakedAmount:    10000,
			Output:          "0xout1",
			Committees:      []uint64{1},
			MaxPausedHeight: 0,
			UnstakingHeight: 0,
			Delegate:        "",
			Compound:        false,
		},
	}

	mockRPC.On("Validators", mock.Anything, uint64(1)).Return(genesisValidators, nil)
	// No previous validators for genesis
	mockRPC.On("NonSigners", mock.Anything, uint64(1)).Return([]*rpc.RpcNonSigner{}, nil)

	mockChainStore := &mockValidatorsChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	chainsMap := xsync.NewMap[string, *mockValidatorsChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexValidators)

	input := types.IndexValidatorsInput{
		ChainID:   "chain-A",
		Height:    1,
		BlockTime: time.Now().UTC(),
	}

	future, err := env.ExecuteActivity(activityCtx.IndexValidators, input)
	require.NoError(t, err)

	var output types.IndexValidatorsOutput
	require.NoError(t, future.Get(&output))

	// All validators should be inserted at genesis
	assert.Equal(t, uint32(1), output.NumValidators)
	assert.Equal(t, uint64(1), mockChainStore.insertedValidators[0].Height)

	mockRPC.AssertExpectations(t)
}

// TestIndexValidators_NoChanges tests when no validators have changed
func TestIndexValidators_NoChanges(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := &mockValidatorsRPCClient{}

	// Identical validators at both heights
	sameValidators := []*rpc.RpcValidator{
		{
			Address:         "0x111",
			PublicKey:       "pk111",
			NetAddress:      "tcp://1.1.1.1:26656",
			StakedAmount:    1000,
			Output:          "0xout111",
			Committees:      []uint64{1},
			MaxPausedHeight: 0,
			UnstakingHeight: 0,
			Delegate:        "",
			Compound:        false,
		},
	}

	sameNonSigners := []*rpc.RpcNonSigner{
		{Address: "0x111", Counter: 5},
	}

	mockRPC.On("Validators", mock.Anything, uint64(100)).Return(sameValidators, nil)
	mockRPC.On("Validators", mock.Anything, uint64(99)).Return(sameValidators, nil)
	mockRPC.On("NonSigners", mock.Anything, uint64(100)).Return(sameNonSigners, nil)
	mockRPC.On("NonSigners", mock.Anything, uint64(99)).Return(sameNonSigners, nil)

	mockChainStore := &mockValidatorsChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	chainsMap := xsync.NewMap[string, *mockValidatorsChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexValidators)

	input := types.IndexValidatorsInput{
		ChainID:   "chain-A",
		Height:    100,
		BlockTime: time.Now().UTC(),
	}

	future, err := env.ExecuteActivity(activityCtx.IndexValidators, input)
	require.NoError(t, err)

	var output types.IndexValidatorsOutput
	require.NoError(t, future.Get(&output))

	// No changes detected
	assert.Equal(t, uint32(0), output.NumValidators)
	assert.Equal(t, uint32(0), output.NumSigningInfos)
	assert.Empty(t, mockChainStore.insertedValidators)
	assert.Empty(t, mockChainStore.insertedSigningInfos)

	mockRPC.AssertExpectations(t)
}

// TestIndexValidators_RPCError tests handling of RPC errors
func TestIndexValidators_RPCError(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := &mockValidatorsRPCClient{}
	mockRPC.On("Validators", mock.Anything, uint64(100)).Return(([]*rpc.RpcValidator)(nil), assert.AnError)
	mockRPC.On("Validators", mock.Anything, uint64(99)).Return([]*rpc.RpcValidator{}, nil).Maybe()
	mockRPC.On("NonSigners", mock.Anything, mock.Anything).Return([]*rpc.RpcNonSigner{}, nil).Maybe()

	mockChainStore := &mockValidatorsChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	chainsMap := xsync.NewMap[string, *mockValidatorsChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexValidators)

	input := types.IndexValidatorsInput{
		ChainID:   "chain-A",
		Height:    100,
		BlockTime: time.Now().UTC(),
	}

	future, execErr := env.ExecuteActivity(activityCtx.IndexValidators, input)

	var output types.IndexValidatorsOutput
	var actErr error
	if execErr != nil {
		actErr = execErr
	} else {
		actErr = future.Get(&output)
	}

	// Should have an error
	assert.Error(t, actErr)
	assert.Contains(t, actErr.Error(), "fetch current validators")

	mockRPC.AssertExpectations(t)
}

// TestIndexValidators_NonSignersOptional tests that non-signers are optional
func TestIndexValidators_NonSignersOptional(t *testing.T) {
	logger := zaptest.NewLogger(t)

	adminStore := &fakeAdminStore{
		chain: &admin.Chain{
			ChainID:      "chain-A",
			RPCEndpoints: []string{"http://rpc.local"},
		},
	}

	mockRPC := &mockValidatorsRPCClient{}

	validators := []*rpc.RpcValidator{
		{
			Address:         "0x111",
			PublicKey:       "pk111",
			NetAddress:      "tcp://1.1.1.1:26656",
			StakedAmount:    1000,
			Output:          "0xout111",
			Committees:      []uint64{1},
			MaxPausedHeight: 0,
			UnstakingHeight: 0,
			Delegate:        "",
			Compound:        false,
		},
	}

	mockRPC.On("Validators", mock.Anything, uint64(100)).Return(validators, nil)
	mockRPC.On("Validators", mock.Anything, uint64(99)).Return([]*rpc.RpcValidator{}, nil)
	// Non-signers endpoint fails (maybe not implemented on this chain)
	mockRPC.On("NonSigners", mock.Anything, uint64(100)).Return(([]*rpc.RpcNonSigner)(nil), assert.AnError)
	mockRPC.On("NonSigners", mock.Anything, uint64(99)).Return(([]*rpc.RpcNonSigner)(nil), assert.AnError)

	mockChainStore := &mockValidatorsChainStore{
		chainID:      "chain-A",
		databaseName: "chain_a",
	}

	chainsMap := xsync.NewMap[string, *mockValidatorsChainStore]()
	chainsMap.Store("chain-A", mockChainStore)

	activityCtx := &activity.Context{
		Logger:     logger,
		AdminDB:    adminStore,
		RPCFactory: &fakeRPCFactory{client: mockRPC},
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(activityCtx.IndexValidators)

	input := types.IndexValidatorsInput{
		ChainID:   "chain-A",
		Height:    100,
		BlockTime: time.Now().UTC(),
	}

	future, err := env.ExecuteActivity(activityCtx.IndexValidators, input)
	require.NoError(t, err)

	var output types.IndexValidatorsOutput
	require.NoError(t, future.Get(&output))

	// Should still succeed with validators indexed
	assert.Equal(t, uint32(1), output.NumValidators)
	assert.Equal(t, uint32(0), output.NumSigningInfos) // No signing info since non-signers failed
	require.Len(t, mockChainStore.insertedValidators, 1)

	mockRPC.AssertExpectations(t)
}

// Mock implementations

type mockValidatorsRPCClient struct {
	mock.Mock
}

func (m *mockValidatorsRPCClient) ChainHead(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (m *mockValidatorsRPCClient) BlockByHeight(ctx context.Context, height uint64) (*indexermodels.Block, error) {
	return nil, nil
}

func (m *mockValidatorsRPCClient) TxsByHeight(ctx context.Context, height uint64) ([]*indexermodels.Transaction, error) {
	return nil, nil
}

func (m *mockValidatorsRPCClient) AccountsByHeight(ctx context.Context, height uint64) ([]*rpc.Account, error) {
	return nil, nil
}

func (m *mockValidatorsRPCClient) GetGenesisState(ctx context.Context, height uint64) (*rpc.GenesisState, error) {
	return nil, nil
}

func (m *mockValidatorsRPCClient) EventsByHeight(ctx context.Context, height uint64) ([]*indexermodels.Event, error) {
	return nil, nil
}

func (m *mockValidatorsRPCClient) OrdersByHeight(ctx context.Context, height uint64, chainID uint64) ([]*rpc.RpcOrder, error) {
	return nil, nil
}

func (m *mockValidatorsRPCClient) DexPrice(ctx context.Context, chainID uint64) (*indexermodels.DexPrice, error) {
	return nil, nil
}

func (m *mockValidatorsRPCClient) DexPrices(ctx context.Context) ([]*indexermodels.DexPrice, error) {
	return nil, nil
}

func (m *mockValidatorsRPCClient) PoolByID(ctx context.Context, id uint64) (*rpc.RpcPool, error) {
	return nil, nil
}

func (m *mockValidatorsRPCClient) Pools(ctx context.Context) ([]*rpc.RpcPool, error) {
	return nil, nil
}

func (m *mockValidatorsRPCClient) AllParams(ctx context.Context, height uint64) (*rpc.RpcAllParams, error) {
	return nil, nil
}

func (m *mockValidatorsRPCClient) Validators(ctx context.Context, height uint64) ([]*rpc.RpcValidator, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*rpc.RpcValidator), args.Error(1)
}

func (m *mockValidatorsRPCClient) NonSigners(ctx context.Context, height uint64) ([]*rpc.RpcNonSigner, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*rpc.RpcNonSigner), args.Error(1)
}

func (m *mockValidatorsRPCClient) ValParams(ctx context.Context, height uint64) (*rpc.RpcValidatorParams, error) {
	args := m.Called(ctx, height)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rpc.RpcValidatorParams), args.Error(1)
}

func (m *mockValidatorsRPCClient) CommitteesData(ctx context.Context, height uint64) ([]*rpc.RpcCommittee, error) {
	return nil, nil
}

func (m *mockValidatorsRPCClient) SubsidizedCommittees(ctx context.Context, height uint64) ([]uint64, error) {
	return nil, nil
}

func (m *mockValidatorsRPCClient) RetiredCommittees(ctx context.Context, height uint64) ([]uint64, error) {
	return nil, nil
}

func (m *mockValidatorsRPCClient) Poll(ctx context.Context) (map[string]*rpc.RpcPollResult, error) {
	return nil, nil
}

func (m *mockValidatorsRPCClient) DexBatchByHeight(ctx context.Context, height uint64, committee uint64) (*rpc.RpcDexBatch, error) {
	return nil, nil
}

func (m *mockValidatorsRPCClient) NextDexBatchByHeight(ctx context.Context, height uint64, committee uint64) (*rpc.RpcDexBatch, error) {
	return nil, nil
}

type mockValidatorsChainStore struct {
	chainID                     string
	databaseName                string
	insertedValidators          []*indexermodels.Validator
	insertedSigningInfos        []*indexermodels.ValidatorSigningInfo
	insertedCommitteeValidators []*indexermodels.CommitteeValidator
}

func (m *mockValidatorsChainStore) DatabaseName() string { return m.databaseName }
func (m *mockValidatorsChainStore) ChainKey() string     { return m.chainID }

func (m *mockValidatorsChainStore) InsertValidatorsStaging(ctx context.Context, validators []*indexermodels.Validator) error {
	m.insertedValidators = append(m.insertedValidators, validators...)
	return nil
}

func (m *mockValidatorsChainStore) InsertValidatorSigningInfoStaging(ctx context.Context, signingInfos []*indexermodels.ValidatorSigningInfo) error {
	m.insertedSigningInfos = append(m.insertedSigningInfos, signingInfos...)
	return nil
}

func (m *mockValidatorsChainStore) InsertCommitteeValidatorsStaging(ctx context.Context, committeeValidators []*indexermodels.CommitteeValidator) error {
	m.insertedCommitteeValidators = append(m.insertedCommitteeValidators, committeeValidators...)
	return nil
}

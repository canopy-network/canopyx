package activity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/canopy-network/canopy/fsm"
	"github.com/canopy-network/canopyx/app/indexer/types"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// IndexParams indexes chain parameters for a given block height.
// Params are only inserted when they differ from the previous height to maintain a sparse historical record.
// This follows the RPC(H) vs RPC(H-1) pattern for change detection, never querying the database.
// Returns output indicating whether params changed and execution duration in milliseconds.
func (ac *Context) IndexParams(ctx context.Context, in types.ActivityIndexAtHeight) (types.ActivityIndexParamsOutput, error) {
	start := time.Now()

	// Get RPC client with height-aware endpoint selection
	cli, err := ac.rpcClientForHeight(ctx, in.Height)
	if err != nil {
		return types.ActivityIndexParamsOutput{}, err
	}

	// Acquire (or ping) the chain DB to validate it exists
	chainDb, chainDbErr := ac.GetChainDb(ctx, ac.ChainID)
	if chainDbErr != nil {
		return types.ActivityIndexParamsOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	// Parallel RPC fetch using shared worker pool for performance
	var (
		paramsAtH   *fsm.Params
		paramsAtH1  *fsm.Params
		paramsErr   error
		paramsH1Err error
	)

	// Get a subgroup from the shared worker pool for parallel RPC fetching
	pool := ac.WorkerPool(2)
	group := pool.NewGroupContext(ctx)
	groupCtx := group.Context()

	// Worker 1: Fetch params at height H
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		paramsAtH, paramsErr = cli.AllParamsByHeight(groupCtx, in.Height)
	})

	// Worker 2: Fetch params at height H-1
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		if in.Height <= 1 {
			paramsAtH1 = nil
			return
		}
		paramsAtH1, paramsH1Err = cli.AllParamsByHeight(groupCtx, in.Height-1)
	})

	// Wait for all workers to complete
	if err := group.Wait(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, pond.ErrGroupStopped) {
		ac.Logger.Warn("parallel RPC fetch encountered error",
			zap.Uint64("chainId", ac.ChainID),
			zap.Uint64("height", in.Height),
			zap.Error(err),
		)
	}

	// Check for errors
	if paramsErr != nil {
		return types.ActivityIndexParamsOutput{}, fmt.Errorf("fetch params at height %d: %w", in.Height, paramsErr)
	}
	if paramsH1Err != nil {
		return types.ActivityIndexParamsOutput{}, fmt.Errorf("fetch params at height %d: %w", in.Height-1, paramsH1Err)
	}

	// Convert RPC params at H to an entity model
	currentParams := convertRpcParamsToEntity(paramsAtH, in.Height, in.BlockTime)

	// Determine if params changed by comparing with H-1
	var paramsChanged bool
	if in.Height == 1 {
		// Genesis block: always insert params
		paramsChanged = true
		ac.Logger.Debug("IndexParams genesis block - inserting initial params",
			zap.Uint64("height", in.Height))
	} else {
		// Convert RPC params at H-1 to an entity model (using dummy time since we only compare values)
		prevParams := convertRpcParamsToEntity(paramsAtH1, in.Height-1, time.Time{})

		// Compare all 31 fields between H and H-1
		paramsChanged = !paramsEqual(prevParams, currentParams)

		ac.Logger.Debug("IndexParams compared RPC(H) vs RPC(H-1)",
			zap.Uint64("height", in.Height),
			zap.Bool("paramsChanged", paramsChanged))
	}

	// Only insert if params changed (sparse insert)
	if paramsChanged {
		if err := chainDb.InsertParamsStaging(ctx, currentParams); err != nil {
			return types.ActivityIndexParamsOutput{}, err
		}
		ac.Logger.Info("Params changed, inserted to staging",
			zap.Uint64("height", in.Height),
			zap.Uint64("chainID", ac.ChainID))
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.ActivityIndexParamsOutput{
		ParamsChanged: paramsChanged,
		DurationMs:    durationMs,
	}, nil
}

// convertRpcParamsToEntity converts fsm.Params (protobuf type from Canopy) to the entity model.
func convertRpcParamsToEntity(rpcParams *fsm.Params, height uint64, blockTime time.Time) *indexermodels.Params {
	params := &indexermodels.Params{
		Height:     height,
		HeightTime: blockTime,
	}

	// Consensus params (handle nil)
	if rpcParams.Consensus != nil {
		params.BlockSize = rpcParams.Consensus.BlockSize
		params.ProtocolVersion = rpcParams.Consensus.ProtocolVersion
		params.RootChainID = rpcParams.Consensus.RootChainId
		params.Retired = rpcParams.Consensus.Retired
	}

	// Validator params (handle nil)
	if rpcParams.Validator != nil {
		params.UnstakingBlocks = rpcParams.Validator.UnstakingBlocks
		params.MaxPauseBlocks = rpcParams.Validator.MaxPauseBlocks
		params.DoubleSignSlashPercentage = rpcParams.Validator.DoubleSignSlashPercentage
		params.NonSignSlashPercentage = rpcParams.Validator.NonSignSlashPercentage
		params.MaxNonSign = rpcParams.Validator.MaxNonSign
		params.NonSignWindow = rpcParams.Validator.NonSignWindow
		params.MaxCommittees = rpcParams.Validator.MaxCommittees
		params.MaxCommitteeSize = rpcParams.Validator.MaxCommitteeSize
		params.EarlyWithdrawalPenalty = rpcParams.Validator.EarlyWithdrawalPenalty
		params.DelegateUnstakingBlocks = rpcParams.Validator.DelegateUnstakingBlocks
		params.MinimumOrderSize = rpcParams.Validator.MinimumOrderSize
		params.StakePercentForSubsidized = rpcParams.Validator.StakePercentForSubsidizedCommittee
		params.MaxSlashPerCommittee = rpcParams.Validator.MaxSlashPerCommittee
		params.DelegateRewardPercentage = rpcParams.Validator.DelegateRewardPercentage
		params.BuyDeadlineBlocks = rpcParams.Validator.BuyDeadlineBlocks
		params.LockOrderFeeMultiplier = rpcParams.Validator.LockOrderFeeMultiplier
		params.MinimumStakeForValidators = rpcParams.Validator.MinimumStakeForValidators
		params.MinimumStakeForDelegates = rpcParams.Validator.MinimumStakeForDelegates
		params.MaximumDelegatesPerCommittee = rpcParams.Validator.MaximumDelegatesPerCommittee
	}

	// Fee params (handle nil)
	if rpcParams.Fee != nil {
		params.SendFee = rpcParams.Fee.SendFee
		params.StakeFee = rpcParams.Fee.StakeFee
		params.EditStakeFee = rpcParams.Fee.EditStakeFee
		params.UnstakeFee = rpcParams.Fee.UnstakeFee
		params.PauseFee = rpcParams.Fee.PauseFee
		params.UnpauseFee = rpcParams.Fee.UnpauseFee
		params.ChangeParameterFee = rpcParams.Fee.ChangeParameterFee
		params.DaoTransferFee = rpcParams.Fee.DaoTransferFee
		params.CertificateResultsFee = rpcParams.Fee.CertificateResultsFee
		params.SubsidyFee = rpcParams.Fee.SubsidyFee
		params.CreateOrderFee = rpcParams.Fee.CreateOrderFee
		params.EditOrderFee = rpcParams.Fee.EditOrderFee
		params.DeleteOrderFee = rpcParams.Fee.DeleteOrderFee
		params.DexLimitOrderFee = rpcParams.Fee.DexLimitOrderFee
		params.DexLiquidityDepositFee = rpcParams.Fee.DexLiquidityDepositFee
		params.DexLiquidityWithdrawFee = rpcParams.Fee.DexLiquidityWithdrawFee
	}

	// Governance params (handle nil)
	if rpcParams.Governance != nil {
		params.DaoRewardPercentage = rpcParams.Governance.DaoRewardPercentage
	}

	return params
}

// paramsEqual compares all fields of two Params instances (excluding Height and HeightTime).
// Returns true if all parameter values are identical.
func paramsEqual(a, b *indexermodels.Params) bool {
	// Compare all semantic fields, excluding height and height_time which always differ
	return a.BlockSize == b.BlockSize &&
		a.ProtocolVersion == b.ProtocolVersion &&
		a.RootChainID == b.RootChainID &&
		a.Retired == b.Retired &&
		a.UnstakingBlocks == b.UnstakingBlocks &&
		a.MaxPauseBlocks == b.MaxPauseBlocks &&
		a.DoubleSignSlashPercentage == b.DoubleSignSlashPercentage &&
		a.NonSignSlashPercentage == b.NonSignSlashPercentage &&
		a.MaxNonSign == b.MaxNonSign &&
		a.NonSignWindow == b.NonSignWindow &&
		a.MaxCommittees == b.MaxCommittees &&
		a.MaxCommitteeSize == b.MaxCommitteeSize &&
		a.EarlyWithdrawalPenalty == b.EarlyWithdrawalPenalty &&
		a.DelegateUnstakingBlocks == b.DelegateUnstakingBlocks &&
		a.MinimumOrderSize == b.MinimumOrderSize &&
		a.StakePercentForSubsidized == b.StakePercentForSubsidized &&
		a.MaxSlashPerCommittee == b.MaxSlashPerCommittee &&
		a.DelegateRewardPercentage == b.DelegateRewardPercentage &&
		a.BuyDeadlineBlocks == b.BuyDeadlineBlocks &&
		a.LockOrderFeeMultiplier == b.LockOrderFeeMultiplier &&
		a.MinimumStakeForValidators == b.MinimumStakeForValidators &&
		a.MinimumStakeForDelegates == b.MinimumStakeForDelegates &&
		a.MaximumDelegatesPerCommittee == b.MaximumDelegatesPerCommittee &&
		a.SendFee == b.SendFee &&
		a.StakeFee == b.StakeFee &&
		a.EditStakeFee == b.EditStakeFee &&
		a.UnstakeFee == b.UnstakeFee &&
		a.PauseFee == b.PauseFee &&
		a.UnpauseFee == b.UnpauseFee &&
		a.ChangeParameterFee == b.ChangeParameterFee &&
		a.DaoTransferFee == b.DaoTransferFee &&
		a.CertificateResultsFee == b.CertificateResultsFee &&
		a.SubsidyFee == b.SubsidyFee &&
		a.CreateOrderFee == b.CreateOrderFee &&
		a.EditOrderFee == b.EditOrderFee &&
		a.DeleteOrderFee == b.DeleteOrderFee &&
		a.DexLimitOrderFee == b.DexLimitOrderFee &&
		a.DexLiquidityDepositFee == b.DexLiquidityDepositFee &&
		a.DexLiquidityWithdrawFee == b.DexLiquidityWithdrawFee &&
		a.DaoRewardPercentage == b.DaoRewardPercentage
}

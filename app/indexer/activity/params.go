package activity

import (
	"context"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// IndexParams indexes chain parameters for a given block height.
// Params are only inserted when they differ from the previous height to maintain a sparse historical record.
// This follows the RPC(H) vs RPC(H-1) pattern for change detection, never querying the database.
// Returns output indicating whether params changed and execution duration in milliseconds.
func (ac *Context) IndexParams(ctx context.Context, in types.ActivityIndexAtHeight) (types.ActivityIndexParamsOutput, error) {
	start := time.Now()

	cli, err := ac.rpcClient(ctx)
	if err != nil {
		return types.ActivityIndexParamsOutput{}, err
	}

	// Acquire (or ping) the chain DB to validate it exists
	chainDb, chainDbErr := ac.GetChainDb(ctx, ac.ChainID)
	if chainDbErr != nil {
		return types.ActivityIndexParamsOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	// Fetch params at the current height (H)
	paramsAtH, err := cli.AllParamsByHeight(ctx, in.Height)
	if err != nil {
		return types.ActivityIndexParamsOutput{}, fmt.Errorf("fetch params at height %d: %w", in.Height, err)
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
		// Fetch params at the previous height (H-1)
		paramsAtH1, err := cli.AllParamsByHeight(ctx, in.Height-1)
		if err != nil {
			return types.ActivityIndexParamsOutput{}, fmt.Errorf("fetch params at height %d: %w", in.Height-1, err)
		}

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

// convertRpcParamsToEntity converts RPC params to the entity model.
func convertRpcParamsToEntity(rpcParams *rpc.RpcAllParams, height uint64, blockTime time.Time) *indexermodels.Params {
	params := &indexermodels.Params{
		Height:     height,
		HeightTime: blockTime,
	}

	// Consensus params (handle nil)
	if rpcParams.ConsensusParams != nil {
		params.BlockSize = rpcParams.ConsensusParams.BlockSize
		params.ProtocolVersion = rpcParams.ConsensusParams.ProtocolVersion
		params.RootChainID = rpcParams.ConsensusParams.RootChainID
		params.Retired = rpcParams.ConsensusParams.Retired
	}

	// Validator params (handle nil)
	if rpcParams.ValidatorParams != nil {
		params.UnstakingBlocks = rpcParams.ValidatorParams.UnstakingBlocks
		params.MaxPauseBlocks = rpcParams.ValidatorParams.MaxPauseBlocks
		params.DoubleSignSlashPercentage = rpcParams.ValidatorParams.DoubleSignSlashPercentage
		params.NonSignSlashPercentage = rpcParams.ValidatorParams.NonSignSlashPercentage
		params.MaxNonSign = rpcParams.ValidatorParams.MaxNonSign
		params.NonSignWindow = rpcParams.ValidatorParams.NonSignWindow
		params.MaxCommittees = rpcParams.ValidatorParams.MaxCommittees
		params.MaxCommitteeSize = rpcParams.ValidatorParams.MaxCommitteeSize
		params.EarlyWithdrawalPenalty = rpcParams.ValidatorParams.EarlyWithdrawalPenalty
		params.DelegateUnstakingBlocks = rpcParams.ValidatorParams.DelegateUnstakingBlocks
		params.MinimumOrderSize = rpcParams.ValidatorParams.MinimumOrderSize
		params.StakePercentForSubsidized = rpcParams.ValidatorParams.StakePercentForSubsidized
		params.MaxSlashPerCommittee = rpcParams.ValidatorParams.MaxSlashPerCommittee
		params.DelegateRewardPercentage = rpcParams.ValidatorParams.DelegateRewardPercentage
		params.BuyDeadlineBlocks = rpcParams.ValidatorParams.BuyDeadlineBlocks
		params.LockOrderFeeMultiplier = rpcParams.ValidatorParams.LockOrderFeeMultiplier
	}

	// Fee params (handle nil)
	if rpcParams.FeeParams != nil {
		params.SendFee = rpcParams.FeeParams.SendFee
		params.StakeFee = rpcParams.FeeParams.StakeFee
		params.EditStakeFee = rpcParams.FeeParams.EditStakeFee
		params.UnstakeFee = rpcParams.FeeParams.UnstakeFee
		params.PauseFee = rpcParams.FeeParams.PauseFee
		params.UnpauseFee = rpcParams.FeeParams.UnpauseFee
		params.ChangeParameterFee = rpcParams.FeeParams.ChangeParameterFee
		params.DaoTransferFee = rpcParams.FeeParams.DaoTransferFee
		params.CertificateResultsFee = rpcParams.FeeParams.CertificateResultsFee
		params.SubsidyFee = rpcParams.FeeParams.SubsidyFee
		params.CreateOrderFee = rpcParams.FeeParams.CreateOrderFee
		params.EditOrderFee = rpcParams.FeeParams.EditOrderFee
		params.DeleteOrderFee = rpcParams.FeeParams.DeleteOrderFee
		params.DexLimitOrderFee = rpcParams.FeeParams.DexLimitOrderFee
		params.DexLiquidityDepositFee = rpcParams.FeeParams.DexLiquidityDepositFee
		params.DexLiquidityWithdrawFee = rpcParams.FeeParams.DexLiquidityWithdrawFee
	}

	// Governance params (handle nil)
	if rpcParams.GovParams != nil {
		params.DaoRewardPercentage = rpcParams.GovParams.DaoRewardPercentage
	}

	return params
}

// paramsEqual compares all fields of two Params instances (excluding Height and HeightTime).
// Returns true if all parameter values are identical.
func paramsEqual(a, b *indexermodels.Params) bool {
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

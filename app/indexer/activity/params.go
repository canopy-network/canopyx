package activity

import (
	"context"
	"time"

	globalstore "github.com/canopy-network/canopyx/pkg/db/global"

	"github.com/canopy-network/canopy/fsm"
	"github.com/canopy-network/canopyx/app/indexer/types"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// IndexParams indexes chain parameters for a given block height.
func (ac *Context) indexParamsFromBlob(ctx context.Context, chainDb globalstore.Store, height uint64, heightTime time.Time, currentData *blobData, previousData *blobData) (types.ActivityIndexParamsOutput, float64, error) {
	start := time.Now()
	currentParams := currentData.params
	if currentParams == nil {
		currentParams = &fsm.Params{}
	}
	currentParamsEntity := convertRpcParamsToEntity(currentParams, height, heightTime)
	var paramsChanged bool
	if height == 1 {
		paramsChanged = true
	} else {
		prevParams := previousData.params
		if prevParams == nil {
			prevParams = &fsm.Params{}
		}
		prevParamsEntity := convertRpcParamsToEntity(prevParams, height-1, time.Time{})
		paramsChanged = !paramsEqual(prevParamsEntity, currentParamsEntity)
	}
	if paramsChanged {
		if err := chainDb.InsertParams(ctx, currentParamsEntity); err != nil {
			return types.ActivityIndexParamsOutput{}, 0, err
		}
	}
	out := types.ActivityIndexParamsOutput{
		ParamsChanged: paramsChanged,
	}
	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return out, durationMs, nil
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
		params.RootChainID = uint16(rpcParams.Consensus.RootChainId)
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

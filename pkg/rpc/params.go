package rpc

import (
	"context"
	"fmt"
	"net/http"
)

// AllParamsByHeight queries the /v1/query/params endpoint to retrieve all chain parameters at a specific height.
// This includes consensus, validator, fee, and governance parameters.
//
// Parameters:
//   - height: The block height to query params for (0 = latest)
//
// Returns:
//   - *RpcAllParams: Complete set of chain parameters at the specified height
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) AllParamsByHeight(ctx context.Context, height uint64) (*RpcAllParams, error) {
	var params RpcAllParams
	req := QueryByHeightRequest{Height: height}
	if err := c.doJSON(ctx, http.MethodPost, allParamsPath, req, &params); err != nil {
		return nil, fmt.Errorf("fetch all params at height %d: %w", height, err)
	}
	return &params, nil
}

// ValParamsByHeight queries the /v1/query/val-params endpoint to retrieve validator parameters at a specific height.
// Validator parameters control staking, slashing, committees, and orders.
//
// Parameters:
//   - height: The block height to query params for (0 = latest)
//
// Returns:
//   - *ValidatorParams: Validator-related parameters at the specified height
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) ValParamsByHeight(ctx context.Context, height uint64) (*ValidatorParams, error) {
	var params ValidatorParams
	req := QueryByHeightRequest{Height: height}
	if err := c.doJSON(ctx, http.MethodPost, valParamsPath, req, &params); err != nil {
		return nil, fmt.Errorf("fetch validator params at height %d: %w", height, err)
	}
	return &params, nil
}

// ConsensusParams represents consensus-related parameters.
// Includes block size limits, protocol version, and root chain ID.
type ConsensusParams struct {
	BlockSize       uint64 `json:"blockSize"`       // Maximum allowed block size
	ProtocolVersion string `json:"protocolVersion"` // Minimum protocol version required
	RootChainID     uint64 `json:"rootChainID"`     // Root chain ID (parent chain)
	Retired         uint64 `json:"retired"`         // Whether chain is retired
}

// ValidatorParams represents validator-related parameters.
// Controls staking, slashing, committees, and order-related settings.
type ValidatorParams struct {
	UnstakingBlocks              uint64 `json:"unstakingBlocks"`                    // Number of blocks before unstaking completes
	MaxPauseBlocks               uint64 `json:"maxPauseBlocks"`                     // Maximum blocks a validator can be paused
	DoubleSignSlashPercentage    uint64 `json:"doubleSignSlashPercentage"`          // Slash percentage for double signing
	NonSignSlashPercentage       uint64 `json:"nonSignSlashPercentage"`             // Slash percentage for not signing
	MaxNonSign                   uint64 `json:"maxNonSign"`                         // Maximum allowed non-sign events
	NonSignWindow                uint64 `json:"nonSignWindow"`                      // Window size for counting non-signs
	MaxCommittees                uint64 `json:"maxCommittees"`                      // Maximum number of committees
	MaxCommitteeSize             uint64 `json:"maxCommitteeSize"`                   // Maximum size of a committee
	EarlyWithdrawalPenalty       uint64 `json:"earlyWithdrawalPenalty"`             // Penalty for early withdrawal
	DelegateUnstakingBlocks      uint64 `json:"delegateUnstakingBlocks"`            // Blocks before delegate unstaking completes
	MinimumOrderSize             uint64 `json:"minimumOrderSize"`                   // Minimum size for an order
	StakePercentForSubsidized    uint64 `json:"stakePercentForSubsidizedCommittee"` // Stake percentage required for subsidized committee
	MaxSlashPerCommittee         uint64 `json:"maxSlashPerCommittee"`               // Maximum slash amount per committee
	DelegateRewardPercentage     uint64 `json:"delegateRewardPercentage"`           // Percentage of rewards that go to delegates
	BuyDeadlineBlocks            uint64 `json:"buyDeadlineBlocks"`                  // Deadline in blocks for buy orders
	LockOrderFeeMultiplier       uint64 `json:"lockOrderFeeMultiplier"`             // Fee multiplier for locked orders
	MinimumStakeForValidators    uint64 `json:"minimumStakeForValidators"`          // Minimum stake required to become a validator (from PR #261)
	MinimumStakeForDelegates     uint64 `json:"minimumStakeForDelegates"`           // Minimum stake required to delegate (from PR #261)
	MaximumDelegatesPerCommittee uint64 `json:"maximumDelegatesPerCommittee"`       // Maximum number of delegates allowed per committee (from PR #261)
}

// FeeParams represents fee-related parameters.
// Defines costs for various transaction types.
type FeeParams struct {
	SendFee                 uint64 `json:"sendFee"`                 // Fee for send transactions
	StakeFee                uint64 `json:"stakeFee"`                // Fee for staking
	EditStakeFee            uint64 `json:"editStakeFee"`            // Fee for editing stake
	UnstakeFee              uint64 `json:"unstakeFee"`              // Fee for unstaking
	PauseFee                uint64 `json:"pauseFee"`                // Fee for pausing validator
	UnpauseFee              uint64 `json:"unpauseFee"`              // Fee for unpausing validator
	ChangeParameterFee      uint64 `json:"changeParameterFee"`      // Fee for changing parameters
	DaoTransferFee          uint64 `json:"daoTransferFee"`          // Fee for DAO transfers
	CertificateResultsFee   uint64 `json:"certificateResultsFee"`   // Fee for certificate results
	SubsidyFee              uint64 `json:"subsidyFee"`              // Fee for subsidy transactions
	CreateOrderFee          uint64 `json:"createOrderFee"`          // Fee for creating orders
	EditOrderFee            uint64 `json:"editOrderFee"`            // Fee for editing orders
	DeleteOrderFee          uint64 `json:"deleteOrderFee"`          // Fee for deleting orders
	DexLimitOrderFee        uint64 `json:"dexLimitOrderFee"`        // Fee for DEX limit orders
	DexLiquidityDepositFee  uint64 `json:"dexLiquidityDepositFee"`  // Fee for DEX liquidity deposits
	DexLiquidityWithdrawFee uint64 `json:"dexLiquidityWithdrawFee"` // Fee for DEX liquidity withdrawals
}

// GovParams represents governance-related parameters.
// Controls DAO and governance settings.
type GovParams struct {
	DaoRewardPercentage uint64 `json:"daoRewardPercentage"` // Percentage of rewards that go to DAO
}

// RpcAllParams represents the complete set of chain parameters from /v1/query/params.
// This is returned by the AllParamsByHeight() RPC method.
type RpcAllParams struct {
	ConsensusParams *ConsensusParams `json:"consensus"`
	ValidatorParams *ValidatorParams `json:"validator"`
	FeeParams       *FeeParams       `json:"fee"`
	GovParams       *GovParams       `json:"governance"`
}

package indexer

import "time"

const (
	// ParamsProductionTableName is the production table name for params
	ParamsProductionTableName = "params"
	// ParamsStagingTableName is the staging table name for params
	ParamsStagingTableName = "params_staging"
)

// Params represents chain parameters at a specific height.
// This entity stores all governance parameters including consensus, validator, fee, and gov settings.
// Parameters are only inserted when they change to maintain a sparse historical record.
type Params struct {
	// Height and timestamp
	Height     uint64    `ch:"height"`      // Block height when these params became effective
	HeightTime time.Time `ch:"height_time"` // Timestamp of the block

	// Consensus parameters (4 fields)
	BlockSize       uint64 `ch:"block_size"`       // Maximum allowed block size in bytes
	ProtocolVersion string `ch:"protocol_version"` // Minimum protocol version required
	RootChainID     uint64 `ch:"root_chain_id"`    // Root chain ID (parent chain)
	Retired         uint64 `ch:"retired"`          // Whether chain is retired (0=active, 1=retired)

	// Validator parameters (16 fields)
	UnstakingBlocks           uint64 `ch:"unstaking_blocks"`             // Number of blocks before unstaking completes
	MaxPauseBlocks            uint64 `ch:"max_pause_blocks"`             // Maximum blocks a validator can be paused
	DoubleSignSlashPercentage uint64 `ch:"double_sign_slash_percentage"` // Slash percentage for double signing
	NonSignSlashPercentage    uint64 `ch:"non_sign_slash_percentage"`    // Slash percentage for not signing
	MaxNonSign                uint64 `ch:"max_non_sign"`                 // Maximum allowed non-sign events
	NonSignWindow             uint64 `ch:"non_sign_window"`              // Window size for counting non-signs
	MaxCommittees             uint64 `ch:"max_committees"`               // Maximum number of committees
	MaxCommitteeSize          uint64 `ch:"max_committee_size"`           // Maximum size of a committee
	EarlyWithdrawalPenalty    uint64 `ch:"early_withdrawal_penalty"`     // Penalty for early withdrawal
	DelegateUnstakingBlocks   uint64 `ch:"delegate_unstaking_blocks"`    // Blocks before delegate unstaking completes
	MinimumOrderSize          uint64 `ch:"minimum_order_size"`           // Minimum size for an order
	StakePercentForSubsidized uint64 `ch:"stake_percent_for_subsidized"` // Stake percentage required for subsidized committee
	MaxSlashPerCommittee      uint64 `ch:"max_slash_per_committee"`      // Maximum slash amount per committee
	DelegateRewardPercentage  uint64 `ch:"delegate_reward_percentage"`   // Percentage of rewards that go to delegates
	BuyDeadlineBlocks         uint64 `ch:"buy_deadline_blocks"`          // Deadline in blocks for buy orders
	LockOrderFeeMultiplier    uint64 `ch:"lock_order_fee_multiplier"`    // Fee multiplier for locked orders

	// Fee parameters (16 fields)
	SendFee                 uint64 `ch:"send_fee"`                   // Fee for send transactions
	StakeFee                uint64 `ch:"stake_fee"`                  // Fee for staking
	EditStakeFee            uint64 `ch:"edit_stake_fee"`             // Fee for editing stake
	UnstakeFee              uint64 `ch:"unstake_fee"`                // Fee for unstaking
	PauseFee                uint64 `ch:"pause_fee"`                  // Fee for pausing validator
	UnpauseFee              uint64 `ch:"unpause_fee"`                // Fee for unpausing validator
	ChangeParameterFee      uint64 `ch:"change_parameter_fee"`       // Fee for changing parameters
	DaoTransferFee          uint64 `ch:"dao_transfer_fee"`           // Fee for DAO transfers
	CertificateResultsFee   uint64 `ch:"certificate_results_fee"`    // Fee for certificate results
	SubsidyFee              uint64 `ch:"subsidy_fee"`                // Fee for subsidy transactions
	CreateOrderFee          uint64 `ch:"create_order_fee"`           // Fee for creating orders
	EditOrderFee            uint64 `ch:"edit_order_fee"`             // Fee for editing orders
	DeleteOrderFee          uint64 `ch:"delete_order_fee"`           // Fee for deleting orders
	DexLimitOrderFee        uint64 `ch:"dex_limit_order_fee"`        // Fee for DEX limit orders
	DexLiquidityDepositFee  uint64 `ch:"dex_liquidity_deposit_fee"`  // Fee for DEX liquidity deposits
	DexLiquidityWithdrawFee uint64 `ch:"dex_liquidity_withdraw_fee"` // Fee for DEX liquidity withdrawals

	// Governance parameters (1 field)
	DaoRewardPercentage uint64 `ch:"dao_reward_percentage"` // Percentage of rewards that go to DAO
}

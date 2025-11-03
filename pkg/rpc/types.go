package rpc

// TODO: replace this types with the RPC types from canopy as package once available
//   that will means a mayor refactor probably but deserves the time

// --- Query types

type QueryByHeightRequest map[string]any

func NewQueryByHeightRequest(height uint64) QueryByHeightRequest {
	return QueryByHeightRequest{"height": height}
}

func (r QueryByHeightRequest) Height() uint64 {
	if height, ok := r["height"].(uint64); !ok {
		return 0
	} else {
		return height
	}
}

// --- Response types

// GenesisState represents the full genesis state returned by the RPC endpoint.
// This contains all initial blockchain states, including accounts, validators, pools, and parameters.
type GenesisState struct {
	Time     uint64     `json:"time"`     // Genesis timestamp
	Accounts []*Account `json:"accounts"` // Initial account balances
	// Future fields for complete genesis state
	Validators []interface{} `json:"validators"` // Initial validators (not used yet)
	Pools      []interface{} `json:"pools"`      // Initial pools (not used yet)
	Params     *Params       `json:"params"`     // Chain parameters
}

// Params represents the governance parameters returned from /v1/query/state.
// Contains consensus, validator, fee, and other blockchain parameters.
type Params struct {
	ConsensusParams *ConsensusParams `json:"consensusParams"`
	ValidatorParams *ValidatorParams `json:"validatorParams"`
	FeeParams       *FeeParams       `json:"feeParams"`
	GovParams       *GovParams       `json:"govParams"`
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
	UnstakingBlocks           uint64 `json:"unstakingBlocks"`                    // Number of blocks before unstaking completes
	MaxPauseBlocks            uint64 `json:"maxPauseBlocks"`                     // Maximum blocks a validator can be paused
	DoubleSignSlashPercentage uint64 `json:"doubleSignSlashPercentage"`          // Slash percentage for double signing
	NonSignSlashPercentage    uint64 `json:"nonSignSlashPercentage"`             // Slash percentage for not signing
	MaxNonSign                uint64 `json:"maxNonSign"`                         // Maximum allowed non-sign events
	NonSignWindow             uint64 `json:"nonSignWindow"`                      // Window size for counting non-signs
	MaxCommittees             uint64 `json:"maxCommittees"`                      // Maximum number of committees
	MaxCommitteeSize          uint64 `json:"maxCommitteeSize"`                   // Maximum size of a committee
	EarlyWithdrawalPenalty    uint64 `json:"earlyWithdrawalPenalty"`             // Penalty for early withdrawal
	DelegateUnstakingBlocks   uint64 `json:"delegateUnstakingBlocks"`            // Blocks before delegate unstaking completes
	MinimumOrderSize          uint64 `json:"minimumOrderSize"`                   // Minimum size for an order
	StakePercentForSubsidized uint64 `json:"stakePercentForSubsidizedCommittee"` // Stake percentage required for subsidized committee
	MaxSlashPerCommittee      uint64 `json:"maxSlashPerCommittee"`               // Maximum slash amount per committee
	DelegateRewardPercentage  uint64 `json:"delegateRewardPercentage"`           // Percentage of rewards that go to delegates
	BuyDeadlineBlocks         uint64 `json:"buyDeadlineBlocks"`                  // Deadline in blocks for buy orders
	LockOrderFeeMultiplier    uint64 `json:"lockOrderFeeMultiplier"`             // Fee multiplier for locked orders
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
// This is returned by the AllParams() RPC method.
type RpcAllParams struct {
	ConsensusParams *ConsensusParams `json:"consensusParams"`
	ValidatorParams *ValidatorParams `json:"validatorParams"`
	FeeParams       *FeeParams       `json:"feeParams"`
	GovParams       *GovParams       `json:"govParams"`
}

// StateResponse represents the response from /v1/query/state endpoint.
// This is an alias for GenesisState as they return the same structure.
type StateResponse = GenesisState

// RpcDexLimitOrder represents a DEX limit order from the RPC.
// This matches the DexLimitOrder proto structure returned by dex-batch endpoints.
type RpcDexLimitOrder struct {
	OrderID         string `json:"orderId"`         // Hex-encoded order ID
	AmountForSale   uint64 `json:"amountForSale"`   // Amount of asset being sold
	RequestedAmount uint64 `json:"requestedAmount"` // Minimum amount of counter-asset to receive
	Address         string `json:"address"`         // Hex-encoded address
}

// RpcDexLiquidityDeposit represents a liquidity deposit from the RPC.
// This matches the DexLiquidityDeposit proto structure.
type RpcDexLiquidityDeposit struct {
	OrderID string `json:"orderId"` // Hex-encoded order ID
	Address string `json:"address"` // Hex-encoded address
	Amount  uint64 `json:"amount"`  // Amount being deposited
}

// RpcDexLiquidityWithdraw represents a liquidity withdrawal from the RPC.
// This matches the DexLiquidityWithdraw proto structure.
type RpcDexLiquidityWithdraw struct {
	OrderID string `json:"orderId"` // Hex-encoded order ID
	Address string `json:"address"` // Hex-encoded address
	Percent uint64 `json:"percent"` // Percentage of pool points being withdrawn (0-100)
}

// RpcPoolPoints represents an address's pool points allocation.
// This matches the PoolPoints proto structure.
type RpcPoolPoints struct {
	Address string `json:"address"` // Hex-encoded address
	Points  uint64 `json:"points"`  // Number of pool points
}

// RpcDexBatch represents a DEX batch from the RPC.
// This matches the DexBatch proto structure returned by /v1/query/dex-batch and /v1/query/next-dex-batch.
type RpcDexBatch struct {
	Committee       uint64                     `json:"committee"`       // Committee ID (counter-asset chain ID)
	ReceiptHash     string                     `json:"receiptHash"`     // Hash of counter chain batch receipts
	Orders          []*RpcDexLimitOrder        `json:"orders"`          // Limit orders in this batch
	Deposits        []*RpcDexLiquidityDeposit  `json:"deposits"`        // Liquidity deposits in this batch
	Withdrawals     []*RpcDexLiquidityWithdraw `json:"withdrawals"`     // Liquidity withdrawals in this batch
	PoolSize        uint64                     `json:"poolSize"`        // Current liquidity pool balance
	CounterPoolSize uint64                     `json:"counterPoolSize"` // Last known counter chain pool size
	PoolPoints      []*RpcPoolPoints           `json:"poolPoints"`      // Pool points by holder
	TotalPoolPoints uint64                     `json:"totalPoolPoints"` // Total pool points
	Receipts        []uint64                   `json:"receipts"`        // Execution receipts (amounts distributed)
	LockedHeight    uint64                     `json:"lockedHeight"`    // Height when batch was locked
}

// RpcValidator represents a validator returned from the RPC.
// This matches the Validator protobuf structure from Canopy (fsm/validator.pb.go:45-76).
// The structure contains exactly 10 fields that map directly to Canopy's validator state.
type RpcValidator struct {
	Address         string   `json:"address"`              // Hex-encoded validator address
	PublicKey       string   `json:"publicKey"`            // Hex-encoded Ed25519 public key
	NetAddress      string   `json:"netAddress"`           // P2P network address (host:port)
	StakedAmount    uint64   `json:"stakedAmount"`         // Amount staked by validator in uCNPY
	Committees      []uint64 `json:"committees,omitempty"` // Committee IDs validator is assigned to
	MaxPausedHeight uint64   `json:"maxPausedHeight"`      // Block height when pause expires (0 = not paused)
	UnstakingHeight uint64   `json:"unstakingHeight"`      // Block height when unstaking completes (0 = not unstaking)
	Output          string   `json:"output,omitempty"`     // Reward destination address (hex-encoded, empty = self)
	Delegate        bool     `json:"delegate,omitempty"`   // Whether validator accepts delegations
	Compound        bool     `json:"compound,omitempty"`   // Whether rewards auto-compound into stake
}

// RpcNonSigner represents non-signing info returned from the RPC.
// This matches the NonSigner protobuf structure from Canopy (fsm/validator.pb.go:236-244).
// Tracks validators that have failed to sign blocks within the non-sign tracking window.
type RpcNonSigner struct {
	Address string `json:"address"` // Hex-encoded validator address
	Counter uint64 `json:"counter"` // Number of blocks missed within the tracking window
}

// RpcPaymentPercent represents a payment recipient and their reward percentage.
// This matches the PaymentPercents proto structure.
type RpcPaymentPercent struct {
	Address string `json:"address"` // Hex-encoded recipient address
	Percent uint64 `json:"percent"` // Percentage of rewards (0-100)
}

// RpcCommitteeData represents committee data returned from the RPC.
// This matches the CommitteeData proto structure returned by /v1/query/committee-data and /v1/query/committees-data.
// A committee is a group of validators responsible for a specific chain.
type RpcCommitteeData struct {
	ChainID                uint64               `json:"chainID"`                // Unique identifier of the chain/committee
	LastRootHeightUpdated  uint64               `json:"lastRootHeightUpdated"`  // Canopy height of most recent Certificate Results tx
	LastChainHeightUpdated uint64               `json:"lastChainHeightUpdated"` // 3rd party chain height of most recent Certificate Results
	PaymentPercents        []*RpcPaymentPercent `json:"paymentPercents"`        // List of reward recipients and percentages
	NumberOfSamples        uint64               `json:"numberOfSamples"`        // Count of processed Certificate Result Transactions
}

// RpcCommitteesData represents a list of committee data returned from /v1/query/committees-data.
type RpcCommitteesData struct {
	List []*RpcCommitteeData `json:"list"` // List of all committees
}

// RpcVoteStats represents vote statistics for a proposal.
// This matches the VoteStats structure from Canopy's governance system.
type RpcVoteStats struct {
	ApproveTokens     uint64 `json:"approveTokens"`    // Tokens voted 'yay'
	RejectTokens      uint64 `json:"rejectTokens"`     // Tokens voted 'nay'
	TotalVotedTokens  uint64 `json:"totalVotedTokens"` // Total tokens that voted
	TotalTokens       uint64 `json:"totalTokens"`      // Total tokens that could vote
	ApprovePercentage uint64 `json:"approvedPercent"`  // % approve (note: "approved" not "approve" in JSON)
	RejectPercentage  uint64 `json:"rejectPercent"`    // % reject
	VotedPercentage   uint64 `json:"votedPercent"`     // % participated
}

// RpcPollResult represents the current state of voting for a proposal.
// This matches the PollResult structure from Canopy's governance system.
type RpcPollResult struct {
	ProposalHash string       `json:"proposalHash"` // Hash of the proposal
	ProposalURL  string       `json:"proposalURL"`  // URL of the proposal document
	Accounts     RpcVoteStats `json:"accounts"`     // Vote statistics for accounts
	Validators   RpcVoteStats `json:"validators"`   // Vote statistics for validators
}

// RpcPoll represents a map of poll results keyed by proposal hash.
// This matches the Poll structure from Canopy's governance system.
// The map may be empty if there are no active proposals.
type RpcPoll map[string]RpcPollResult

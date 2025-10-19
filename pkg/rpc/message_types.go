package rpc

// MessageType identifies the type of transaction
type MessageType string

const (
	MsgTypeSend       MessageType = "send"
	MsgTypeDelegate   MessageType = "delegate"
	MsgTypeUndelegate MessageType = "undelegate"
	MsgTypeStake      MessageType = "stake"
	MsgTypeUnstake    MessageType = "unstake"
	MsgTypeEditStake  MessageType = "edit_stake"
	MsgTypeVote       MessageType = "vote"
	MsgTypeProposal   MessageType = "proposal"
	MsgTypeContract   MessageType = "contract"
	MsgTypeSystem     MessageType = "system"
	MsgTypeUnknown    MessageType = "unknown"
	MsgTypePause                 MessageType = "pause"
	MsgTypeUnpause               MessageType = "unpause"
	MsgTypeChangeParameter       MessageType = "changeParameter"
	MsgTypeDAOTransfer           MessageType = "daoTransfer"
	MsgTypeCertificateResults    MessageType = "certificateResults"
	MsgTypeSubsidy               MessageType = "subsidy"
	MsgTypeCreateOrder           MessageType = "createOrder"
	MsgTypeEditOrder             MessageType = "editOrder"
	MsgTypeDeleteOrder           MessageType = "deleteOrder"
	MsgTypeDexLimitOrder         MessageType = "dexLimitOrder"
	MsgTypeDexLiquidityDeposit   MessageType = "dexLiquidityDeposit"
	MsgTypeDexLiquidityWithdraw  MessageType = "dexLiquidityWithdraw"
)

// Message is the interface all transaction message types implement.
// This enables polymorphic handling of different transaction types while
// extracting common queryable fields (signer, counterparty, amount).
type Message interface {
	Type() MessageType
	GetSigner() string
	GetCounterparty() *string // nil for messages without counterparty
	GetAmount() *uint64       // nil for messages without amount
	GetValidatorAddress() *string
	GetCommission() *float64
	GetChainID() *uint64
	GetSellAmount() *uint64
	GetBuyAmount() *uint64
	GetLiquidityAmount() *uint64
	GetOrderID() *string
	GetPrice() *float64
	GetParamKey() *string
	GetParamValue() *string
	GetCommitteeID() *uint64
	GetRecipient() *string
}

// SendMessage represents a token transfer
type SendMessage struct {
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
	Amount      uint64 `json:"amount"`
	Memo        string `json:"memo,omitempty"`
}

func (m *SendMessage) Type() MessageType              { return MsgTypeSend }
func (m *SendMessage) GetSigner() string              { return m.FromAddress }
func (m *SendMessage) GetCounterparty() *string       { return &m.ToAddress }
func (m *SendMessage) GetAmount() *uint64             { return &m.Amount }
func (m *SendMessage) GetValidatorAddress() *string   { return nil }
func (m *SendMessage) GetCommission() *float64        { return nil }
func (m *SendMessage) GetChainID() *uint64            { return nil }
func (m *SendMessage) GetSellAmount() *uint64         { return nil }
func (m *SendMessage) GetBuyAmount() *uint64          { return nil }
func (m *SendMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *SendMessage) GetOrderID() *string            { return nil }
func (m *SendMessage) GetPrice() *float64             { return nil }
func (m *SendMessage) GetParamKey() *string           { return nil }
func (m *SendMessage) GetParamValue() *string         { return nil }
func (m *SendMessage) GetCommitteeID() *uint64        { return nil }
func (m *SendMessage) GetRecipient() *string          { return nil }

// DelegateMessage represents delegation to a validator
type DelegateMessage struct {
	Delegator        string `json:"delegator"`
	ValidatorAddress string `json:"validator_address"`
	Amount           uint64 `json:"amount"`
	Memo             string `json:"memo,omitempty"`
}

func (m *DelegateMessage) Type() MessageType              { return MsgTypeDelegate }
func (m *DelegateMessage) GetSigner() string              { return m.Delegator }
func (m *DelegateMessage) GetCounterparty() *string       { return &m.ValidatorAddress }
func (m *DelegateMessage) GetAmount() *uint64             { return &m.Amount }
func (m *DelegateMessage) GetValidatorAddress() *string   { return &m.ValidatorAddress }
func (m *DelegateMessage) GetCommission() *float64        { return nil }
func (m *DelegateMessage) GetChainID() *uint64            { return nil }
func (m *DelegateMessage) GetSellAmount() *uint64         { return nil }
func (m *DelegateMessage) GetBuyAmount() *uint64          { return nil }
func (m *DelegateMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *DelegateMessage) GetOrderID() *string            { return nil }
func (m *DelegateMessage) GetPrice() *float64             { return nil }
func (m *DelegateMessage) GetParamKey() *string           { return nil }
func (m *DelegateMessage) GetParamValue() *string         { return nil }
func (m *DelegateMessage) GetCommitteeID() *uint64        { return nil }
func (m *DelegateMessage) GetRecipient() *string          { return nil }

// UndelegateMessage represents undelegation from a validator
type UndelegateMessage struct {
	Delegator        string `json:"delegator"`
	ValidatorAddress string `json:"validator_address"`
	Amount           uint64 `json:"amount"`
}

func (m *UndelegateMessage) Type() MessageType              { return MsgTypeUndelegate }
func (m *UndelegateMessage) GetSigner() string              { return m.Delegator }
func (m *UndelegateMessage) GetCounterparty() *string       { return &m.ValidatorAddress }
func (m *UndelegateMessage) GetAmount() *uint64             { return &m.Amount }
func (m *UndelegateMessage) GetValidatorAddress() *string   { return &m.ValidatorAddress }
func (m *UndelegateMessage) GetCommission() *float64        { return nil }
func (m *UndelegateMessage) GetChainID() *uint64            { return nil }
func (m *UndelegateMessage) GetSellAmount() *uint64         { return nil }
func (m *UndelegateMessage) GetBuyAmount() *uint64          { return nil }
func (m *UndelegateMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *UndelegateMessage) GetOrderID() *string            { return nil }
func (m *UndelegateMessage) GetPrice() *float64             { return nil }
func (m *UndelegateMessage) GetParamKey() *string           { return nil }
func (m *UndelegateMessage) GetParamValue() *string         { return nil }
func (m *UndelegateMessage) GetCommitteeID() *uint64        { return nil }
func (m *UndelegateMessage) GetRecipient() *string          { return nil }

// StakeMessage represents staking to a pool
type StakeMessage struct {
	Staker     string  `json:"staker"`
	Pool       string  `json:"pool"`
	Amount     uint64  `json:"amount"`
	LockPeriod *uint32 `json:"lock_period,omitempty"` // Optional lock period
}

func (m *StakeMessage) Type() MessageType              { return MsgTypeStake }
func (m *StakeMessage) GetSigner() string              { return m.Staker }
func (m *StakeMessage) GetCounterparty() *string       { return &m.Pool }
func (m *StakeMessage) GetAmount() *uint64             { return &m.Amount }
func (m *StakeMessage) GetValidatorAddress() *string   { return nil }
func (m *StakeMessage) GetCommission() *float64        { return nil }
func (m *StakeMessage) GetChainID() *uint64            { return nil }
func (m *StakeMessage) GetSellAmount() *uint64         { return nil }
func (m *StakeMessage) GetBuyAmount() *uint64          { return nil }
func (m *StakeMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *StakeMessage) GetOrderID() *string            { return nil }
func (m *StakeMessage) GetPrice() *float64             { return nil }
func (m *StakeMessage) GetParamKey() *string           { return nil }
func (m *StakeMessage) GetParamValue() *string         { return nil }
func (m *StakeMessage) GetCommitteeID() *uint64        { return nil }
func (m *StakeMessage) GetRecipient() *string          { return nil }

// UnstakeMessage represents unstaking from a pool
type UnstakeMessage struct {
	Staker string `json:"staker"`
	Pool   string `json:"pool"`
	Amount uint64 `json:"amount"`
}

func (m *UnstakeMessage) Type() MessageType              { return MsgTypeUnstake }
func (m *UnstakeMessage) GetSigner() string              { return m.Staker }
func (m *UnstakeMessage) GetCounterparty() *string       { return &m.Pool }
func (m *UnstakeMessage) GetAmount() *uint64             { return &m.Amount }
func (m *UnstakeMessage) GetValidatorAddress() *string   { return nil }
func (m *UnstakeMessage) GetCommission() *float64        { return nil }
func (m *UnstakeMessage) GetChainID() *uint64            { return nil }
func (m *UnstakeMessage) GetSellAmount() *uint64         { return nil }
func (m *UnstakeMessage) GetBuyAmount() *uint64          { return nil }
func (m *UnstakeMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *UnstakeMessage) GetOrderID() *string            { return nil }
func (m *UnstakeMessage) GetPrice() *float64             { return nil }
func (m *UnstakeMessage) GetParamKey() *string           { return nil }
func (m *UnstakeMessage) GetParamValue() *string         { return nil }
func (m *UnstakeMessage) GetCommitteeID() *uint64        { return nil }
func (m *UnstakeMessage) GetRecipient() *string          { return nil }

// EditStakeMessage represents editing stake parameters
type EditStakeMessage struct {
	Staker     string  `json:"staker"`
	Pool       string  `json:"pool"`
	Amount     *uint64 `json:"amount,omitempty"`      // nil if not changing amount
	LockPeriod *uint32 `json:"lock_period,omitempty"` // nil if not changing lock
}

func (m *EditStakeMessage) Type() MessageType              { return MsgTypeEditStake }
func (m *EditStakeMessage) GetSigner() string              { return m.Staker }
func (m *EditStakeMessage) GetCounterparty() *string       { return &m.Pool }
func (m *EditStakeMessage) GetAmount() *uint64             { return m.Amount }
func (m *EditStakeMessage) GetValidatorAddress() *string   { return nil }
func (m *EditStakeMessage) GetCommission() *float64        { return nil }
func (m *EditStakeMessage) GetChainID() *uint64            { return nil }
func (m *EditStakeMessage) GetSellAmount() *uint64         { return nil }
func (m *EditStakeMessage) GetBuyAmount() *uint64          { return nil }
func (m *EditStakeMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *EditStakeMessage) GetOrderID() *string            { return nil }
func (m *EditStakeMessage) GetPrice() *float64             { return nil }
func (m *EditStakeMessage) GetParamKey() *string           { return nil }
func (m *EditStakeMessage) GetParamValue() *string         { return nil }
func (m *EditStakeMessage) GetCommitteeID() *uint64        { return nil }
func (m *EditStakeMessage) GetRecipient() *string          { return nil }

// VoteMessage represents a governance vote
type VoteMessage struct {
	Voter      string `json:"voter"`
	ProposalID uint64 `json:"proposal_id"`
	Option     string `json:"option"` // "yes", "no", "abstain", "veto"
	Memo       string `json:"memo,omitempty"`
}

func (m *VoteMessage) Type() MessageType              { return MsgTypeVote }
func (m *VoteMessage) GetSigner() string              { return m.Voter }
func (m *VoteMessage) GetCounterparty() *string       { return nil }
func (m *VoteMessage) GetAmount() *uint64             { return nil }
func (m *VoteMessage) GetValidatorAddress() *string   { return nil }
func (m *VoteMessage) GetCommission() *float64        { return nil }
func (m *VoteMessage) GetChainID() *uint64            { return nil }
func (m *VoteMessage) GetSellAmount() *uint64         { return nil }
func (m *VoteMessage) GetBuyAmount() *uint64          { return nil }
func (m *VoteMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *VoteMessage) GetOrderID() *string            { return nil }
func (m *VoteMessage) GetPrice() *float64             { return nil }
func (m *VoteMessage) GetParamKey() *string           { return nil }
func (m *VoteMessage) GetParamValue() *string         { return nil }
func (m *VoteMessage) GetCommitteeID() *uint64        { return nil }
func (m *VoteMessage) GetRecipient() *string          { return nil }

// ProposalMessage represents creating a governance proposal
type ProposalMessage struct {
	Proposer    string `json:"proposer"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Deposit     uint64 `json:"deposit"`
}

func (m *ProposalMessage) Type() MessageType              { return MsgTypeProposal }
func (m *ProposalMessage) GetSigner() string              { return m.Proposer }
func (m *ProposalMessage) GetCounterparty() *string       { return nil }
func (m *ProposalMessage) GetAmount() *uint64             { return &m.Deposit }
func (m *ProposalMessage) GetValidatorAddress() *string   { return nil }
func (m *ProposalMessage) GetCommission() *float64        { return nil }
func (m *ProposalMessage) GetChainID() *uint64            { return nil }
func (m *ProposalMessage) GetSellAmount() *uint64         { return nil }
func (m *ProposalMessage) GetBuyAmount() *uint64          { return nil }
func (m *ProposalMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *ProposalMessage) GetOrderID() *string            { return nil }
func (m *ProposalMessage) GetPrice() *float64             { return nil }
func (m *ProposalMessage) GetParamKey() *string           { return nil }
func (m *ProposalMessage) GetParamValue() *string         { return nil }
func (m *ProposalMessage) GetCommitteeID() *uint64        { return nil }
func (m *ProposalMessage) GetRecipient() *string          { return nil }

// ContractMessage represents a smart contract interaction
type ContractMessage struct {
	Caller          string  `json:"caller"`
	ContractAddress string  `json:"contract_address"`
	Method          string  `json:"method"`
	CallData        string  `json:"call_data"` // JSON or hex-encoded
	Value           *uint64 `json:"value,omitempty"`
}

func (m *ContractMessage) Type() MessageType              { return MsgTypeContract }
func (m *ContractMessage) GetSigner() string              { return m.Caller }
func (m *ContractMessage) GetCounterparty() *string       { return &m.ContractAddress }
func (m *ContractMessage) GetAmount() *uint64             { return m.Value }
func (m *ContractMessage) GetValidatorAddress() *string   { return nil }
func (m *ContractMessage) GetCommission() *float64        { return nil }
func (m *ContractMessage) GetChainID() *uint64            { return nil }
func (m *ContractMessage) GetSellAmount() *uint64         { return nil }
func (m *ContractMessage) GetBuyAmount() *uint64          { return nil }
func (m *ContractMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *ContractMessage) GetOrderID() *string            { return nil }
func (m *ContractMessage) GetPrice() *float64             { return nil }
func (m *ContractMessage) GetParamKey() *string           { return nil }
func (m *ContractMessage) GetParamValue() *string         { return nil }
func (m *ContractMessage) GetCommitteeID() *uint64        { return nil }
func (m *ContractMessage) GetRecipient() *string          { return nil }

// SystemMessage represents a system-level transaction
type SystemMessage struct {
	Executor string                 `json:"executor"`
	Action   string                 `json:"action"`
	Params   map[string]interface{} `json:"params,omitempty"`
}

func (m *SystemMessage) Type() MessageType              { return MsgTypeSystem }
func (m *SystemMessage) GetSigner() string              { return m.Executor }
func (m *SystemMessage) GetCounterparty() *string       { return nil }
func (m *SystemMessage) GetAmount() *uint64             { return nil }
func (m *SystemMessage) GetValidatorAddress() *string   { return nil }
func (m *SystemMessage) GetCommission() *float64        { return nil }
func (m *SystemMessage) GetChainID() *uint64            { return nil }
func (m *SystemMessage) GetSellAmount() *uint64         { return nil }
func (m *SystemMessage) GetBuyAmount() *uint64          { return nil }
func (m *SystemMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *SystemMessage) GetOrderID() *string            { return nil }
func (m *SystemMessage) GetPrice() *float64             { return nil }
func (m *SystemMessage) GetParamKey() *string           { return nil }
func (m *SystemMessage) GetParamValue() *string         { return nil }
func (m *SystemMessage) GetCommitteeID() *uint64        { return nil }
func (m *SystemMessage) GetRecipient() *string          { return nil }

// UnknownMessage represents an unrecognized transaction type.
// This is a fallback for transaction types we don't yet support.
type UnknownMessage struct {
	Signer string                 `json:"signer"`
	Data   map[string]interface{} `json:"data"`
}

func (m *UnknownMessage) Type() MessageType              { return MsgTypeUnknown }
func (m *UnknownMessage) GetSigner() string              { return m.Signer }
func (m *UnknownMessage) GetCounterparty() *string       { return nil }
func (m *UnknownMessage) GetAmount() *uint64             { return nil }
func (m *UnknownMessage) GetValidatorAddress() *string   { return nil }
func (m *UnknownMessage) GetCommission() *float64        { return nil }
func (m *UnknownMessage) GetChainID() *uint64            { return nil }
func (m *UnknownMessage) GetSellAmount() *uint64         { return nil }
func (m *UnknownMessage) GetBuyAmount() *uint64          { return nil }
func (m *UnknownMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *UnknownMessage) GetOrderID() *string            { return nil }
func (m *UnknownMessage) GetPrice() *float64             { return nil }
func (m *UnknownMessage) GetParamKey() *string           { return nil }
func (m *UnknownMessage) GetParamValue() *string         { return nil }
func (m *UnknownMessage) GetCommitteeID() *uint64        { return nil }
func (m *UnknownMessage) GetRecipient() *string          { return nil }

// PauseMessage represents pausing a validator
type PauseMessage struct {
	Address string `json:"address"`
}

func (m *PauseMessage) Type() MessageType              { return MsgTypePause }
func (m *PauseMessage) GetSigner() string              { return m.Address }
func (m *PauseMessage) GetCounterparty() *string       { return nil }
func (m *PauseMessage) GetAmount() *uint64             { return nil }
func (m *PauseMessage) GetValidatorAddress() *string   { return nil }
func (m *PauseMessage) GetCommission() *float64        { return nil }
func (m *PauseMessage) GetChainID() *uint64            { return nil }
func (m *PauseMessage) GetSellAmount() *uint64         { return nil }
func (m *PauseMessage) GetBuyAmount() *uint64          { return nil }
func (m *PauseMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *PauseMessage) GetOrderID() *string            { return nil }
func (m *PauseMessage) GetPrice() *float64             { return nil }
func (m *PauseMessage) GetParamKey() *string           { return nil }
func (m *PauseMessage) GetParamValue() *string         { return nil }
func (m *PauseMessage) GetCommitteeID() *uint64        { return nil }
func (m *PauseMessage) GetRecipient() *string          { return nil }

// UnpauseMessage represents unpausing a validator
type UnpauseMessage struct {
	Address string `json:"address"`
}

func (m *UnpauseMessage) Type() MessageType              { return MsgTypeUnpause }
func (m *UnpauseMessage) GetSigner() string              { return m.Address }
func (m *UnpauseMessage) GetCounterparty() *string       { return nil }
func (m *UnpauseMessage) GetAmount() *uint64             { return nil }
func (m *UnpauseMessage) GetValidatorAddress() *string   { return nil }
func (m *UnpauseMessage) GetCommission() *float64        { return nil }
func (m *UnpauseMessage) GetChainID() *uint64            { return nil }
func (m *UnpauseMessage) GetSellAmount() *uint64         { return nil }
func (m *UnpauseMessage) GetBuyAmount() *uint64          { return nil }
func (m *UnpauseMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *UnpauseMessage) GetOrderID() *string            { return nil }
func (m *UnpauseMessage) GetPrice() *float64             { return nil }
func (m *UnpauseMessage) GetParamKey() *string           { return nil }
func (m *UnpauseMessage) GetParamValue() *string         { return nil }
func (m *UnpauseMessage) GetCommitteeID() *uint64        { return nil }
func (m *UnpauseMessage) GetRecipient() *string          { return nil }

// ChangeParameterMessage represents changing a protocol parameter
type ChangeParameterMessage struct {
	ParamKey   string `json:"param_key"`
	ParamValue string `json:"param_value"`
	Signer     string `json:"signer"`
}

func (m *ChangeParameterMessage) Type() MessageType              { return MsgTypeChangeParameter }
func (m *ChangeParameterMessage) GetSigner() string              { return m.Signer }
func (m *ChangeParameterMessage) GetCounterparty() *string       { return nil }
func (m *ChangeParameterMessage) GetAmount() *uint64             { return nil }
func (m *ChangeParameterMessage) GetValidatorAddress() *string   { return nil }
func (m *ChangeParameterMessage) GetCommission() *float64        { return nil }
func (m *ChangeParameterMessage) GetChainID() *uint64            { return nil }
func (m *ChangeParameterMessage) GetSellAmount() *uint64         { return nil }
func (m *ChangeParameterMessage) GetBuyAmount() *uint64          { return nil }
func (m *ChangeParameterMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *ChangeParameterMessage) GetOrderID() *string            { return nil }
func (m *ChangeParameterMessage) GetPrice() *float64             { return nil }
func (m *ChangeParameterMessage) GetParamKey() *string           { return &m.ParamKey }
func (m *ChangeParameterMessage) GetParamValue() *string         { return &m.ParamValue }
func (m *ChangeParameterMessage) GetCommitteeID() *uint64        { return nil }
func (m *ChangeParameterMessage) GetRecipient() *string          { return nil }

// DAOTransferMessage represents a DAO treasury transfer
type DAOTransferMessage struct {
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
	Amount      uint64 `json:"amount"`
}

func (m *DAOTransferMessage) Type() MessageType              { return MsgTypeDAOTransfer }
func (m *DAOTransferMessage) GetSigner() string              { return m.FromAddress }
func (m *DAOTransferMessage) GetCounterparty() *string       { return nil }
func (m *DAOTransferMessage) GetAmount() *uint64             { return &m.Amount }
func (m *DAOTransferMessage) GetValidatorAddress() *string   { return nil }
func (m *DAOTransferMessage) GetCommission() *float64        { return nil }
func (m *DAOTransferMessage) GetChainID() *uint64            { return nil }
func (m *DAOTransferMessage) GetSellAmount() *uint64         { return nil }
func (m *DAOTransferMessage) GetBuyAmount() *uint64          { return nil }
func (m *DAOTransferMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *DAOTransferMessage) GetOrderID() *string            { return nil }
func (m *DAOTransferMessage) GetPrice() *float64             { return nil }
func (m *DAOTransferMessage) GetParamKey() *string           { return nil }
func (m *DAOTransferMessage) GetParamValue() *string         { return nil }
func (m *DAOTransferMessage) GetCommitteeID() *uint64        { return nil }
func (m *DAOTransferMessage) GetRecipient() *string          { return &m.ToAddress }

// CertificateResultsMessage represents certificate results submission
type CertificateResultsMessage struct {
	Signer          string `json:"signer"`
	CertificateData string `json:"certificate_data"`
}

func (m *CertificateResultsMessage) Type() MessageType              { return MsgTypeCertificateResults }
func (m *CertificateResultsMessage) GetSigner() string              { return m.Signer }
func (m *CertificateResultsMessage) GetCounterparty() *string       { return nil }
func (m *CertificateResultsMessage) GetAmount() *uint64             { return nil }
func (m *CertificateResultsMessage) GetValidatorAddress() *string   { return nil }
func (m *CertificateResultsMessage) GetCommission() *float64        { return nil }
func (m *CertificateResultsMessage) GetChainID() *uint64            { return nil }
func (m *CertificateResultsMessage) GetSellAmount() *uint64         { return nil }
func (m *CertificateResultsMessage) GetBuyAmount() *uint64          { return nil }
func (m *CertificateResultsMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *CertificateResultsMessage) GetOrderID() *string            { return nil }
func (m *CertificateResultsMessage) GetPrice() *float64             { return nil }
func (m *CertificateResultsMessage) GetParamKey() *string           { return nil }
func (m *CertificateResultsMessage) GetParamValue() *string         { return nil }
func (m *CertificateResultsMessage) GetCommitteeID() *uint64        { return nil }
func (m *CertificateResultsMessage) GetRecipient() *string          { return nil }

// SubsidyMessage represents a subsidy payment
type SubsidyMessage struct {
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
	Amount      uint64 `json:"amount"`
	CommitteeID uint64 `json:"committee_id"`
}

func (m *SubsidyMessage) Type() MessageType              { return MsgTypeSubsidy }
func (m *SubsidyMessage) GetSigner() string              { return m.FromAddress }
func (m *SubsidyMessage) GetCounterparty() *string       { return nil }
func (m *SubsidyMessage) GetAmount() *uint64             { return &m.Amount }
func (m *SubsidyMessage) GetValidatorAddress() *string   { return nil }
func (m *SubsidyMessage) GetCommission() *float64        { return nil }
func (m *SubsidyMessage) GetChainID() *uint64            { return nil }
func (m *SubsidyMessage) GetSellAmount() *uint64         { return nil }
func (m *SubsidyMessage) GetBuyAmount() *uint64          { return nil }
func (m *SubsidyMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *SubsidyMessage) GetOrderID() *string            { return nil }
func (m *SubsidyMessage) GetPrice() *float64             { return nil }
func (m *SubsidyMessage) GetParamKey() *string           { return nil }
func (m *SubsidyMessage) GetParamValue() *string         { return nil }
func (m *SubsidyMessage) GetCommitteeID() *uint64        { return &m.CommitteeID }
func (m *SubsidyMessage) GetRecipient() *string          { return &m.ToAddress }

// CreateOrderMessage represents creating a new order
type CreateOrderMessage struct {
	Signer     string  `json:"signer"`
	OrderID    string  `json:"order_id"`
	ChainID    uint64  `json:"chain_id"`
	SellAmount uint64  `json:"sell_amount"`
	BuyAmount  uint64  `json:"buy_amount"`
	Price      float64 `json:"price"`
}

func (m *CreateOrderMessage) Type() MessageType              { return MsgTypeCreateOrder }
func (m *CreateOrderMessage) GetSigner() string              { return m.Signer }
func (m *CreateOrderMessage) GetCounterparty() *string       { return nil }
func (m *CreateOrderMessage) GetAmount() *uint64             { return nil }
func (m *CreateOrderMessage) GetValidatorAddress() *string   { return nil }
func (m *CreateOrderMessage) GetCommission() *float64        { return nil }
func (m *CreateOrderMessage) GetChainID() *uint64            { return &m.ChainID }
func (m *CreateOrderMessage) GetSellAmount() *uint64         { return &m.SellAmount }
func (m *CreateOrderMessage) GetBuyAmount() *uint64          { return &m.BuyAmount }
func (m *CreateOrderMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *CreateOrderMessage) GetOrderID() *string            { return &m.OrderID }
func (m *CreateOrderMessage) GetPrice() *float64             { return &m.Price }
func (m *CreateOrderMessage) GetParamKey() *string           { return nil }
func (m *CreateOrderMessage) GetParamValue() *string         { return nil }
func (m *CreateOrderMessage) GetCommitteeID() *uint64        { return nil }
func (m *CreateOrderMessage) GetRecipient() *string          { return nil }

// EditOrderMessage represents editing an existing order
type EditOrderMessage struct {
	Signer  string  `json:"signer"`
	OrderID string  `json:"order_id"`
	Price   float64 `json:"price"`
}

func (m *EditOrderMessage) Type() MessageType              { return MsgTypeEditOrder }
func (m *EditOrderMessage) GetSigner() string              { return m.Signer }
func (m *EditOrderMessage) GetCounterparty() *string       { return nil }
func (m *EditOrderMessage) GetAmount() *uint64             { return nil }
func (m *EditOrderMessage) GetValidatorAddress() *string   { return nil }
func (m *EditOrderMessage) GetCommission() *float64        { return nil }
func (m *EditOrderMessage) GetChainID() *uint64            { return nil }
func (m *EditOrderMessage) GetSellAmount() *uint64         { return nil }
func (m *EditOrderMessage) GetBuyAmount() *uint64          { return nil }
func (m *EditOrderMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *EditOrderMessage) GetOrderID() *string            { return &m.OrderID }
func (m *EditOrderMessage) GetPrice() *float64             { return &m.Price }
func (m *EditOrderMessage) GetParamKey() *string           { return nil }
func (m *EditOrderMessage) GetParamValue() *string         { return nil }
func (m *EditOrderMessage) GetCommitteeID() *uint64        { return nil }
func (m *EditOrderMessage) GetRecipient() *string          { return nil }

// DeleteOrderMessage represents deleting an order
type DeleteOrderMessage struct {
	Signer  string `json:"signer"`
	OrderID string `json:"order_id"`
}

func (m *DeleteOrderMessage) Type() MessageType              { return MsgTypeDeleteOrder }
func (m *DeleteOrderMessage) GetSigner() string              { return m.Signer }
func (m *DeleteOrderMessage) GetCounterparty() *string       { return nil }
func (m *DeleteOrderMessage) GetAmount() *uint64             { return nil }
func (m *DeleteOrderMessage) GetValidatorAddress() *string   { return nil }
func (m *DeleteOrderMessage) GetCommission() *float64        { return nil }
func (m *DeleteOrderMessage) GetChainID() *uint64            { return nil }
func (m *DeleteOrderMessage) GetSellAmount() *uint64         { return nil }
func (m *DeleteOrderMessage) GetBuyAmount() *uint64          { return nil }
func (m *DeleteOrderMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *DeleteOrderMessage) GetOrderID() *string            { return &m.OrderID }
func (m *DeleteOrderMessage) GetPrice() *float64             { return nil }
func (m *DeleteOrderMessage) GetParamKey() *string           { return nil }
func (m *DeleteOrderMessage) GetParamValue() *string         { return nil }
func (m *DeleteOrderMessage) GetCommitteeID() *uint64        { return nil }
func (m *DeleteOrderMessage) GetRecipient() *string          { return nil }

// DexLimitOrderMessage represents a DEX limit order
type DexLimitOrderMessage struct {
	From       string  `json:"from"`
	ChainID    uint64  `json:"chain_id"`
	SellAmount uint64  `json:"sell_amount"`
	BuyAmount  uint64  `json:"buy_amount"`
	Price      float64 `json:"price"`
}

func (m *DexLimitOrderMessage) Type() MessageType              { return MsgTypeDexLimitOrder }
func (m *DexLimitOrderMessage) GetSigner() string              { return m.From }
func (m *DexLimitOrderMessage) GetCounterparty() *string       { return nil }
func (m *DexLimitOrderMessage) GetAmount() *uint64             { return nil }
func (m *DexLimitOrderMessage) GetValidatorAddress() *string   { return nil }
func (m *DexLimitOrderMessage) GetCommission() *float64        { return nil }
func (m *DexLimitOrderMessage) GetChainID() *uint64            { return &m.ChainID }
func (m *DexLimitOrderMessage) GetSellAmount() *uint64         { return &m.SellAmount }
func (m *DexLimitOrderMessage) GetBuyAmount() *uint64          { return &m.BuyAmount }
func (m *DexLimitOrderMessage) GetLiquidityAmount() *uint64    { return nil }
func (m *DexLimitOrderMessage) GetOrderID() *string            { return nil }
func (m *DexLimitOrderMessage) GetPrice() *float64             { return &m.Price }
func (m *DexLimitOrderMessage) GetParamKey() *string           { return nil }
func (m *DexLimitOrderMessage) GetParamValue() *string         { return nil }
func (m *DexLimitOrderMessage) GetCommitteeID() *uint64        { return nil }
func (m *DexLimitOrderMessage) GetRecipient() *string          { return nil }

// DexLiquidityDepositMessage represents adding liquidity to a DEX pool
type DexLiquidityDepositMessage struct {
	From    string `json:"from"`
	ChainID uint64 `json:"chain_id"`
	Amount  uint64 `json:"amount"`
}

func (m *DexLiquidityDepositMessage) Type() MessageType              { return MsgTypeDexLiquidityDeposit }
func (m *DexLiquidityDepositMessage) GetSigner() string              { return m.From }
func (m *DexLiquidityDepositMessage) GetCounterparty() *string       { return nil }
func (m *DexLiquidityDepositMessage) GetAmount() *uint64             { return nil }
func (m *DexLiquidityDepositMessage) GetValidatorAddress() *string   { return nil }
func (m *DexLiquidityDepositMessage) GetCommission() *float64        { return nil }
func (m *DexLiquidityDepositMessage) GetChainID() *uint64            { return &m.ChainID }
func (m *DexLiquidityDepositMessage) GetSellAmount() *uint64         { return nil }
func (m *DexLiquidityDepositMessage) GetBuyAmount() *uint64          { return nil }
func (m *DexLiquidityDepositMessage) GetLiquidityAmount() *uint64    { return &m.Amount }
func (m *DexLiquidityDepositMessage) GetOrderID() *string            { return nil }
func (m *DexLiquidityDepositMessage) GetPrice() *float64             { return nil }
func (m *DexLiquidityDepositMessage) GetParamKey() *string           { return nil }
func (m *DexLiquidityDepositMessage) GetParamValue() *string         { return nil }
func (m *DexLiquidityDepositMessage) GetCommitteeID() *uint64        { return nil }
func (m *DexLiquidityDepositMessage) GetRecipient() *string          { return nil }

// DexLiquidityWithdrawMessage represents removing liquidity from a DEX pool
type DexLiquidityWithdrawMessage struct {
	From    string `json:"from"`
	ChainID uint64 `json:"chain_id"`
	Amount  uint64 `json:"amount"`
}

func (m *DexLiquidityWithdrawMessage) Type() MessageType              { return MsgTypeDexLiquidityWithdraw }
func (m *DexLiquidityWithdrawMessage) GetSigner() string              { return m.From }
func (m *DexLiquidityWithdrawMessage) GetCounterparty() *string       { return nil }
func (m *DexLiquidityWithdrawMessage) GetAmount() *uint64             { return nil }
func (m *DexLiquidityWithdrawMessage) GetValidatorAddress() *string   { return nil }
func (m *DexLiquidityWithdrawMessage) GetCommission() *float64        { return nil }
func (m *DexLiquidityWithdrawMessage) GetChainID() *uint64            { return &m.ChainID }
func (m *DexLiquidityWithdrawMessage) GetSellAmount() *uint64         { return nil }
func (m *DexLiquidityWithdrawMessage) GetBuyAmount() *uint64          { return nil }
func (m *DexLiquidityWithdrawMessage) GetLiquidityAmount() *uint64    { return &m.Amount }
func (m *DexLiquidityWithdrawMessage) GetOrderID() *string            { return nil }
func (m *DexLiquidityWithdrawMessage) GetPrice() *float64             { return nil }
func (m *DexLiquidityWithdrawMessage) GetParamKey() *string           { return nil }
func (m *DexLiquidityWithdrawMessage) GetParamValue() *string         { return nil }
func (m *DexLiquidityWithdrawMessage) GetCommitteeID() *uint64        { return nil }
func (m *DexLiquidityWithdrawMessage) GetRecipient() *string          { return nil }
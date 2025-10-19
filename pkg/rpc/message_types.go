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
)

// Message is the interface all transaction message types implement.
// This enables polymorphic handling of different transaction types while
// extracting common queryable fields (signer, counterparty, amount).
type Message interface {
	Type() MessageType
	GetSigner() string
	GetCounterparty() *string // nil for messages without counterparty
	GetAmount() *uint64       // nil for messages without amount
}

// SendMessage represents a token transfer
type SendMessage struct {
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
	Amount      uint64 `json:"amount"`
	Memo        string `json:"memo,omitempty"`
}

func (m *SendMessage) Type() MessageType        { return MsgTypeSend }
func (m *SendMessage) GetSigner() string        { return m.FromAddress }
func (m *SendMessage) GetCounterparty() *string { return &m.ToAddress }
func (m *SendMessage) GetAmount() *uint64       { return &m.Amount }

// DelegateMessage represents delegation to a validator
type DelegateMessage struct {
	Delegator        string `json:"delegator"`
	ValidatorAddress string `json:"validator_address"`
	Amount           uint64 `json:"amount"`
	Memo             string `json:"memo,omitempty"`
}

func (m *DelegateMessage) Type() MessageType        { return MsgTypeDelegate }
func (m *DelegateMessage) GetSigner() string        { return m.Delegator }
func (m *DelegateMessage) GetCounterparty() *string { return &m.ValidatorAddress }
func (m *DelegateMessage) GetAmount() *uint64       { return &m.Amount }

// UndelegateMessage represents undelegation from a validator
type UndelegateMessage struct {
	Delegator        string `json:"delegator"`
	ValidatorAddress string `json:"validator_address"`
	Amount           uint64 `json:"amount"`
}

func (m *UndelegateMessage) Type() MessageType        { return MsgTypeUndelegate }
func (m *UndelegateMessage) GetSigner() string        { return m.Delegator }
func (m *UndelegateMessage) GetCounterparty() *string { return &m.ValidatorAddress }
func (m *UndelegateMessage) GetAmount() *uint64       { return &m.Amount }

// StakeMessage represents staking to a pool
type StakeMessage struct {
	Staker     string  `json:"staker"`
	Pool       string  `json:"pool"`
	Amount     uint64  `json:"amount"`
	LockPeriod *uint32 `json:"lock_period,omitempty"` // Optional lock period
}

func (m *StakeMessage) Type() MessageType        { return MsgTypeStake }
func (m *StakeMessage) GetSigner() string        { return m.Staker }
func (m *StakeMessage) GetCounterparty() *string { return &m.Pool }
func (m *StakeMessage) GetAmount() *uint64       { return &m.Amount }

// UnstakeMessage represents unstaking from a pool
type UnstakeMessage struct {
	Staker string `json:"staker"`
	Pool   string `json:"pool"`
	Amount uint64 `json:"amount"`
}

func (m *UnstakeMessage) Type() MessageType        { return MsgTypeUnstake }
func (m *UnstakeMessage) GetSigner() string        { return m.Staker }
func (m *UnstakeMessage) GetCounterparty() *string { return &m.Pool }
func (m *UnstakeMessage) GetAmount() *uint64       { return &m.Amount }

// EditStakeMessage represents editing stake parameters
type EditStakeMessage struct {
	Staker     string  `json:"staker"`
	Pool       string  `json:"pool"`
	Amount     *uint64 `json:"amount,omitempty"`      // nil if not changing amount
	LockPeriod *uint32 `json:"lock_period,omitempty"` // nil if not changing lock
}

func (m *EditStakeMessage) Type() MessageType        { return MsgTypeEditStake }
func (m *EditStakeMessage) GetSigner() string        { return m.Staker }
func (m *EditStakeMessage) GetCounterparty() *string { return &m.Pool }
func (m *EditStakeMessage) GetAmount() *uint64       { return m.Amount }

// VoteMessage represents a governance vote
type VoteMessage struct {
	Voter      string `json:"voter"`
	ProposalID uint64 `json:"proposal_id"`
	Option     string `json:"option"` // "yes", "no", "abstain", "veto"
	Memo       string `json:"memo,omitempty"`
}

func (m *VoteMessage) Type() MessageType        { return MsgTypeVote }
func (m *VoteMessage) GetSigner() string        { return m.Voter }
func (m *VoteMessage) GetCounterparty() *string { return nil } // Votes have no counterparty
func (m *VoteMessage) GetAmount() *uint64       { return nil } // Votes have no amount

// ProposalMessage represents creating a governance proposal
type ProposalMessage struct {
	Proposer    string `json:"proposer"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Deposit     uint64 `json:"deposit"`
}

func (m *ProposalMessage) Type() MessageType        { return MsgTypeProposal }
func (m *ProposalMessage) GetSigner() string        { return m.Proposer }
func (m *ProposalMessage) GetCounterparty() *string { return nil }
func (m *ProposalMessage) GetAmount() *uint64       { return &m.Deposit }

// ContractMessage represents a smart contract interaction
type ContractMessage struct {
	Caller          string  `json:"caller"`
	ContractAddress string  `json:"contract_address"`
	Method          string  `json:"method"`
	CallData        string  `json:"call_data"` // JSON or hex-encoded
	Value           *uint64 `json:"value,omitempty"`
}

func (m *ContractMessage) Type() MessageType        { return MsgTypeContract }
func (m *ContractMessage) GetSigner() string        { return m.Caller }
func (m *ContractMessage) GetCounterparty() *string { return &m.ContractAddress }
func (m *ContractMessage) GetAmount() *uint64       { return m.Value }

// SystemMessage represents a system-level transaction
type SystemMessage struct {
	Executor string                 `json:"executor"`
	Action   string                 `json:"action"`
	Params   map[string]interface{} `json:"params,omitempty"`
}

func (m *SystemMessage) Type() MessageType        { return MsgTypeSystem }
func (m *SystemMessage) GetSigner() string        { return m.Executor }
func (m *SystemMessage) GetCounterparty() *string { return nil }
func (m *SystemMessage) GetAmount() *uint64       { return nil }

// UnknownMessage represents an unrecognized transaction type.
// This is a fallback for transaction types we don't yet support.
type UnknownMessage struct {
	Signer string                 `json:"signer"`
	Data   map[string]interface{} `json:"data"`
}

func (m *UnknownMessage) Type() MessageType        { return MsgTypeUnknown }
func (m *UnknownMessage) GetSigner() string        { return m.Signer }
func (m *UnknownMessage) GetCounterparty() *string { return nil }
func (m *UnknownMessage) GetAmount() *uint64       { return nil }
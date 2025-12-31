package rpc

// MessageType identifies the type of transaction
type MessageType string

// StartPollMemo represents the memo structure for starting a governance poll.
// Poll hash is SHA-256 hex string (64 chars), endHeight must be > current height.
type StartPollMemo struct {
	StartPoll string `json:"startPoll"`
	Url       string `json:"url,omitempty"`
	EndHeight uint64 `json:"endHeight"`
}

// VotePollMemo represents the memo structure for voting on a poll.
// Poll hash must match an existing poll's startPoll hash.
type VotePollMemo struct {
	VotePoll string `json:"votePoll"`
	Approve  bool   `json:"approve"`
}

// LockOrderMemo represents the memo structure for locking/reserving an order.
// OrderID is first 20 bytes of order creation tx hash (hex).
type LockOrderMemo struct {
	OrderID             string `json:"orderID"`
	ChainID             uint64 `json:"chainID"`
	BuyerReceiveAddress string `json:"buyerReceiveAddress"`
	BuyerSendAddress    string `json:"buyerSendAddress"`
	BuyerChainDeadline  uint64 `json:"buyerChainDeadline"`
}

// CloseOrderMemo represents the memo structure for closing/completing an order.
// CloseOrder flag must be true to identify as close order operation.
type CloseOrderMemo struct {
	OrderID    string `json:"orderID"`
	ChainID    uint64 `json:"chainID"`
	CloseOrder bool   `json:"closeOrder"`
}

const (
	MsgTypeUnknown MessageType = "unknown"

	MsgTypeSend MessageType = "send" // -

	MsgTypeStake     MessageType = "stake"      // -
	MsgTypeUnstake   MessageType = "unstake"    // -
	MsgTypeEditStake MessageType = "edit_stake" // -

	MsgTypePause           MessageType = "pause"           // -
	MsgTypeUnpause         MessageType = "unpause"         // -
	MsgTypeChangeParameter MessageType = "changeParameter" // -
	MsgTypeDAOTransfer     MessageType = "daoTransfer"     // -

	MsgTypeCertificateResults MessageType = "certificateResults" // NOT IN POPULATOR

	MsgTypeSubsidy MessageType = "subsidy" // -

	MsgTypeCreateOrder MessageType = "createOrder" // -
	MsgTypeEditOrder   MessageType = "editOrder"   // -
	MsgTypeDeleteOrder MessageType = "deleteOrder" // -

	MsgTypeDexLimitOrder        MessageType = "dexLimitOrder"        // -
	MsgTypeDexLiquidityDeposit  MessageType = "dexLiquidityDeposit"  // -
	MsgTypeDexLiquidityWithdraw MessageType = "dexLiquidityWithdraw" // -

	// --- Memo-based transaction types (detected from send tx memo field)

	MsgTypeStartPoll  MessageType = "startPoll"  // send tx with startPoll memo - creates governance poll
	MsgTypeVotePoll   MessageType = "votePoll"   // send tx with votePoll memo - votes on poll
	MsgTypeLockOrder  MessageType = "lockOrder"  // send tx with lockOrder memo - reserves order
	MsgTypeCloseOrder MessageType = "closeOrder" // send tx with closeOrder memo - completes order
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
	GetPollHash() *string            // For startPoll, votePoll
	GetBuyerReceiveAddress() *string // For lockOrder
	GetBuyerSendAddress() *string    // For lockOrder
	GetBuyerChainDeadline() *uint64  // For lockOrder
}

// SendMessage represents a token transfer
type SendMessage struct {
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
	Amount      uint64 `json:"amount"`
	Memo        string `json:"memo,omitempty"`
}

func (m *SendMessage) Type() MessageType               { return MsgTypeSend }
func (m *SendMessage) GetSigner() string               { return m.FromAddress }
func (m *SendMessage) GetCounterparty() *string        { return &m.ToAddress }
func (m *SendMessage) GetAmount() *uint64              { return &m.Amount }
func (m *SendMessage) GetValidatorAddress() *string    { return nil }
func (m *SendMessage) GetCommission() *float64         { return nil }
func (m *SendMessage) GetChainID() *uint64             { return nil }
func (m *SendMessage) GetSellAmount() *uint64          { return nil }
func (m *SendMessage) GetBuyAmount() *uint64           { return nil }
func (m *SendMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *SendMessage) GetOrderID() *string             { return nil }
func (m *SendMessage) GetPrice() *float64              { return nil }
func (m *SendMessage) GetParamKey() *string            { return nil }
func (m *SendMessage) GetParamValue() *string          { return nil }
func (m *SendMessage) GetCommitteeID() *uint64         { return nil }
func (m *SendMessage) GetRecipient() *string           { return nil }
func (m *SendMessage) GetPollHash() *string            { return nil }
func (m *SendMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *SendMessage) GetBuyerSendAddress() *string    { return nil }
func (m *SendMessage) GetBuyerChainDeadline() *uint64  { return nil }

// StakeMessage represents staking to a pool
type StakeMessage struct {
	Staker     string  `json:"staker"`
	Pool       string  `json:"pool"`
	Amount     uint64  `json:"amount"`
	LockPeriod *uint32 `json:"lock_period,omitempty"` // Optional lock period
}

func (m *StakeMessage) Type() MessageType               { return MsgTypeStake }
func (m *StakeMessage) GetSigner() string               { return m.Staker }
func (m *StakeMessage) GetCounterparty() *string        { return &m.Pool }
func (m *StakeMessage) GetAmount() *uint64              { return &m.Amount }
func (m *StakeMessage) GetValidatorAddress() *string    { return nil }
func (m *StakeMessage) GetCommission() *float64         { return nil }
func (m *StakeMessage) GetChainID() *uint64             { return nil }
func (m *StakeMessage) GetSellAmount() *uint64          { return nil }
func (m *StakeMessage) GetBuyAmount() *uint64           { return nil }
func (m *StakeMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *StakeMessage) GetOrderID() *string             { return nil }
func (m *StakeMessage) GetPrice() *float64              { return nil }
func (m *StakeMessage) GetParamKey() *string            { return nil }
func (m *StakeMessage) GetParamValue() *string          { return nil }
func (m *StakeMessage) GetCommitteeID() *uint64         { return nil }
func (m *StakeMessage) GetRecipient() *string           { return nil }
func (m *StakeMessage) GetPollHash() *string            { return nil }
func (m *StakeMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *StakeMessage) GetBuyerSendAddress() *string    { return nil }
func (m *StakeMessage) GetBuyerChainDeadline() *uint64  { return nil }

// UnstakeMessage represents unstaking from a pool
type UnstakeMessage struct {
	Staker string `json:"staker"`
	Pool   string `json:"pool"`
	Amount uint64 `json:"amount"`
}

func (m *UnstakeMessage) Type() MessageType               { return MsgTypeUnstake }
func (m *UnstakeMessage) GetSigner() string               { return m.Staker }
func (m *UnstakeMessage) GetCounterparty() *string        { return &m.Pool }
func (m *UnstakeMessage) GetAmount() *uint64              { return &m.Amount }
func (m *UnstakeMessage) GetValidatorAddress() *string    { return nil }
func (m *UnstakeMessage) GetCommission() *float64         { return nil }
func (m *UnstakeMessage) GetChainID() *uint64             { return nil }
func (m *UnstakeMessage) GetSellAmount() *uint64          { return nil }
func (m *UnstakeMessage) GetBuyAmount() *uint64           { return nil }
func (m *UnstakeMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *UnstakeMessage) GetOrderID() *string             { return nil }
func (m *UnstakeMessage) GetPrice() *float64              { return nil }
func (m *UnstakeMessage) GetParamKey() *string            { return nil }
func (m *UnstakeMessage) GetParamValue() *string          { return nil }
func (m *UnstakeMessage) GetCommitteeID() *uint64         { return nil }
func (m *UnstakeMessage) GetRecipient() *string           { return nil }
func (m *UnstakeMessage) GetPollHash() *string            { return nil }
func (m *UnstakeMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *UnstakeMessage) GetBuyerSendAddress() *string    { return nil }
func (m *UnstakeMessage) GetBuyerChainDeadline() *uint64  { return nil }

// EditStakeMessage represents editing stake parameters
type EditStakeMessage struct {
	Staker     string  `json:"staker"`
	Pool       string  `json:"pool"`
	Amount     *uint64 `json:"amount,omitempty"`      // nil if not changing amount
	LockPeriod *uint32 `json:"lock_period,omitempty"` // nil if not changing lock
}

func (m *EditStakeMessage) Type() MessageType               { return MsgTypeEditStake }
func (m *EditStakeMessage) GetSigner() string               { return m.Staker }
func (m *EditStakeMessage) GetCounterparty() *string        { return &m.Pool }
func (m *EditStakeMessage) GetAmount() *uint64              { return m.Amount }
func (m *EditStakeMessage) GetValidatorAddress() *string    { return nil }
func (m *EditStakeMessage) GetCommission() *float64         { return nil }
func (m *EditStakeMessage) GetChainID() *uint64             { return nil }
func (m *EditStakeMessage) GetSellAmount() *uint64          { return nil }
func (m *EditStakeMessage) GetBuyAmount() *uint64           { return nil }
func (m *EditStakeMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *EditStakeMessage) GetOrderID() *string             { return nil }
func (m *EditStakeMessage) GetPrice() *float64              { return nil }
func (m *EditStakeMessage) GetParamKey() *string            { return nil }
func (m *EditStakeMessage) GetParamValue() *string          { return nil }
func (m *EditStakeMessage) GetCommitteeID() *uint64         { return nil }
func (m *EditStakeMessage) GetRecipient() *string           { return nil }
func (m *EditStakeMessage) GetPollHash() *string            { return nil }
func (m *EditStakeMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *EditStakeMessage) GetBuyerSendAddress() *string    { return nil }
func (m *EditStakeMessage) GetBuyerChainDeadline() *uint64  { return nil }

// UnknownMessage represents an unrecognized transaction type.
// This is a fallback for transaction types we don't yet support.
type UnknownMessage struct {
	Signer string                 `json:"signer"`
	Data   map[string]interface{} `json:"data"`
}

func (m *UnknownMessage) Type() MessageType               { return MsgTypeUnknown }
func (m *UnknownMessage) GetSigner() string               { return m.Signer }
func (m *UnknownMessage) GetCounterparty() *string        { return nil }
func (m *UnknownMessage) GetAmount() *uint64              { return nil }
func (m *UnknownMessage) GetValidatorAddress() *string    { return nil }
func (m *UnknownMessage) GetCommission() *float64         { return nil }
func (m *UnknownMessage) GetChainID() *uint64             { return nil }
func (m *UnknownMessage) GetSellAmount() *uint64          { return nil }
func (m *UnknownMessage) GetBuyAmount() *uint64           { return nil }
func (m *UnknownMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *UnknownMessage) GetOrderID() *string             { return nil }
func (m *UnknownMessage) GetPrice() *float64              { return nil }
func (m *UnknownMessage) GetParamKey() *string            { return nil }
func (m *UnknownMessage) GetParamValue() *string          { return nil }
func (m *UnknownMessage) GetCommitteeID() *uint64         { return nil }
func (m *UnknownMessage) GetRecipient() *string           { return nil }
func (m *UnknownMessage) GetPollHash() *string            { return nil }
func (m *UnknownMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *UnknownMessage) GetBuyerSendAddress() *string    { return nil }
func (m *UnknownMessage) GetBuyerChainDeadline() *uint64  { return nil }

// PauseMessage represents pausing a validator
type PauseMessage struct {
	Address string `json:"address"`
}

func (m *PauseMessage) Type() MessageType               { return MsgTypePause }
func (m *PauseMessage) GetSigner() string               { return m.Address }
func (m *PauseMessage) GetCounterparty() *string        { return nil }
func (m *PauseMessage) GetAmount() *uint64              { return nil }
func (m *PauseMessage) GetValidatorAddress() *string    { return nil }
func (m *PauseMessage) GetCommission() *float64         { return nil }
func (m *PauseMessage) GetChainID() *uint64             { return nil }
func (m *PauseMessage) GetSellAmount() *uint64          { return nil }
func (m *PauseMessage) GetBuyAmount() *uint64           { return nil }
func (m *PauseMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *PauseMessage) GetOrderID() *string             { return nil }
func (m *PauseMessage) GetPrice() *float64              { return nil }
func (m *PauseMessage) GetParamKey() *string            { return nil }
func (m *PauseMessage) GetParamValue() *string          { return nil }
func (m *PauseMessage) GetCommitteeID() *uint64         { return nil }
func (m *PauseMessage) GetRecipient() *string           { return nil }
func (m *PauseMessage) GetPollHash() *string            { return nil }
func (m *PauseMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *PauseMessage) GetBuyerSendAddress() *string    { return nil }
func (m *PauseMessage) GetBuyerChainDeadline() *uint64  { return nil }

// UnpauseMessage represents unpausing a validator
type UnpauseMessage struct {
	Address string `json:"address"`
}

func (m *UnpauseMessage) Type() MessageType               { return MsgTypeUnpause }
func (m *UnpauseMessage) GetSigner() string               { return m.Address }
func (m *UnpauseMessage) GetCounterparty() *string        { return nil }
func (m *UnpauseMessage) GetAmount() *uint64              { return nil }
func (m *UnpauseMessage) GetValidatorAddress() *string    { return nil }
func (m *UnpauseMessage) GetCommission() *float64         { return nil }
func (m *UnpauseMessage) GetChainID() *uint64             { return nil }
func (m *UnpauseMessage) GetSellAmount() *uint64          { return nil }
func (m *UnpauseMessage) GetBuyAmount() *uint64           { return nil }
func (m *UnpauseMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *UnpauseMessage) GetOrderID() *string             { return nil }
func (m *UnpauseMessage) GetPrice() *float64              { return nil }
func (m *UnpauseMessage) GetParamKey() *string            { return nil }
func (m *UnpauseMessage) GetParamValue() *string          { return nil }
func (m *UnpauseMessage) GetCommitteeID() *uint64         { return nil }
func (m *UnpauseMessage) GetRecipient() *string           { return nil }
func (m *UnpauseMessage) GetPollHash() *string            { return nil }
func (m *UnpauseMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *UnpauseMessage) GetBuyerSendAddress() *string    { return nil }
func (m *UnpauseMessage) GetBuyerChainDeadline() *uint64  { return nil }

// ChangeParameterMessage represents changing a protocol parameter
type ChangeParameterMessage struct {
	ParamKey   string `json:"param_key"`
	ParamValue string `json:"param_value"`
	Signer     string `json:"signer"`
}

func (m *ChangeParameterMessage) Type() MessageType               { return MsgTypeChangeParameter }
func (m *ChangeParameterMessage) GetSigner() string               { return m.Signer }
func (m *ChangeParameterMessage) GetCounterparty() *string        { return nil }
func (m *ChangeParameterMessage) GetAmount() *uint64              { return nil }
func (m *ChangeParameterMessage) GetValidatorAddress() *string    { return nil }
func (m *ChangeParameterMessage) GetCommission() *float64         { return nil }
func (m *ChangeParameterMessage) GetChainID() *uint64             { return nil }
func (m *ChangeParameterMessage) GetSellAmount() *uint64          { return nil }
func (m *ChangeParameterMessage) GetBuyAmount() *uint64           { return nil }
func (m *ChangeParameterMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *ChangeParameterMessage) GetOrderID() *string             { return nil }
func (m *ChangeParameterMessage) GetPrice() *float64              { return nil }
func (m *ChangeParameterMessage) GetParamKey() *string            { return &m.ParamKey }
func (m *ChangeParameterMessage) GetParamValue() *string          { return &m.ParamValue }
func (m *ChangeParameterMessage) GetCommitteeID() *uint64         { return nil }
func (m *ChangeParameterMessage) GetRecipient() *string           { return nil }
func (m *ChangeParameterMessage) GetPollHash() *string            { return nil }
func (m *ChangeParameterMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *ChangeParameterMessage) GetBuyerSendAddress() *string    { return nil }
func (m *ChangeParameterMessage) GetBuyerChainDeadline() *uint64  { return nil }

// DAOTransferMessage represents a DAO treasury transfer
type DAOTransferMessage struct {
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
	Amount      uint64 `json:"amount"`
}

func (m *DAOTransferMessage) Type() MessageType               { return MsgTypeDAOTransfer }
func (m *DAOTransferMessage) GetSigner() string               { return m.FromAddress }
func (m *DAOTransferMessage) GetCounterparty() *string        { return nil }
func (m *DAOTransferMessage) GetAmount() *uint64              { return &m.Amount }
func (m *DAOTransferMessage) GetValidatorAddress() *string    { return nil }
func (m *DAOTransferMessage) GetCommission() *float64         { return nil }
func (m *DAOTransferMessage) GetChainID() *uint64             { return nil }
func (m *DAOTransferMessage) GetSellAmount() *uint64          { return nil }
func (m *DAOTransferMessage) GetBuyAmount() *uint64           { return nil }
func (m *DAOTransferMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *DAOTransferMessage) GetOrderID() *string             { return nil }
func (m *DAOTransferMessage) GetPrice() *float64              { return nil }
func (m *DAOTransferMessage) GetParamKey() *string            { return nil }
func (m *DAOTransferMessage) GetParamValue() *string          { return nil }
func (m *DAOTransferMessage) GetCommitteeID() *uint64         { return nil }
func (m *DAOTransferMessage) GetRecipient() *string           { return &m.ToAddress }
func (m *DAOTransferMessage) GetPollHash() *string            { return nil }
func (m *DAOTransferMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *DAOTransferMessage) GetBuyerSendAddress() *string    { return nil }
func (m *DAOTransferMessage) GetBuyerChainDeadline() *uint64  { return nil }

// CertificateResultsMessage represents certificate results submission
type CertificateResultsMessage struct {
	Signer          string `json:"signer"`
	CertificateData string `json:"certificate_data"`
}

func (m *CertificateResultsMessage) Type() MessageType               { return MsgTypeCertificateResults }
func (m *CertificateResultsMessage) GetSigner() string               { return m.Signer }
func (m *CertificateResultsMessage) GetCounterparty() *string        { return nil }
func (m *CertificateResultsMessage) GetAmount() *uint64              { return nil }
func (m *CertificateResultsMessage) GetValidatorAddress() *string    { return nil }
func (m *CertificateResultsMessage) GetCommission() *float64         { return nil }
func (m *CertificateResultsMessage) GetChainID() *uint64             { return nil }
func (m *CertificateResultsMessage) GetSellAmount() *uint64          { return nil }
func (m *CertificateResultsMessage) GetBuyAmount() *uint64           { return nil }
func (m *CertificateResultsMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *CertificateResultsMessage) GetOrderID() *string             { return nil }
func (m *CertificateResultsMessage) GetPrice() *float64              { return nil }
func (m *CertificateResultsMessage) GetParamKey() *string            { return nil }
func (m *CertificateResultsMessage) GetParamValue() *string          { return nil }
func (m *CertificateResultsMessage) GetCommitteeID() *uint64         { return nil }
func (m *CertificateResultsMessage) GetRecipient() *string           { return nil }
func (m *CertificateResultsMessage) GetPollHash() *string            { return nil }
func (m *CertificateResultsMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *CertificateResultsMessage) GetBuyerSendAddress() *string    { return nil }
func (m *CertificateResultsMessage) GetBuyerChainDeadline() *uint64  { return nil }

// SubsidyMessage represents a subsidy payment
type SubsidyMessage struct {
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
	Amount      uint64 `json:"amount"`
	CommitteeID uint64 `json:"committee_id"`
}

func (m *SubsidyMessage) Type() MessageType               { return MsgTypeSubsidy }
func (m *SubsidyMessage) GetSigner() string               { return m.FromAddress }
func (m *SubsidyMessage) GetCounterparty() *string        { return nil }
func (m *SubsidyMessage) GetAmount() *uint64              { return &m.Amount }
func (m *SubsidyMessage) GetValidatorAddress() *string    { return nil }
func (m *SubsidyMessage) GetCommission() *float64         { return nil }
func (m *SubsidyMessage) GetChainID() *uint64             { return nil }
func (m *SubsidyMessage) GetSellAmount() *uint64          { return nil }
func (m *SubsidyMessage) GetBuyAmount() *uint64           { return nil }
func (m *SubsidyMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *SubsidyMessage) GetOrderID() *string             { return nil }
func (m *SubsidyMessage) GetPrice() *float64              { return nil }
func (m *SubsidyMessage) GetParamKey() *string            { return nil }
func (m *SubsidyMessage) GetParamValue() *string          { return nil }
func (m *SubsidyMessage) GetCommitteeID() *uint64         { return &m.CommitteeID }
func (m *SubsidyMessage) GetRecipient() *string           { return &m.ToAddress }
func (m *SubsidyMessage) GetPollHash() *string            { return nil }
func (m *SubsidyMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *SubsidyMessage) GetBuyerSendAddress() *string    { return nil }
func (m *SubsidyMessage) GetBuyerChainDeadline() *uint64  { return nil }

// CreateOrderMessage represents creating a new order
type CreateOrderMessage struct {
	Signer     string  `json:"signer"`
	OrderID    string  `json:"order_id"`
	ChainID    uint64  `json:"chain_id"`
	SellAmount uint64  `json:"sell_amount"`
	BuyAmount  uint64  `json:"buy_amount"`
	Price      float64 `json:"price"`
}

func (m *CreateOrderMessage) Type() MessageType               { return MsgTypeCreateOrder }
func (m *CreateOrderMessage) GetSigner() string               { return m.Signer }
func (m *CreateOrderMessage) GetCounterparty() *string        { return nil }
func (m *CreateOrderMessage) GetAmount() *uint64              { return nil }
func (m *CreateOrderMessage) GetValidatorAddress() *string    { return nil }
func (m *CreateOrderMessage) GetCommission() *float64         { return nil }
func (m *CreateOrderMessage) GetChainID() *uint64             { return &m.ChainID }
func (m *CreateOrderMessage) GetSellAmount() *uint64          { return &m.SellAmount }
func (m *CreateOrderMessage) GetBuyAmount() *uint64           { return &m.BuyAmount }
func (m *CreateOrderMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *CreateOrderMessage) GetOrderID() *string             { return &m.OrderID }
func (m *CreateOrderMessage) GetPrice() *float64              { return &m.Price }
func (m *CreateOrderMessage) GetParamKey() *string            { return nil }
func (m *CreateOrderMessage) GetParamValue() *string          { return nil }
func (m *CreateOrderMessage) GetCommitteeID() *uint64         { return nil }
func (m *CreateOrderMessage) GetRecipient() *string           { return nil }
func (m *CreateOrderMessage) GetPollHash() *string            { return nil }
func (m *CreateOrderMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *CreateOrderMessage) GetBuyerSendAddress() *string    { return nil }
func (m *CreateOrderMessage) GetBuyerChainDeadline() *uint64  { return nil }

// EditOrderMessage represents editing an existing order
type EditOrderMessage struct {
	Signer  string  `json:"signer"`
	OrderID string  `json:"order_id"`
	Price   float64 `json:"price"`
}

func (m *EditOrderMessage) Type() MessageType               { return MsgTypeEditOrder }
func (m *EditOrderMessage) GetSigner() string               { return m.Signer }
func (m *EditOrderMessage) GetCounterparty() *string        { return nil }
func (m *EditOrderMessage) GetAmount() *uint64              { return nil }
func (m *EditOrderMessage) GetValidatorAddress() *string    { return nil }
func (m *EditOrderMessage) GetCommission() *float64         { return nil }
func (m *EditOrderMessage) GetChainID() *uint64             { return nil }
func (m *EditOrderMessage) GetSellAmount() *uint64          { return nil }
func (m *EditOrderMessage) GetBuyAmount() *uint64           { return nil }
func (m *EditOrderMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *EditOrderMessage) GetOrderID() *string             { return &m.OrderID }
func (m *EditOrderMessage) GetPrice() *float64              { return &m.Price }
func (m *EditOrderMessage) GetParamKey() *string            { return nil }
func (m *EditOrderMessage) GetParamValue() *string          { return nil }
func (m *EditOrderMessage) GetCommitteeID() *uint64         { return nil }
func (m *EditOrderMessage) GetRecipient() *string           { return nil }
func (m *EditOrderMessage) GetPollHash() *string            { return nil }
func (m *EditOrderMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *EditOrderMessage) GetBuyerSendAddress() *string    { return nil }
func (m *EditOrderMessage) GetBuyerChainDeadline() *uint64  { return nil }

// DeleteOrderMessage represents deleting an order
type DeleteOrderMessage struct {
	Signer  string `json:"signer"`
	OrderID string `json:"order_id"`
}

func (m *DeleteOrderMessage) Type() MessageType               { return MsgTypeDeleteOrder }
func (m *DeleteOrderMessage) GetSigner() string               { return m.Signer }
func (m *DeleteOrderMessage) GetCounterparty() *string        { return nil }
func (m *DeleteOrderMessage) GetAmount() *uint64              { return nil }
func (m *DeleteOrderMessage) GetValidatorAddress() *string    { return nil }
func (m *DeleteOrderMessage) GetCommission() *float64         { return nil }
func (m *DeleteOrderMessage) GetChainID() *uint64             { return nil }
func (m *DeleteOrderMessage) GetSellAmount() *uint64          { return nil }
func (m *DeleteOrderMessage) GetBuyAmount() *uint64           { return nil }
func (m *DeleteOrderMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *DeleteOrderMessage) GetOrderID() *string             { return &m.OrderID }
func (m *DeleteOrderMessage) GetPrice() *float64              { return nil }
func (m *DeleteOrderMessage) GetParamKey() *string            { return nil }
func (m *DeleteOrderMessage) GetParamValue() *string          { return nil }
func (m *DeleteOrderMessage) GetCommitteeID() *uint64         { return nil }
func (m *DeleteOrderMessage) GetRecipient() *string           { return nil }
func (m *DeleteOrderMessage) GetPollHash() *string            { return nil }
func (m *DeleteOrderMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *DeleteOrderMessage) GetBuyerSendAddress() *string    { return nil }
func (m *DeleteOrderMessage) GetBuyerChainDeadline() *uint64  { return nil }

// DexLimitOrderMessage represents a DEX limit order
type DexLimitOrderMessage struct {
	From       string  `json:"from"`
	ChainID    uint64  `json:"chain_id"`
	SellAmount uint64  `json:"sell_amount"`
	BuyAmount  uint64  `json:"buy_amount"`
	Price      float64 `json:"price"`
}

func (m *DexLimitOrderMessage) Type() MessageType               { return MsgTypeDexLimitOrder }
func (m *DexLimitOrderMessage) GetSigner() string               { return m.From }
func (m *DexLimitOrderMessage) GetCounterparty() *string        { return nil }
func (m *DexLimitOrderMessage) GetAmount() *uint64              { return nil }
func (m *DexLimitOrderMessage) GetValidatorAddress() *string    { return nil }
func (m *DexLimitOrderMessage) GetCommission() *float64         { return nil }
func (m *DexLimitOrderMessage) GetChainID() *uint64             { return &m.ChainID }
func (m *DexLimitOrderMessage) GetSellAmount() *uint64          { return &m.SellAmount }
func (m *DexLimitOrderMessage) GetBuyAmount() *uint64           { return &m.BuyAmount }
func (m *DexLimitOrderMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *DexLimitOrderMessage) GetOrderID() *string             { return nil }
func (m *DexLimitOrderMessage) GetPrice() *float64              { return &m.Price }
func (m *DexLimitOrderMessage) GetParamKey() *string            { return nil }
func (m *DexLimitOrderMessage) GetParamValue() *string          { return nil }
func (m *DexLimitOrderMessage) GetCommitteeID() *uint64         { return nil }
func (m *DexLimitOrderMessage) GetRecipient() *string           { return nil }
func (m *DexLimitOrderMessage) GetPollHash() *string            { return nil }
func (m *DexLimitOrderMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *DexLimitOrderMessage) GetBuyerSendAddress() *string    { return nil }
func (m *DexLimitOrderMessage) GetBuyerChainDeadline() *uint64  { return nil }

// DexLiquidityDepositMessage represents adding liquidity to a DEX pool
type DexLiquidityDepositMessage struct {
	From    string `json:"from"`
	ChainID uint64 `json:"chain_id"`
	Amount  uint64 `json:"amount"`
}

func (m *DexLiquidityDepositMessage) Type() MessageType               { return MsgTypeDexLiquidityDeposit }
func (m *DexLiquidityDepositMessage) GetSigner() string               { return m.From }
func (m *DexLiquidityDepositMessage) GetCounterparty() *string        { return nil }
func (m *DexLiquidityDepositMessage) GetAmount() *uint64              { return nil }
func (m *DexLiquidityDepositMessage) GetValidatorAddress() *string    { return nil }
func (m *DexLiquidityDepositMessage) GetCommission() *float64         { return nil }
func (m *DexLiquidityDepositMessage) GetChainID() *uint64             { return &m.ChainID }
func (m *DexLiquidityDepositMessage) GetSellAmount() *uint64          { return nil }
func (m *DexLiquidityDepositMessage) GetBuyAmount() *uint64           { return nil }
func (m *DexLiquidityDepositMessage) GetLiquidityAmount() *uint64     { return &m.Amount }
func (m *DexLiquidityDepositMessage) GetOrderID() *string             { return nil }
func (m *DexLiquidityDepositMessage) GetPrice() *float64              { return nil }
func (m *DexLiquidityDepositMessage) GetParamKey() *string            { return nil }
func (m *DexLiquidityDepositMessage) GetParamValue() *string          { return nil }
func (m *DexLiquidityDepositMessage) GetCommitteeID() *uint64         { return nil }
func (m *DexLiquidityDepositMessage) GetRecipient() *string           { return nil }
func (m *DexLiquidityDepositMessage) GetPollHash() *string            { return nil }
func (m *DexLiquidityDepositMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *DexLiquidityDepositMessage) GetBuyerSendAddress() *string    { return nil }
func (m *DexLiquidityDepositMessage) GetBuyerChainDeadline() *uint64  { return nil }

// DexLiquidityWithdrawMessage represents removing liquidity from a DEX pool
type DexLiquidityWithdrawMessage struct {
	From    string `json:"from"`
	ChainID uint64 `json:"chain_id"`
	Amount  uint64 `json:"amount"`
}

func (m *DexLiquidityWithdrawMessage) Type() MessageType               { return MsgTypeDexLiquidityWithdraw }
func (m *DexLiquidityWithdrawMessage) GetSigner() string               { return m.From }
func (m *DexLiquidityWithdrawMessage) GetCounterparty() *string        { return nil }
func (m *DexLiquidityWithdrawMessage) GetAmount() *uint64              { return nil }
func (m *DexLiquidityWithdrawMessage) GetValidatorAddress() *string    { return nil }
func (m *DexLiquidityWithdrawMessage) GetCommission() *float64         { return nil }
func (m *DexLiquidityWithdrawMessage) GetChainID() *uint64             { return &m.ChainID }
func (m *DexLiquidityWithdrawMessage) GetSellAmount() *uint64          { return nil }
func (m *DexLiquidityWithdrawMessage) GetBuyAmount() *uint64           { return nil }
func (m *DexLiquidityWithdrawMessage) GetLiquidityAmount() *uint64     { return &m.Amount }
func (m *DexLiquidityWithdrawMessage) GetOrderID() *string             { return nil }
func (m *DexLiquidityWithdrawMessage) GetPrice() *float64              { return nil }
func (m *DexLiquidityWithdrawMessage) GetParamKey() *string            { return nil }
func (m *DexLiquidityWithdrawMessage) GetParamValue() *string          { return nil }
func (m *DexLiquidityWithdrawMessage) GetCommitteeID() *uint64         { return nil }
func (m *DexLiquidityWithdrawMessage) GetRecipient() *string           { return nil }
func (m *DexLiquidityWithdrawMessage) GetPollHash() *string            { return nil }
func (m *DexLiquidityWithdrawMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *DexLiquidityWithdrawMessage) GetBuyerSendAddress() *string    { return nil }
func (m *DexLiquidityWithdrawMessage) GetBuyerChainDeadline() *uint64  { return nil }

// StartPollMessage represents starting a governance poll via send tx memo
type StartPollMessage struct {
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
	Amount      uint64 `json:"amount"`
	PollHash    string `json:"poll_hash"`  // Extracted from memo startPoll field
	PollUrl     string `json:"poll_url"`   // Extracted from memo url field
	EndHeight   uint64 `json:"end_height"` // Extracted from memo endHeight field
}

func (m *StartPollMessage) Type() MessageType               { return MsgTypeStartPoll }
func (m *StartPollMessage) GetSigner() string               { return m.FromAddress }
func (m *StartPollMessage) GetCounterparty() *string        { return &m.ToAddress }
func (m *StartPollMessage) GetAmount() *uint64              { return &m.Amount }
func (m *StartPollMessage) GetValidatorAddress() *string    { return nil }
func (m *StartPollMessage) GetCommission() *float64         { return nil }
func (m *StartPollMessage) GetChainID() *uint64             { return nil }
func (m *StartPollMessage) GetSellAmount() *uint64          { return nil }
func (m *StartPollMessage) GetBuyAmount() *uint64           { return nil }
func (m *StartPollMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *StartPollMessage) GetOrderID() *string             { return nil }
func (m *StartPollMessage) GetPrice() *float64              { return nil }
func (m *StartPollMessage) GetParamKey() *string            { return nil }
func (m *StartPollMessage) GetParamValue() *string          { return nil }
func (m *StartPollMessage) GetCommitteeID() *uint64         { return nil }
func (m *StartPollMessage) GetRecipient() *string           { return nil }
func (m *StartPollMessage) GetPollHash() *string            { return &m.PollHash }
func (m *StartPollMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *StartPollMessage) GetBuyerSendAddress() *string    { return nil }
func (m *StartPollMessage) GetBuyerChainDeadline() *uint64  { return nil }

// VotePollMessage represents voting on a poll via send tx memo
type VotePollMessage struct {
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
	Amount      uint64 `json:"amount"`
	PollHash    string `json:"poll_hash"` // Extracted from memo votePoll field
	Approve     bool   `json:"approve"`   // Extracted from memo approve field
}

func (m *VotePollMessage) Type() MessageType               { return MsgTypeVotePoll }
func (m *VotePollMessage) GetSigner() string               { return m.FromAddress }
func (m *VotePollMessage) GetCounterparty() *string        { return &m.ToAddress }
func (m *VotePollMessage) GetAmount() *uint64              { return &m.Amount }
func (m *VotePollMessage) GetValidatorAddress() *string    { return nil }
func (m *VotePollMessage) GetCommission() *float64         { return nil }
func (m *VotePollMessage) GetChainID() *uint64             { return nil }
func (m *VotePollMessage) GetSellAmount() *uint64          { return nil }
func (m *VotePollMessage) GetBuyAmount() *uint64           { return nil }
func (m *VotePollMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *VotePollMessage) GetOrderID() *string             { return nil }
func (m *VotePollMessage) GetPrice() *float64              { return nil }
func (m *VotePollMessage) GetParamKey() *string            { return nil }
func (m *VotePollMessage) GetParamValue() *string          { return nil }
func (m *VotePollMessage) GetCommitteeID() *uint64         { return nil }
func (m *VotePollMessage) GetRecipient() *string           { return nil }
func (m *VotePollMessage) GetPollHash() *string            { return &m.PollHash }
func (m *VotePollMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *VotePollMessage) GetBuyerSendAddress() *string    { return nil }
func (m *VotePollMessage) GetBuyerChainDeadline() *uint64  { return nil }

// LockOrderMessage represents locking an order via send tx memo
type LockOrderMessage struct {
	FromAddress         string `json:"from_address"`
	ToAddress           string `json:"to_address"`
	Amount              uint64 `json:"amount"`
	OrderID             string `json:"order_id"`              // Extracted from memo
	ChainID             uint64 `json:"chain_id"`              // Extracted from memo
	BuyerReceiveAddress string `json:"buyer_receive_address"` // Extracted from memo
	BuyerSendAddress    string `json:"buyer_send_address"`    // Extracted from memo
	BuyerChainDeadline  uint64 `json:"buyer_chain_deadline"`  // Extracted from memo
}

func (m *LockOrderMessage) Type() MessageType               { return MsgTypeLockOrder }
func (m *LockOrderMessage) GetSigner() string               { return m.FromAddress }
func (m *LockOrderMessage) GetCounterparty() *string        { return &m.ToAddress }
func (m *LockOrderMessage) GetAmount() *uint64              { return &m.Amount }
func (m *LockOrderMessage) GetValidatorAddress() *string    { return nil }
func (m *LockOrderMessage) GetCommission() *float64         { return nil }
func (m *LockOrderMessage) GetChainID() *uint64             { return &m.ChainID }
func (m *LockOrderMessage) GetSellAmount() *uint64          { return nil }
func (m *LockOrderMessage) GetBuyAmount() *uint64           { return nil }
func (m *LockOrderMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *LockOrderMessage) GetOrderID() *string             { return &m.OrderID }
func (m *LockOrderMessage) GetPrice() *float64              { return nil }
func (m *LockOrderMessage) GetParamKey() *string            { return nil }
func (m *LockOrderMessage) GetParamValue() *string          { return nil }
func (m *LockOrderMessage) GetCommitteeID() *uint64         { return nil }
func (m *LockOrderMessage) GetRecipient() *string           { return nil }
func (m *LockOrderMessage) GetPollHash() *string            { return nil }
func (m *LockOrderMessage) GetBuyerReceiveAddress() *string { return &m.BuyerReceiveAddress }
func (m *LockOrderMessage) GetBuyerSendAddress() *string    { return &m.BuyerSendAddress }
func (m *LockOrderMessage) GetBuyerChainDeadline() *uint64  { return &m.BuyerChainDeadline }

// CloseOrderMessage represents closing an order via send tx memo
type CloseOrderMessage struct {
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
	Amount      uint64 `json:"amount"`
	OrderID     string `json:"order_id"` // Extracted from memo
	ChainID     uint64 `json:"chain_id"` // Extracted from memo
}

func (m *CloseOrderMessage) Type() MessageType               { return MsgTypeCloseOrder }
func (m *CloseOrderMessage) GetSigner() string               { return m.FromAddress }
func (m *CloseOrderMessage) GetCounterparty() *string        { return &m.ToAddress }
func (m *CloseOrderMessage) GetAmount() *uint64              { return &m.Amount }
func (m *CloseOrderMessage) GetValidatorAddress() *string    { return nil }
func (m *CloseOrderMessage) GetCommission() *float64         { return nil }
func (m *CloseOrderMessage) GetChainID() *uint64             { return &m.ChainID }
func (m *CloseOrderMessage) GetSellAmount() *uint64          { return nil }
func (m *CloseOrderMessage) GetBuyAmount() *uint64           { return nil }
func (m *CloseOrderMessage) GetLiquidityAmount() *uint64     { return nil }
func (m *CloseOrderMessage) GetOrderID() *string             { return &m.OrderID }
func (m *CloseOrderMessage) GetPrice() *float64              { return nil }
func (m *CloseOrderMessage) GetParamKey() *string            { return nil }
func (m *CloseOrderMessage) GetParamValue() *string          { return nil }
func (m *CloseOrderMessage) GetCommitteeID() *uint64         { return nil }
func (m *CloseOrderMessage) GetRecipient() *string           { return nil }
func (m *CloseOrderMessage) GetPollHash() *string            { return nil }
func (m *CloseOrderMessage) GetBuyerReceiveAddress() *string { return nil }
func (m *CloseOrderMessage) GetBuyerSendAddress() *string    { return nil }
func (m *CloseOrderMessage) GetBuyerChainDeadline() *uint64  { return nil }

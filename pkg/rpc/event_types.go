package rpc

// EventType identifies the type of event emitted during block processing
type EventType string

const (
	EventTypeReward                   EventType = "reward"
	EventTypeSlash                    EventType = "slash"
	EventTypeDexLiquidityDeposit      EventType = "dex-liquidity-deposit"
	EventTypeDexLiquidityWithdraw     EventType = "dex-liquidity-withdraw"
	EventTypeDexSwap                  EventType = "dex-swap"
	EventTypeOrderBookSwap            EventType = "order-book-swap"
	EventTypeAutomaticPause           EventType = "automatic-pause"
	EventTypeAutomaticBeginUnstaking  EventType = "automatic-begin-unstaking"
	EventTypeAutomaticFinishUnstaking EventType = "automatic-finish-unstaking"
)

func EventTypeAsStr(evt EventType) string {
	return string(evt)
}

// EventMessage is the interface all event types implement.
// This enables polymorphic handling of different event types while
// extracting common queryable fields (address, amounts, success flags).
type EventMessage interface {
	Type() EventType
	GetAddress() string
	GetAmount() *uint64
	GetSoldAmount() *uint64
	GetBoughtAmount() *uint64
	GetLocalAmount() *uint64
	GetRemoteAmount() *uint64
	GetSuccess() *bool
	GetLocalOrigin() *bool
	GetOrderID() *string
}

// RewardEvent represents a validator reward event
type RewardEvent struct {
	Data map[string]interface{}
}

func (e *RewardEvent) Type() EventType          { return EventTypeReward }
func (e *RewardEvent) GetAddress() string       { return GetStringField(e.Data, "address") }
func (e *RewardEvent) GetAmount() *uint64       { return GetOptionalUint64Field(e.Data, "amount") }
func (e *RewardEvent) GetSoldAmount() *uint64   { return nil }
func (e *RewardEvent) GetBoughtAmount() *uint64 { return nil }
func (e *RewardEvent) GetLocalAmount() *uint64  { return nil }
func (e *RewardEvent) GetRemoteAmount() *uint64 { return nil }
func (e *RewardEvent) GetSuccess() *bool        { return nil }
func (e *RewardEvent) GetLocalOrigin() *bool    { return nil }
func (e *RewardEvent) GetOrderID() *string      { return nil }

// SlashEvent represents a validator slash event
type SlashEvent struct {
	Data map[string]interface{}
}

func (e *SlashEvent) Type() EventType          { return EventTypeSlash }
func (e *SlashEvent) GetAddress() string       { return GetStringField(e.Data, "address") }
func (e *SlashEvent) GetAmount() *uint64       { return GetOptionalUint64Field(e.Data, "amount") }
func (e *SlashEvent) GetSoldAmount() *uint64   { return nil }
func (e *SlashEvent) GetBoughtAmount() *uint64 { return nil }
func (e *SlashEvent) GetLocalAmount() *uint64  { return nil }
func (e *SlashEvent) GetRemoteAmount() *uint64 { return nil }
func (e *SlashEvent) GetSuccess() *bool        { return nil }
func (e *SlashEvent) GetLocalOrigin() *bool    { return nil }
func (e *SlashEvent) GetOrderID() *string      { return nil }

// DexLiquidityDepositEvent represents adding liquidity to a DEX pool
type DexLiquidityDepositEvent struct {
	Data map[string]interface{}
}

// TODO: ADD POINTS
func (e *DexLiquidityDepositEvent) Type() EventType    { return EventTypeDexLiquidityDeposit }
func (e *DexLiquidityDepositEvent) GetAddress() string { return GetStringField(e.Data, "address") }
func (e *DexLiquidityDepositEvent) GetAmount() *uint64 {
	return GetOptionalUint64Field(e.Data, "amount")
}
func (e *DexLiquidityDepositEvent) GetSoldAmount() *uint64   { return nil }
func (e *DexLiquidityDepositEvent) GetBoughtAmount() *uint64 { return nil }
func (e *DexLiquidityDepositEvent) GetLocalAmount() *uint64  { return nil }
func (e *DexLiquidityDepositEvent) GetRemoteAmount() *uint64 { return nil }
func (e *DexLiquidityDepositEvent) GetSuccess() *bool        { return nil }
func (e *DexLiquidityDepositEvent) GetLocalOrigin() *bool {
	return GetOptionalBoolField(e.Data, "localOrigin")
}
func (e *DexLiquidityDepositEvent) GetOrderID() *string {
	return GetOptionalStringField(e.Data, "orderID")
}

// DexLiquidityWithdrawEvent represents removing liquidity from a DEX pool
type DexLiquidityWithdrawEvent struct {
	Data map[string]interface{}
}

// TODO: ADD POINTS
func (e *DexLiquidityWithdrawEvent) Type() EventType          { return EventTypeDexLiquidityWithdraw }
func (e *DexLiquidityWithdrawEvent) GetAddress() string       { return GetStringField(e.Data, "address") }
func (e *DexLiquidityWithdrawEvent) GetAmount() *uint64       { return nil }
func (e *DexLiquidityWithdrawEvent) GetSoldAmount() *uint64   { return nil }
func (e *DexLiquidityWithdrawEvent) GetBoughtAmount() *uint64 { return nil }
func (e *DexLiquidityWithdrawEvent) GetLocalAmount() *uint64 {
	return GetOptionalUint64Field(e.Data, "localAmount")
}
func (e *DexLiquidityWithdrawEvent) GetRemoteAmount() *uint64 {
	return GetOptionalUint64Field(e.Data, "remoteAmount")
}
func (e *DexLiquidityWithdrawEvent) GetSuccess() *bool     { return nil }
func (e *DexLiquidityWithdrawEvent) GetLocalOrigin() *bool { return nil }
func (e *DexLiquidityWithdrawEvent) GetOrderID() *string {
	return GetOptionalStringField(e.Data, "orderID")
}

// DexSwapEvent represents a DEX swap event
type DexSwapEvent struct {
	Data map[string]interface{}
}

func (e *DexSwapEvent) Type() EventType        { return EventTypeDexSwap }
func (e *DexSwapEvent) GetAddress() string     { return GetStringField(e.Data, "address") }
func (e *DexSwapEvent) GetAmount() *uint64     { return nil }
func (e *DexSwapEvent) GetSoldAmount() *uint64 { return GetOptionalUint64Field(e.Data, "soldAmount") }
func (e *DexSwapEvent) GetBoughtAmount() *uint64 {
	return GetOptionalUint64Field(e.Data, "boughtAmount")
}
func (e *DexSwapEvent) GetLocalAmount() *uint64  { return nil }
func (e *DexSwapEvent) GetRemoteAmount() *uint64 { return nil }
func (e *DexSwapEvent) GetSuccess() *bool        { return GetOptionalBoolField(e.Data, "success") }
func (e *DexSwapEvent) GetLocalOrigin() *bool    { return GetOptionalBoolField(e.Data, "localOrigin") }
func (e *DexSwapEvent) GetOrderID() *string {
	return GetOptionalStringField(e.Data, "orderID")
}

// OrderBookSwapEvent represents an order book swap event
type OrderBookSwapEvent struct {
	Data map[string]interface{}
}

// IMPORTANT: We need eric branch for this - oracle branch - eth-oracle -> PR #204
// TODO - ADD DATA (generic field to allow which eth account)
//   - [] try to get name of the token

// TODO: ADD:
//    // sellers_receive_address: the address of the seller where the 'counter asset' will be received
//    SellerReceiveAddress []byte protobuf:"bytes,4,opt,name=SellerReceiveAddress,proto3" json:"sellerReceiveAddress" // @gotags: json:"sellerReceiveAddress"
//    // buyer_send_address: the address of the buyer where the 'counter asset' will be sent from
//    BuyerSendAddress []byte protobuf:"bytes,5,opt,name=BuyerSendAddress,proto3" json:"buyerSendAddress" // @gotags: json:"buyerSendAddress"

func (e *OrderBookSwapEvent) Type() EventType    { return EventTypeOrderBookSwap }
func (e *OrderBookSwapEvent) GetAddress() string { return GetStringField(e.Data, "sellersSendAddress") }
func (e *OrderBookSwapEvent) GetAmount() *uint64 { return nil }
func (e *OrderBookSwapEvent) GetSoldAmount() *uint64 {
	return GetOptionalUint64Field(e.Data, "soldAmount")
}
func (e *OrderBookSwapEvent) GetBoughtAmount() *uint64 {
	return GetOptionalUint64Field(e.Data, "boughtAmount")
}
func (e *OrderBookSwapEvent) GetLocalAmount() *uint64  { return nil }
func (e *OrderBookSwapEvent) GetRemoteAmount() *uint64 { return nil }
func (e *OrderBookSwapEvent) GetSuccess() *bool        { return nil }
func (e *OrderBookSwapEvent) GetLocalOrigin() *bool    { return nil }
func (e *OrderBookSwapEvent) GetOrderID() *string      { return GetOptionalStringField(e.Data, "orderID") }

// AutomaticPauseEvent represents an automatic validator pause event
type AutomaticPauseEvent struct {
	Data map[string]interface{}
}

func (e *AutomaticPauseEvent) Type() EventType          { return EventTypeAutomaticPause }
func (e *AutomaticPauseEvent) GetAddress() string       { return GetStringField(e.Data, "address") }
func (e *AutomaticPauseEvent) GetAmount() *uint64       { return nil }
func (e *AutomaticPauseEvent) GetSoldAmount() *uint64   { return nil }
func (e *AutomaticPauseEvent) GetBoughtAmount() *uint64 { return nil }
func (e *AutomaticPauseEvent) GetLocalAmount() *uint64  { return nil }
func (e *AutomaticPauseEvent) GetRemoteAmount() *uint64 { return nil }
func (e *AutomaticPauseEvent) GetSuccess() *bool        { return nil }
func (e *AutomaticPauseEvent) GetLocalOrigin() *bool    { return nil }
func (e *AutomaticPauseEvent) GetOrderID() *string      { return nil }

// AutomaticBeginUnstakingEvent represents beginning automatic unstaking
type AutomaticBeginUnstakingEvent struct {
	Data map[string]interface{}
}

func (e *AutomaticBeginUnstakingEvent) Type() EventType          { return EventTypeAutomaticBeginUnstaking }
func (e *AutomaticBeginUnstakingEvent) GetAddress() string       { return GetStringField(e.Data, "address") }
func (e *AutomaticBeginUnstakingEvent) GetAmount() *uint64       { return nil }
func (e *AutomaticBeginUnstakingEvent) GetSoldAmount() *uint64   { return nil }
func (e *AutomaticBeginUnstakingEvent) GetBoughtAmount() *uint64 { return nil }
func (e *AutomaticBeginUnstakingEvent) GetLocalAmount() *uint64  { return nil }
func (e *AutomaticBeginUnstakingEvent) GetRemoteAmount() *uint64 { return nil }
func (e *AutomaticBeginUnstakingEvent) GetSuccess() *bool        { return nil }
func (e *AutomaticBeginUnstakingEvent) GetLocalOrigin() *bool    { return nil }
func (e *AutomaticBeginUnstakingEvent) GetOrderID() *string      { return nil }

// AutomaticFinishUnstakingEvent represents finishing automatic unstaking
type AutomaticFinishUnstakingEvent struct {
	Data map[string]interface{}
}

func (e *AutomaticFinishUnstakingEvent) Type() EventType          { return EventTypeAutomaticFinishUnstaking }
func (e *AutomaticFinishUnstakingEvent) GetAddress() string       { return GetStringField(e.Data, "address") }
func (e *AutomaticFinishUnstakingEvent) GetAmount() *uint64       { return nil }
func (e *AutomaticFinishUnstakingEvent) GetSoldAmount() *uint64   { return nil }
func (e *AutomaticFinishUnstakingEvent) GetBoughtAmount() *uint64 { return nil }
func (e *AutomaticFinishUnstakingEvent) GetLocalAmount() *uint64  { return nil }
func (e *AutomaticFinishUnstakingEvent) GetRemoteAmount() *uint64 { return nil }
func (e *AutomaticFinishUnstakingEvent) GetSuccess() *bool        { return nil }
func (e *AutomaticFinishUnstakingEvent) GetLocalOrigin() *bool    { return nil }
func (e *AutomaticFinishUnstakingEvent) GetOrderID() *string      { return nil }

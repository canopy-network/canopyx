package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// RpcEvent represents an event from the Canopy RPC.
// Events are emitted during block processing and can be associated with:
//   - begin_block: Events at the start of block processing
//   - tx_hash: Events during transaction execution
//   - end_block: Events at the end of block processing
type RpcEvent struct {
	EventType string                 `json:"eventType"`
	Msg       map[string]interface{} `json:"msg"`
	Height    uint64                 `json:"height"`
	Reference string                 `json:"reference"`
	ChainID   uint64                 `json:"chainId"`
	Address   string                 `json:"address"`
}

// detectEventType normalizes event type strings and maps them to EventType constants.
// This handles case variations and different naming conventions from the RPC layer.
func detectEventType(eventType string) EventType {
	// Normalize by converting to lowercase and removing common separators
	normalized := strings.ToLower(strings.TrimSpace(eventType))

	switch normalized {
	case "reward", "rewards":
		return EventTypeReward
	case "slash", "slashing":
		return EventTypeSlash
	case "dex-liquidity-deposit", "dexliquiditydeposit", "dex_liquidity_deposit":
		return EventTypeDexLiquidityDeposit
	case "dex-liquidity-withdraw", "dexliquiditywithdraw", "dex_liquidity_withdraw":
		return EventTypeDexLiquidityWithdraw
	case "dex-swap", "dexswap", "dex_swap":
		return EventTypeDexSwap
	case "order-book-swap", "orderbookswap", "order_book_swap":
		return EventTypeOrderBookSwap
	case "automatic-pause", "automaticpause", "automatic_pause":
		return EventTypeAutomaticPause
	case "automatic-begin-unstaking", "automaticbeginunstaking", "automatic_begin_unstaking":
		return EventTypeAutomaticBeginUnstaking
	case "automatic-finish-unstaking", "automaticfinishunstaking", "automatic_finish_unstaking":
		return EventTypeAutomaticFinishUnstaking
	default:
		// Return the normalized string as an EventType
		// This allows for graceful handling of unknown event types
		return EventType(normalized)
	}
}

// parseEventMessage converts an RPC event message into a typed EventMessage interface.
// This enables polymorphic handling of different event types while extracting common fields.
func parseEventMessage(eventType string, msgData map[string]interface{}) EventMessage {
	evtType := detectEventType(eventType)

	switch evtType {
	case EventTypeReward:
		return &RewardEvent{Data: msgData}
	case EventTypeSlash:
		return &SlashEvent{Data: msgData}
	case EventTypeDexLiquidityDeposit:
		return &DexLiquidityDepositEvent{Data: msgData}
	case EventTypeDexLiquidityWithdraw:
		return &DexLiquidityWithdrawEvent{Data: msgData}
	case EventTypeDexSwap:
		return &DexSwapEvent{Data: msgData}
	case EventTypeOrderBookSwap:
		return &OrderBookSwapEvent{Data: msgData}
	case EventTypeAutomaticPause:
		return &AutomaticPauseEvent{Data: msgData}
	case EventTypeAutomaticBeginUnstaking:
		return &AutomaticBeginUnstakingEvent{Data: msgData}
	case EventTypeAutomaticFinishUnstaking:
		return &AutomaticFinishUnstakingEvent{Data: msgData}
	default:
		// For unknown event types, create a generic event that tries to extract address
		// This provides forward compatibility for new event types
		return &UnknownEvent{
			EventType: evtType,
			Data:      msgData,
		}
	}
}

// UnknownEvent represents an event type that is not yet supported.
// This provides forward compatibility as new event types are added to the protocol.
type UnknownEvent struct {
	EventType EventType
	Data      map[string]interface{}
}

func (e *UnknownEvent) Type() EventType          { return e.EventType }
func (e *UnknownEvent) GetAddress() string       { return getStringField(e.Data, "address") }
func (e *UnknownEvent) GetAmount() *uint64       { return nil }
func (e *UnknownEvent) GetSoldAmount() *uint64   { return nil }
func (e *UnknownEvent) GetBoughtAmount() *uint64 { return nil }
func (e *UnknownEvent) GetLocalAmount() *uint64  { return nil }
func (e *UnknownEvent) GetRemoteAmount() *uint64 { return nil }
func (e *UnknownEvent) GetSuccess() *bool        { return nil }
func (e *UnknownEvent) GetLocalOrigin() *bool    { return nil }
func (e *UnknownEvent) GetOrderID() *string      { return nil }

// ToEvent maps an rpc.RpcEvent into the single-table Event model.
// This uses the event message parsing to extract type-specific fields into nullable columns.
func (e *RpcEvent) ToEvent() (*indexer.Event, error) {
	// Parse event message into typed interface
	msg := parseEventMessage(e.EventType, e.Msg)

	// Marshal full message to JSON (will be compressed by ClickHouse with ZSTD)
	msgJSON, err := json.Marshal(e.Msg)
	if err != nil {
		return nil, fmt.Errorf("marshal message to JSON: %w", err)
	}

	return &indexer.Event{
		Height:       e.Height,
		ChainID:      e.ChainID,
		Address:      e.Address,
		Reference:    e.Reference,
		EventType:    string(msg.Type()),
		Amount:       msg.GetAmount(),
		SoldAmount:   msg.GetSoldAmount(),
		BoughtAmount: msg.GetBoughtAmount(),
		LocalAmount:  msg.GetLocalAmount(),
		RemoteAmount: msg.GetRemoteAmount(),
		Success:      msg.GetSuccess(),
		LocalOrigin:  msg.GetLocalOrigin(),
		OrderID:      msg.GetOrderID(),
		Msg:          string(msgJSON),
		// HeightTime will be set later by the activity layer
	}, nil
}

// EventsByHeight returns all events for a given height.
// This method follows the same pattern as TxsByHeight, using pagination and graceful error handling.
func (c *HTTPClient) EventsByHeight(ctx context.Context, h uint64) ([]*indexer.Event, error) {
	events, err := ListPaged[*RpcEvent](ctx, c, eventsByHeightPath, map[string]any{"height": h})
	if err != nil {
		return nil, err
	}

	// Convert RPC events to DB model
	dbEvents := make([]*indexer.Event, 0, len(events))
	for _, e := range events {
		evt, err := e.ToEvent()
		if err != nil {
			// Log warning but continue processing other events
			// In production, you might want to track failed events
			continue
		}
		dbEvents = append(dbEvents, evt)
	}

	return dbEvents, nil
}

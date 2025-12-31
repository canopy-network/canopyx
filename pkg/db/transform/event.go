package transform

import (
	"fmt"

	"github.com/canopy-network/canopy/lib"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"google.golang.org/protobuf/encoding/protojson"
)

// Event maps a Canopy lib.Event (protobuf) into the single-table Event model.
// The Event uses protobuf oneof for polymorphic message types.
// Type-specific fields are extracted and stored in nullable columns for efficient querying.
func Event(e *lib.Event) (*indexer.Event, error) {
	// Marshal full event to JSON for storage (compressed by ClickHouse with ZSTD)
	msgJSON, err := protojson.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("marshal event to JSON: %w", err)
	}

	// Initialize the event struct
	evt := &indexer.Event{}

	// Set common fields from lib.Event
	evt.Height = e.Height
	evt.ChainID = e.ChainId
	evt.Address = bytesToHex(e.Address)
	evt.Reference = e.Reference
	evt.EventType = e.EventType
	evt.BlockHeight = e.BlockHeight
	evt.Msg = string(msgJSON)

	// Extract type-specific fields using oneof type switch and fill evt directly
	extractEventFields(e, evt)

	// HeightTime will be set later by the activity layer

	return evt, nil
}

// extractEventFields uses type switch on protobuf oneof to extract fields directly into evt.
func extractEventFields(e *lib.Event, evt *indexer.Event) {
	switch msg := e.Msg.(type) {
	case *lib.Event_Reward:
		evt.Amount = &msg.Reward.Amount

	case *lib.Event_Slash:
		evt.Amount = &msg.Slash.Amount

	case *lib.Event_DexLiquidityDeposit:
		evt.Amount = &msg.DexLiquidityDeposit.Amount
		evt.LocalOrigin = &msg.DexLiquidityDeposit.LocalOrigin
		evt.OrderID = ptrHex(msg.DexLiquidityDeposit.OrderId)
		evt.PointsReceived = &msg.DexLiquidityDeposit.Points

	case *lib.Event_DexLiquidityWithdrawal:
		evt.LocalAmount = &msg.DexLiquidityWithdrawal.LocalAmount
		evt.RemoteAmount = &msg.DexLiquidityWithdrawal.RemoteAmount
		evt.OrderID = ptrHex(msg.DexLiquidityWithdrawal.OrderId)
		evt.PointsBurned = &msg.DexLiquidityWithdrawal.PointsBurned

	case *lib.Event_DexSwap:
		evt.SoldAmount = &msg.DexSwap.SoldAmount
		evt.BoughtAmount = &msg.DexSwap.BoughtAmount
		evt.LocalOrigin = &msg.DexSwap.LocalOrigin
		evt.Success = &msg.DexSwap.Success
		evt.OrderID = ptrHex(msg.DexSwap.OrderId)

	case *lib.Event_OrderBookSwap:
		evt.SoldAmount = &msg.OrderBookSwap.SoldAmount
		evt.BoughtAmount = &msg.OrderBookSwap.BoughtAmount
		evt.OrderID = ptrHex(msg.OrderBookSwap.OrderId)
		evt.Data = ptrHex(msg.OrderBookSwap.Data)
		evt.SellerReceiveAddress = ptrHex(msg.OrderBookSwap.SellerReceiveAddress)
		evt.BuyerSendAddress = ptrHex(msg.OrderBookSwap.BuyerSendAddress)
		evt.SellersSendAddress = ptrHex(msg.OrderBookSwap.SellersSendAddress)
		// Note: OrderBookSwap doesn't have Success or LocalOrigin fields (unlike DexSwap)

	case *lib.Event_AutoPause:
		// AutoPause has no queryable fields beyond the common ones

	case *lib.Event_AutoBeginUnstaking:
		// AutoBeginUnstaking has no queryable fields beyond the common ones

	case *lib.Event_FinishUnstaking:
		// FinishUnstaking has no queryable fields beyond the common ones

	default:
		// Unknown event type - no fields extracted
	}
}

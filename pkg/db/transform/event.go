package transform

import (
	"encoding/json"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/rpc"
)

// Event maps an rpc.RpcEvent into the single-table Event model.
// This uses the event message parsing to extract type-specific fields into nullable columns.
func Event(e *rpc.RpcEvent) (*indexer.Event, error) {
	// Parse event message into typed interface
	msg := rpc.ParseEventMessage(e.EventType, e.Msg)

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

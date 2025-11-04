package transform

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/rpc"
)

// Transaction maps a rpc.Transaction into the single-table Transaction model.
// This uses the message parsing to extract type-specific fields.
func Transaction(tx *rpc.Transaction) (*indexer.Transaction, error) {
	// Convert the msg struct to map for parsing
	msgBytes, err := json.Marshal(tx.Transaction.Msg)
	if err != nil {
		return nil, fmt.Errorf("marshal message: %w", err)
	}

	var msgMap map[string]interface{}
	if err := json.Unmarshal(msgBytes, &msgMap); err != nil {
		return nil, fmt.Errorf("unmarshal message: %w", err)
	}

	// Parse message into typed interface
	msg, err := rpc.ParseMessage(tx.MessageType, msgMap)
	if err != nil {
		return nil, fmt.Errorf("parse message: %w", err)
	}

	// Marshal full message to JSON (will be compressed by ClickHouse with ZSTD)
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal message to JSON: %w", err)
	}

	// Extract optional signature fields
	var publicKey *string
	if tx.Transaction.Signature.PublicKey != "" {
		publicKey = &tx.Transaction.Signature.PublicKey
	}
	var signature *string
	if tx.Transaction.Signature.Signature != "" {
		signature = &tx.Transaction.Signature.Signature
	}

	return &indexer.Transaction{
		Height:           uint64(tx.Height),
		TxHash:           tx.TxHash,
		Time:             time.UnixMicro(tx.Transaction.Time),
		MessageType:      string(msg.Type()),
		Signer:           msg.GetSigner(),
		Counterparty:     msg.GetCounterparty(),
		Amount:           msg.GetAmount(),
		Fee:              uint64(tx.Transaction.Fee),
		ValidatorAddress: msg.GetValidatorAddress(),
		Commission:       msg.GetCommission(),
		ChainID:          msg.GetChainID(),
		SellAmount:       msg.GetSellAmount(),
		BuyAmount:        msg.GetBuyAmount(),
		LiquidityAmt:     msg.GetLiquidityAmount(),
		OrderID:          msg.GetOrderID(),
		Price:            msg.GetPrice(),
		ParamKey:         msg.GetParamKey(),
		ParamValue:       msg.GetParamValue(),
		CommitteeID:      msg.GetCommitteeID(),
		Recipient:        msg.GetRecipient(),
		Msg:              string(msgJSON),
		PublicKey:        publicKey,
		Signature:        signature,
	}, nil
}

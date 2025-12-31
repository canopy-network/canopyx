package transform

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/canopy-network/canopy/fsm"
	"github.com/canopy-network/canopy/lib"
	"github.com/canopy-network/canopy/lib/crypto"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"google.golang.org/protobuf/encoding/protojson"
)

// Transaction maps a Canopy lib.TxResult (protobuf) into the single-table Transaction model.
// The Transaction uses google.protobuf.Any for polymorphic message types (26 variants).
// Type-specific fields are extracted and stored in nullable columns for efficient querying.
func Transaction(txResult *lib.TxResult) (*indexer.Transaction, error) {
	tx := txResult.Transaction

	// Extract type-specific fields by unpacking the Any message
	fields, err := extractTransactionFields(tx, txResult.GetTxHash())
	if err != nil {
		return nil, fmt.Errorf("extract transaction fields: %w", err)
	}

	// Parse memo for poll/order operations (memo-based transactions)
	if tx.Memo != "" {
		parseMemoFields(tx.Memo, fields)
	}

	// Marshal full transaction to JSON for storage (compressed by ClickHouse with ZSTD)
	msgJSON, err := protojson.Marshal(tx)
	if err != nil {
		return nil, fmt.Errorf("marshal transaction to JSON: %w", err)
	}

	// Extract optional signature fields
	var publicKey *string
	if tx.Signature != nil && len(tx.Signature.PublicKey) > 0 {
		publicKey = ptrHex(tx.Signature.PublicKey)
	}
	var signature *string
	if tx.Signature != nil && len(tx.Signature.Signature) > 0 {
		signature = ptrHex(tx.Signature.Signature)
	}

	// Extract optional memo
	var memo *string
	if tx.Memo != "" {
		memo = &tx.Memo
	}

	// Determine signer: prefer txResult.Sender, fallback to deriving from public key
	signer := bytesToHex(txResult.Sender)
	if signer == "" && tx.Signature != nil && len(tx.Signature.PublicKey) > 0 {
		// Derive address from public key: SHA256 hash -> first 20 bytes
		pubHash := crypto.Hash(tx.Signature.PublicKey)
		if len(pubHash) >= 20 {
			signer = bytesToHex(pubHash[:20])
		}
	}

	return &indexer.Transaction{
		Height:              txResult.Height,
		TxHash:              txResult.TxHash,
		TxIndex:             uint32(txResult.Index),
		Time:                time.UnixMicro(int64(tx.Time)),
		CreatedHeight:       tx.CreatedHeight,
		NetworkID:           tx.NetworkId,
		MessageType:         tx.MessageType,
		Signer:              signer, // Canonical signer from signature verification, with message-specific fallback
		Amount:              fields.Amount,
		Fee:                 tx.Fee,
		Memo:                memo,
		ValidatorAddress:    fields.ValidatorAddress,
		Commission:          fields.Commission,
		ChainID:             fields.ChainID,
		SellAmount:          fields.SellAmount,
		BuyAmount:           fields.BuyAmount,
		LiquidityAmt:        fields.LiquidityAmount,
		LiquidityPercent:    fields.LiquidityPercent,
		OrderID:             fields.OrderID,
		Price:               fields.Price,
		ParamKey:            fields.ParamKey,
		ParamValue:          fields.ParamValue,
		CommitteeID:         fields.CommitteeID,
		Recipient:           fields.Recipient,
		PollHash:            fields.PollHash,
		BuyerReceiveAddress: fields.BuyerReceiveAddress,
		BuyerSendAddress:    fields.BuyerSendAddress,
		BuyerChainDeadline:  fields.BuyerChainDeadline,
		Msg:                 string(msgJSON),
		PublicKey:           publicKey,
		Signature:           signature,
		// HeightTime will be set later by the activity layer
	}, nil
}

// TransactionFields holds extracted type-specific fields from transaction messages.
type TransactionFields struct {
	Amount              *uint64
	ValidatorAddress    *string
	Commission          *float64
	ChainID             *uint64
	SellAmount          *uint64
	BuyAmount           *uint64
	LiquidityAmount     *uint64
	LiquidityPercent    *uint64
	OrderID             *string
	Price               *float64
	ParamKey            *string
	ParamValue          *string
	CommitteeID         *uint64
	Recipient           *string
	PollHash            *string
	BuyerReceiveAddress *string
	BuyerSendAddress    *string
	BuyerChainDeadline  *uint64
}

// extractTransactionFields unpacks google.protobuf.Any and extracts fields based on message type.
func extractTransactionFields(tx *lib.Transaction, txHash string) (*TransactionFields, error) {
	fields := &TransactionFields{}

	// Unpack the Any message based on message_type
	switch tx.MessageType {
	case "send":
		var msg fsm.MessageSend
		if err := tx.Msg.UnmarshalTo(&msg); err != nil {
			return nil, fmt.Errorf("unmarshal MessageSend: %w", err)
		}
		fields.Recipient = ptrHex(msg.ToAddress)
		fields.Amount = &msg.Amount

	case "stake":
		var msg fsm.MessageStake
		if err := tx.Msg.UnmarshalTo(&msg); err != nil {
			return nil, fmt.Errorf("unmarshal MessageStake: %w", err)
		}
		fields.Recipient = ptrHex(msg.OutputAddress) // Validator address (short form of public key)
		fields.Amount = &msg.Amount
		// Note: Validators stake to multiple committees, not a single ChainID

	case "editStake":
		var msg fsm.MessageEditStake
		if err := tx.Msg.UnmarshalTo(&msg); err != nil {
			return nil, fmt.Errorf("unmarshal MessageEditStake: %w", err)
		}
		fields.Recipient = ptrHex(msg.Address)
		fields.Amount = &msg.Amount

	case "unstake":
		var msg fsm.MessageUnstake
		if err := tx.Msg.UnmarshalTo(&msg); err != nil {
			return nil, fmt.Errorf("unmarshal MessageUnstake: %w", err)
		}
		fields.Recipient = ptrHex(msg.Address)

	case "pause":
		var msg fsm.MessagePause
		if err := tx.Msg.UnmarshalTo(&msg); err != nil {
			return nil, fmt.Errorf("unmarshal MessagePause: %w", err)
		}
		fields.Recipient = ptrHex(msg.Address)

	case "unpause":
		var msg fsm.MessageUnpause
		if err := tx.Msg.UnmarshalTo(&msg); err != nil {
			return nil, fmt.Errorf("unmarshal MessageUnpause: %w", err)
		}
		fields.Recipient = ptrHex(msg.Address)

	case "changeParameter":
		var msg fsm.MessageChangeParameter
		if err := tx.Msg.UnmarshalTo(&msg); err != nil {
			return nil, fmt.Errorf("unmarshal MessageChangeParameter: %w", err)
		}
		fields.ParamKey = &msg.ParameterKey
		// Extract ParameterValue from Any type as JSON
		if msg.ParameterValue != nil {
			paramValueJSON, err := protojson.Marshal(msg.ParameterValue)
			if err == nil {
				paramValueStr := string(paramValueJSON)
				fields.ParamValue = &paramValueStr
			}
		}

	case "daoTransfer":
		var msg fsm.MessageDAOTransfer
		if err := tx.Msg.UnmarshalTo(&msg); err != nil {
			return nil, fmt.Errorf("unmarshal MessageDAOTransfer: %w", err)
		}
		fields.Recipient = ptrHex(msg.Address)
		fields.Amount = &msg.Amount

	case "subsidy":
		var msg fsm.MessageSubsidy
		if err := tx.Msg.UnmarshalTo(&msg); err != nil {
			return nil, fmt.Errorf("unmarshal MessageSubsidy: %w", err)
		}
		fields.CommitteeID = &msg.ChainId

	case "createOrder":
		var msg fsm.MessageCreateOrder
		if err := tx.Msg.UnmarshalTo(&msg); err != nil {
			return nil, fmt.Errorf("unmarshal MessageCreateOrder: %w", err)
		}
		fields.ChainID = &msg.ChainId
		fields.SellAmount = &msg.AmountForSale
		if msg.RequestedAmount > 0 {
			price := float64(msg.RequestedAmount) / float64(msg.AmountForSale)
			fields.Price = &price
		}
		fields.Recipient = ptrHex(msg.SellerReceiveAddress)
		// Extract order_id from first 20 bytes of tx_hash
		txHashBytes, err := hex.DecodeString(txHash)
		if err == nil && len(txHashBytes) >= 20 {
			orderIDHex := hex.EncodeToString(txHashBytes[:20])
			fields.OrderID = &orderIDHex
		}

	case "editOrder":
		var msg fsm.MessageEditOrder
		if err := tx.Msg.UnmarshalTo(&msg); err != nil {
			return nil, fmt.Errorf("unmarshal MessageEditOrder: %w", err)
		}
		fields.ChainID = &msg.ChainId
		fields.OrderID = ptrHex(msg.OrderId)
		fields.SellAmount = &msg.AmountForSale
		if msg.RequestedAmount > 0 && msg.AmountForSale > 0 {
			price := float64(msg.RequestedAmount) / float64(msg.AmountForSale)
			fields.Price = &price
		}
		fields.Recipient = ptrHex(msg.SellerReceiveAddress) // Seller's receive address for the counter asset

	case "deleteOrder":
		var msg fsm.MessageDeleteOrder
		if err := tx.Msg.UnmarshalTo(&msg); err != nil {
			return nil, fmt.Errorf("unmarshal MessageDeleteOrder: %w", err)
		}
		fields.ChainID = &msg.ChainId
		fields.OrderID = ptrHex(msg.OrderId)

	case "dexLimitOrder":
		var msg fsm.MessageDexLimitOrder
		if err := tx.Msg.UnmarshalTo(&msg); err != nil {
			return nil, fmt.Errorf("unmarshal MessageDexLimitOrder: %w", err)
		}
		fields.ChainID = &msg.ChainId
		fields.SellAmount = &msg.AmountForSale
		fields.BuyAmount = &msg.RequestedAmount

	case "dexLiquidityDeposit":
		var msg fsm.MessageDexLiquidityDeposit
		if err := tx.Msg.UnmarshalTo(&msg); err != nil {
			return nil, fmt.Errorf("unmarshal MessageDexLiquidityDeposit: %w", err)
		}
		fields.ChainID = &msg.ChainId
		fields.LiquidityAmount = &msg.Amount

	case "dexLiquidityWithdraw":
		var msg fsm.MessageDexLiquidityWithdraw
		if err := tx.Msg.UnmarshalTo(&msg); err != nil {
			return nil, fmt.Errorf("unmarshal MessageDexLiquidityWithdraw: %w", err)
		}
		fields.ChainID = &msg.ChainId
		fields.LiquidityPercent = &msg.Percent // Percent of liquidity to withdraw

	default:
		// Unknown message type - no fields extracted
		// This allows forward compatibility with new message types
	}

	return fields, nil
}

// parseMemoFields extracts fields from memo JSON (poll and order operations).
func parseMemoFields(memo string, fields *TransactionFields) {
	// Try to parse as poll operations
	var pollMemo struct {
		StartPoll string `json:"startPoll"`
		VotePoll  string `json:"votePoll"`
		EndHeight uint64 `json:"endHeight"`
	}
	if err := json.Unmarshal([]byte(memo), &pollMemo); err == nil {
		if len(pollMemo.StartPoll) == 64 {
			fields.PollHash = &pollMemo.StartPoll
		} else if len(pollMemo.VotePoll) == 64 {
			fields.PollHash = &pollMemo.VotePoll
		}
		return
	}

	// Try to parse as order operations
	var orderMemo struct {
		LockOrder           bool   `json:"lockOrder"`
		CloseOrder          bool   `json:"closeOrder"`
		OrderID             string `json:"orderId"`
		BuyerReceiveAddress string `json:"buyerReceiveAddress"`
		BuyerSendAddress    string `json:"buyerSendAddress"`
		BuyerChainDeadline  uint64 `json:"buyerChainDeadline"`
	}
	if err := json.Unmarshal([]byte(memo), &orderMemo); err == nil {
		if orderMemo.LockOrder && orderMemo.OrderID != "" {
			fields.OrderID = &orderMemo.OrderID
			if orderMemo.BuyerReceiveAddress != "" {
				fields.BuyerReceiveAddress = &orderMemo.BuyerReceiveAddress
			}
			if orderMemo.BuyerSendAddress != "" {
				fields.BuyerSendAddress = &orderMemo.BuyerSendAddress
			}
			if orderMemo.BuyerChainDeadline > 0 {
				fields.BuyerChainDeadline = &orderMemo.BuyerChainDeadline
			}
		} else if orderMemo.CloseOrder && orderMemo.OrderID != "" {
			fields.OrderID = &orderMemo.OrderID
		}
	}
}

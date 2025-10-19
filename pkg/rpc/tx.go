package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// Transaction represents a transaction from the RPC.
type Transaction struct {
	Sender      string `json:"sender"`
	Recipient   string `json:"recipient"`
	MessageType string `json:"messageType"`
	Height      int    `json:"height"`
	Transaction struct {
		Type string `json:"type"`
		Msg  struct {
			FromAddress      string                 `json:"fromAddress"`
			ToAddress        string                 `json:"toAddress"`
			Amount           int                    `json:"amount"`
			ValidatorAddress string                 `json:"validatorAddress"`
			Delegator        string                 `json:"delegator"`
			Pool             string                 `json:"pool"`
			Staker           string                 `json:"staker"`
			ProposalID       int                    `json:"proposalId"`
			Voter            string                 `json:"voter"`
			Option           string                 `json:"option"`
			Raw              map[string]interface{} `json:"-"` // Capture all fields for unknown types
		} `json:"msg"`
		Signature struct {
			PublicKey string `json:"publicKey"`
			Signature string `json:"signature"`
		} `json:"signature"`
		Time          int64 `json:"time"`
		CreatedHeight int   `json:"createdHeight"`
		Fee           int   `json:"fee"`
		NetworkID     int   `json:"networkID"`
		ChainID       int   `json:"chainID"`
	} `json:"transaction"`
	TxHash string `json:"txHash"`
}

// detectMessageType infers message type from RPC transaction data.
// This examines the messageType field and message structure to determine the type.
func detectMessageType(msgType string, msg map[string]interface{}) MessageType {
	// Normalize the message type string
	switch msgType {
	case "send", "Send", "SEND":
		return MsgTypeSend
	case "delegate", "Delegate", "DELEGATE":
		return MsgTypeDelegate
	case "undelegate", "Undelegate", "UNDELEGATE":
		return MsgTypeUndelegate
	case "stake", "Stake", "STAKE":
		return MsgTypeStake
	case "unstake", "Unstake", "UNSTAKE":
		return MsgTypeUnstake
	case "edit_stake", "EditStake", "EDIT_STAKE":
		return MsgTypeEditStake
	case "vote", "Vote", "VOTE":
		return MsgTypeVote
	case "proposal", "Proposal", "PROPOSAL":
		return MsgTypeProposal
	case "contract", "Contract", "CONTRACT":
		return MsgTypeContract
	case "system", "System", "SYSTEM":
		return MsgTypeSystem
	}

	// Fallback: infer from field presence
	if _, ok := msg["toAddress"]; ok {
		return MsgTypeSend
	}
	if _, ok := msg["validatorAddress"]; ok {
		if _, ok := msg["delegator"]; ok {
			return MsgTypeDelegate
		}
		return MsgTypeUndelegate
	}
	if _, ok := msg["pool"]; ok {
		if _, ok := msg["staker"]; ok {
			return MsgTypeStake
		}
		return MsgTypeUnstake
	}
	if _, ok := msg["proposalId"]; ok {
		if _, ok := msg["voter"]; ok {
			return MsgTypeVote
		}
	}

	return MsgTypeUnknown
}

// parseMessage converts RPC transaction message into a typed Message interface.
// This handles all supported transaction types and falls back to UnknownMessage for unsupported types.
func parseMessage(msgType string, msgData map[string]interface{}) (Message, error) {
	messageType := detectMessageType(msgType, msgData)

	switch messageType {
	case MsgTypeSend:
		return &SendMessage{
			FromAddress: getStringField(msgData, "fromAddress"),
			ToAddress:   getStringField(msgData, "toAddress"),
			Amount:      uint64(getIntField(msgData, "amount")),
			Memo:        getStringField(msgData, "memo"),
		}, nil

	case MsgTypeDelegate:
		return &DelegateMessage{
			Delegator:        getStringField(msgData, "delegator"),
			ValidatorAddress: getStringField(msgData, "validatorAddress"),
			Amount:           uint64(getIntField(msgData, "amount")),
			Memo:             getStringField(msgData, "memo"),
		}, nil

	case MsgTypeUndelegate:
		return &UndelegateMessage{
			Delegator:        getStringField(msgData, "delegator"),
			ValidatorAddress: getStringField(msgData, "validatorAddress"),
			Amount:           uint64(getIntField(msgData, "amount")),
		}, nil

	case MsgTypeStake:
		lockPeriod := getOptionalUint32Field(msgData, "lockPeriod")
		return &StakeMessage{
			Staker:     getStringField(msgData, "staker"),
			Pool:       getStringField(msgData, "pool"),
			Amount:     uint64(getIntField(msgData, "amount")),
			LockPeriod: lockPeriod,
		}, nil

	case MsgTypeUnstake:
		return &UnstakeMessage{
			Staker: getStringField(msgData, "staker"),
			Pool:   getStringField(msgData, "pool"),
			Amount: uint64(getIntField(msgData, "amount")),
		}, nil

	case MsgTypeVote:
		return &VoteMessage{
			Voter:      getStringField(msgData, "voter"),
			ProposalID: uint64(getIntField(msgData, "proposalId")),
			Option:     getStringField(msgData, "option"),
			Memo:       getStringField(msgData, "memo"),
		}, nil

	case MsgTypeProposal:
		return &ProposalMessage{
			Proposer:    getStringField(msgData, "proposer"),
			Title:       getStringField(msgData, "title"),
			Description: getStringField(msgData, "description"),
			Deposit:     uint64(getIntField(msgData, "deposit")),
		}, nil

	case MsgTypeContract:
		value := getOptionalUint64Field(msgData, "value")
		return &ContractMessage{
			Caller:          getStringField(msgData, "caller"),
			ContractAddress: getStringField(msgData, "contractAddress"),
			Method:          getStringField(msgData, "method"),
			CallData:        getStringField(msgData, "callData"),
			Value:           value,
		}, nil

	case MsgTypeSystem:
		params := make(map[string]interface{})
		if p, ok := msgData["params"].(map[string]interface{}); ok {
			params = p
		}
		return &SystemMessage{
			Executor: getStringField(msgData, "executor"),
			Action:   getStringField(msgData, "action"),
			Params:   params,
		}, nil

	default:
		// UnknownMessage fallback - try to extract signer from common fields
		signer := getStringField(msgData, "sender")
		if signer == "" {
			signer = getStringField(msgData, "from")
		}
		if signer == "" {
			signer = getStringField(msgData, "fromAddress")
		}
		return &UnknownMessage{
			Signer: signer,
			Data:   msgData,
		}, nil
	}
}

// Helper functions to safely extract fields from map[string]interface{}

func getStringField(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getIntField(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case int64:
			return int(val)
		case float64:
			return int(val)
		}
	}
	return 0
}

func getOptionalUint32Field(m map[string]interface{}, key string) *uint32 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			u := uint32(val)
			return &u
		case int64:
			u := uint32(val)
			return &u
		case float64:
			u := uint32(val)
			return &u
		case uint32:
			return &val
		}
	}
	return nil
}

func getOptionalUint64Field(m map[string]interface{}, key string) *uint64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			u := uint64(val)
			return &u
		case int64:
			u := uint64(val)
			return &u
		case float64:
			u := uint64(val)
			return &u
		case uint64:
			return &val
		}
	}
	return nil
}

// ToTransaction maps a rpc.Transaction into the single-table Transaction model.
// This uses the new message parsing to extract type-specific fields.
func (tx *Transaction) ToTransaction() (*indexer.Transaction, error) {
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
	msg, err := parseMessage(tx.MessageType, msgMap)
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
		Height:        uint64(tx.Height),
		TxHash:        tx.TxHash,
		Time:          time.UnixMicro(tx.Transaction.Time),
		MessageType:   string(msg.Type()),
		Signer:        msg.GetSigner(),
		Counterparty:  msg.GetCounterparty(),
		Amount:        msg.GetAmount(),
		Fee:           uint64(tx.Transaction.Fee),
		Msg:           string(msgJSON),
		PublicKey:     publicKey,
		Signature:     signature,
		CreatedHeight: uint64(tx.Transaction.CreatedHeight),
	}, nil
}

// TxsByHeight returns all transactions for a given height.
// Updated signature returns single array (no more TransactionRaw).
func (c *HTTPClient) TxsByHeight(ctx context.Context, h uint64) ([]*indexer.Transaction, error) {
	txs, txsErr := ListPaged[*Transaction](ctx, c, txsByHeightPath, map[string]any{"height": h})
	if txsErr != nil {
		return nil, txsErr
	}

	// Convert RPC transactions to DB model
	dbTxs := make([]*indexer.Transaction, 0, len(txs))
	for _, t := range txs {
		tx, err := t.ToTransaction()
		if err != nil {
			// Log warning but continue processing other transactions
			// In production, you might want to track failed transactions
			continue
		}
		dbTxs = append(dbTxs, tx)
	}

	return dbTxs, nil
}
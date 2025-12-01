package rpc

import (
	"context"
	"encoding/json"
)

// Transaction represents a transaction from the RPC.
type Transaction struct {
	Sender      string `json:"sender"`
	Recipient   string `json:"recipient"`
	MessageType string `json:"messageType"`
	Height      int    `json:"height"`
	Index       int    `json:"index"` // Transaction index within block
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
		Time          int64  `json:"time"`
		CreatedHeight int    `json:"createdHeight"`
		Fee           int    `json:"fee"`
		NetworkID     int    `json:"networkID"`
		ChainID       int    `json:"chainID"`
		Memo          string `json:"memo"`
	} `json:"transaction"`
	TxHash string `json:"txHash"`
}

// DetectMemoType checks if a memo contains a recognized pattern (poll/order operations).
// Returns MsgTypeUnknown if memo is empty, invalid JSON, or doesn't match any pattern.
func DetectMemoType(memo string) MessageType {
	if len(memo) == 0 {
		return MsgTypeUnknown
	}

	// Must be valid JSON
	if !json.Valid([]byte(memo)) {
		return MsgTypeUnknown
	}

	// Check for startPoll (must have >= 80 chars and startPoll key with valid hash)
	if len(memo) >= 80 {
		var startPoll StartPollMemo
		if err := json.Unmarshal([]byte(memo), &startPoll); err == nil {
			if len(startPoll.StartPoll) == 64 && startPoll.EndHeight > 0 {
				return MsgTypeStartPoll
			}
		}
	}

	// Check for votePoll (poll hash must be 64 chars hex)
	var votePoll VotePollMemo
	if err := json.Unmarshal([]byte(memo), &votePoll); err == nil {
		if len(votePoll.VotePoll) == 64 {
			return MsgTypeVotePoll
		}
	}

	// Check for closeOrder (check before lockOrder - closeOrder has the flag)
	var closeOrder CloseOrderMemo
	if err := json.Unmarshal([]byte(memo), &closeOrder); err == nil {
		if closeOrder.CloseOrder && closeOrder.OrderID != "" {
			return MsgTypeCloseOrder
		}
	}

	// Check for lockOrder (requires orderID and both buyer addresses)
	var lockOrder LockOrderMemo
	if err := json.Unmarshal([]byte(memo), &lockOrder); err == nil {
		if lockOrder.OrderID != "" && lockOrder.BuyerReceiveAddress != "" && lockOrder.BuyerSendAddress != "" {
			return MsgTypeLockOrder
		}
	}

	return MsgTypeUnknown
}

// DetectMessageType infers a message type from RPC transaction data.
// This examines the messageType field and message structure to determine the type.
// For send transactions, it also checks the memo field for embedded operations.
func DetectMessageType(msgType string, msg map[string]interface{}, memo string) MessageType {
	// Normalize the message type string
	switch msgType {
	case "send", "Send", "SEND":
		// For send transactions, check if memo indicates a special operation
		if memoType := DetectMemoType(memo); memoType != MsgTypeUnknown {
			return memoType
		}
		return MsgTypeSend
	case "stake", "Stake", "STAKE":
		return MsgTypeStake
	case "unstake", "Unstake", "UNSTAKE":
		return MsgTypeUnstake
	case "edit_stake", "EditStake", "EDIT_STAKE":
		return MsgTypeEditStake
	case "pause", "Pause", "PAUSE":
		return MsgTypePause
	case "unpause", "Unpause", "UNPAUSE":
		return MsgTypeUnpause
	case "changeParameter", "ChangeParameter", "CHANGE_PARAMETER":
		return MsgTypeChangeParameter
	case "daoTransfer", "DAOTransfer", "DAO_TRANSFER":
		return MsgTypeDAOTransfer
	case "certificateResults", "CertificateResults", "CERTIFICATE_RESULTS":
		return MsgTypeCertificateResults
	case "subsidy", "Subsidy", "SUBSIDY":
		return MsgTypeSubsidy
	case "createOrder", "CreateOrder", "CREATE_ORDER":
		return MsgTypeCreateOrder
	case "editOrder", "EditOrder", "EDIT_ORDER":
		return MsgTypeEditOrder
	case "deleteOrder", "DeleteOrder", "DELETE_ORDER":
		return MsgTypeDeleteOrder
	case "dexLimitOrder", "DexLimitOrder", "DEX_LIMIT_ORDER":
		return MsgTypeDexLimitOrder
	case "dexLiquidityDeposit", "DexLiquidityDeposit", "DEX_LIQUIDITY_DEPOSIT":
		return MsgTypeDexLiquidityDeposit
	case "dexLiquidityWithdraw", "DexLiquidityWithdraw", "DEX_LIQUIDITY_WITHDRAW":
		return MsgTypeDexLiquidityWithdraw
	}

	// Fallback: infer from field presence

	// Check for parameter change
	if _, hasParamKey := msg["param_key"]; hasParamKey {
		return MsgTypeChangeParameter
	}

	// Check for certificate results
	if _, hasCertData := msg["certificate_data"]; hasCertData {
		return MsgTypeCertificateResults
	}

	// Check for committee ID (subsidy)
	if _, hasCommitteeID := msg["committee_id"]; hasCommitteeID {
		return MsgTypeSubsidy
	}

	// Check for order operations
	if _, hasOrderID := msg["order_id"]; hasOrderID {
		if _, hasPrice := msg["price"]; hasPrice {
			if _, hasSellAmount := msg["sell_amount"]; hasSellAmount {
				return MsgTypeCreateOrder
			}
			return MsgTypeEditOrder
		}
		return MsgTypeDeleteOrder
	}

	// Check for DEX operations (chain_id based)
	if _, hasChainID := msg["chain_id"]; hasChainID {
		if _, hasSellAmount := msg["sell_amount"]; hasSellAmount {
			return MsgTypeDexLimitOrder
		}
		if _, hasAmount := msg["amount"]; hasAmount {
			// Default to deposit if we can't determine direction
			return MsgTypeDexLiquidityDeposit
		}
	}

	// Existing fallback logic
	if _, ok := msg["toAddress"]; ok {
		return MsgTypeSend
	}
	if _, ok := msg["pool"]; ok {
		if _, ok := msg["staker"]; ok {
			return MsgTypeStake
		}
		return MsgTypeUnstake
	}

	return MsgTypeUnknown
}

// ParseMessage converts RPC transaction message into a typed Message interface.
// This handles all supported transaction types and falls back to UnknownMessage for unsupported types.
func ParseMessage(msgType string, msgData map[string]interface{}) (Message, error) {
	memo := GetStringField(msgData, "memo")
	messageType := DetectMessageType(msgType, msgData, memo)

	switch messageType {
	case MsgTypeSend:
		return &SendMessage{
			FromAddress: GetStringField(msgData, "fromAddress"),
			ToAddress:   GetStringField(msgData, "toAddress"),
			Amount:      uint64(GetIntField(msgData, "amount")),
			Memo:        memo,
		}, nil

	case MsgTypeStartPoll:
		var pollMemo StartPollMemo
		_ = json.Unmarshal([]byte(memo), &pollMemo)
		return &StartPollMessage{
			FromAddress: GetStringField(msgData, "fromAddress"),
			ToAddress:   GetStringField(msgData, "toAddress"),
			Amount:      uint64(GetIntField(msgData, "amount")),
			PollHash:    pollMemo.StartPoll,
			PollUrl:     pollMemo.Url,
			EndHeight:   pollMemo.EndHeight,
		}, nil

	case MsgTypeVotePoll:
		var pollMemo VotePollMemo
		_ = json.Unmarshal([]byte(memo), &pollMemo)
		return &VotePollMessage{
			FromAddress: GetStringField(msgData, "fromAddress"),
			ToAddress:   GetStringField(msgData, "toAddress"),
			Amount:      uint64(GetIntField(msgData, "amount")),
			PollHash:    pollMemo.VotePoll,
			Approve:     pollMemo.Approve,
		}, nil

	case MsgTypeLockOrder:
		var orderMemo LockOrderMemo
		_ = json.Unmarshal([]byte(memo), &orderMemo)
		return &LockOrderMessage{
			FromAddress:         GetStringField(msgData, "fromAddress"),
			ToAddress:           GetStringField(msgData, "toAddress"),
			Amount:              uint64(GetIntField(msgData, "amount")),
			OrderID:             orderMemo.OrderID,
			ChainID:             orderMemo.ChainID,
			BuyerReceiveAddress: orderMemo.BuyerReceiveAddress,
			BuyerSendAddress:    orderMemo.BuyerSendAddress,
			BuyerChainDeadline:  orderMemo.BuyerChainDeadline,
		}, nil

	case MsgTypeCloseOrder:
		var orderMemo CloseOrderMemo
		_ = json.Unmarshal([]byte(memo), &orderMemo)
		return &CloseOrderMessage{
			FromAddress: GetStringField(msgData, "fromAddress"),
			ToAddress:   GetStringField(msgData, "toAddress"),
			Amount:      uint64(GetIntField(msgData, "amount")),
			OrderID:     orderMemo.OrderID,
			ChainID:     orderMemo.ChainID,
		}, nil

	case MsgTypeStake:
		lockPeriod := GetOptionalUint32Field(msgData, "lockPeriod")
		return &StakeMessage{
			Staker:     GetStringField(msgData, "staker"),
			Pool:       GetStringField(msgData, "pool"),
			Amount:     uint64(GetIntField(msgData, "amount")),
			LockPeriod: lockPeriod,
		}, nil

	case MsgTypeUnstake:
		return &UnstakeMessage{
			Staker: GetStringField(msgData, "staker"),
			Pool:   GetStringField(msgData, "pool"),
			Amount: uint64(GetIntField(msgData, "amount")),
		}, nil

	case MsgTypePause:
		return &PauseMessage{
			Address: GetStringField(msgData, "address"),
		}, nil

	case MsgTypeUnpause:
		return &UnpauseMessage{
			Address: GetStringField(msgData, "address"),
		}, nil

	case MsgTypeChangeParameter:
		return &ChangeParameterMessage{
			ParamKey:   GetStringField(msgData, "param_key"),
			ParamValue: GetStringField(msgData, "param_value"),
			Signer:     GetStringField(msgData, "signer"),
		}, nil

	case MsgTypeDAOTransfer:
		return &DAOTransferMessage{
			FromAddress: GetStringField(msgData, "from_address"),
			ToAddress:   GetStringField(msgData, "to_address"),
			Amount:      uint64(GetIntField(msgData, "amount")),
		}, nil

	case MsgTypeCertificateResults:
		return &CertificateResultsMessage{
			Signer:          GetStringField(msgData, "signer"),
			CertificateData: GetStringField(msgData, "certificate_data"),
		}, nil

	case MsgTypeSubsidy:
		return &SubsidyMessage{
			FromAddress: GetStringField(msgData, "from_address"),
			ToAddress:   GetStringField(msgData, "to_address"),
			Amount:      uint64(GetIntField(msgData, "amount")),
			CommitteeID: uint64(GetIntField(msgData, "committee_id")),
		}, nil

	case MsgTypeCreateOrder:
		return &CreateOrderMessage{
			Signer:     GetStringField(msgData, "signer"),
			OrderID:    GetStringField(msgData, "order_id"),
			ChainID:    uint64(GetIntField(msgData, "chain_id")),
			SellAmount: uint64(GetIntField(msgData, "sell_amount")),
			BuyAmount:  uint64(GetIntField(msgData, "buy_amount")),
			Price:      GetFloat64Field(msgData, "price"),
		}, nil

	case MsgTypeEditOrder:
		return &EditOrderMessage{
			Signer:  GetStringField(msgData, "signer"),
			OrderID: GetStringField(msgData, "order_id"),
			Price:   GetFloat64Field(msgData, "price"),
		}, nil

	case MsgTypeDeleteOrder:
		return &DeleteOrderMessage{
			Signer:  GetStringField(msgData, "signer"),
			OrderID: GetStringField(msgData, "order_id"),
		}, nil

	case MsgTypeDexLimitOrder:
		return &DexLimitOrderMessage{
			From:       GetStringField(msgData, "from"),
			ChainID:    uint64(GetIntField(msgData, "chain_id")),
			SellAmount: uint64(GetIntField(msgData, "sell_amount")),
			BuyAmount:  uint64(GetIntField(msgData, "buy_amount")),
			Price:      GetFloat64Field(msgData, "price"),
		}, nil

	case MsgTypeDexLiquidityDeposit:
		return &DexLiquidityDepositMessage{
			From:    GetStringField(msgData, "from"),
			ChainID: uint64(GetIntField(msgData, "chain_id")),
			Amount:  uint64(GetIntField(msgData, "amount")),
		}, nil

	case MsgTypeDexLiquidityWithdraw:
		return &DexLiquidityWithdrawMessage{
			From:    GetStringField(msgData, "from"),
			ChainID: uint64(GetIntField(msgData, "chain_id")),
			Amount:  uint64(GetIntField(msgData, "amount")),
		}, nil

	default:
		// UnknownMessage fallback - try to extract signer from common fields
		signer := GetStringField(msgData, "sender")
		if signer == "" {
			signer = GetStringField(msgData, "from")
		}
		if signer == "" {
			signer = GetStringField(msgData, "fromAddress")
		}
		return &UnknownMessage{
			Signer: signer,
			Data:   msgData,
		}, nil
	}
}

// TxsByHeight returns all raw RPC transactions for a given height.
// Callers should convert to indexer models using transform.Transaction() if needed.
func (c *HTTPClient) TxsByHeight(ctx context.Context, height uint64) ([]*Transaction, error) {
	txs, txsErr := ListPaged[*Transaction](ctx, c, txsByHeightPath, NewQueryByHeightRequest(height))
	if txsErr != nil {
		return nil, txsErr
	}

	return txs, nil
}

package rpc_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/pkg/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseSendMessage tests parsing of send transactions
func TestParseSendMessage(t *testing.T) {
	tests := []struct {
		name           string
		msgType        string
		msgData        map[string]interface{}
		expectedSigner string
		expectedTo     string
		expectedAmount uint64
		expectedMemo   string
	}{
		{
			name:    "explicit send type",
			msgType: "send",
			msgData: map[string]interface{}{
				"fromAddress": "addr1",
				"toAddress":   "addr2",
				"amount":      1000,
				"memo":        "test memo",
			},
			expectedSigner: "addr1",
			expectedTo:     "addr2",
			expectedAmount: 1000,
			expectedMemo:   "test memo",
		},
		{
			name:    "capitalized Send type",
			msgType: "Send",
			msgData: map[string]interface{}{
				"fromAddress": "alice",
				"toAddress":   "bob",
				"amount":      500,
			},
			expectedSigner: "alice",
			expectedTo:     "bob",
			expectedAmount: 500,
			expectedMemo:   "",
		},
		{
			name:    "inferred from toAddress field",
			msgType: "unknown",
			msgData: map[string]interface{}{
				"fromAddress": "sender123",
				"toAddress":   "receiver456",
				"amount":      float64(2500), // Test float conversion
			},
			expectedSigner: "sender123",
			expectedTo:     "receiver456",
			expectedAmount: 2500,
			expectedMemo:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := rpc.ParseMessage(tt.msgType, tt.msgData)
			require.NoError(t, err)
			require.NotNil(t, msg)

			sendMsg, ok := msg.(*rpc.SendMessage)
			require.True(t, ok, "expected SendMessage type")

			assert.Equal(t, rpc.MsgTypeSend, sendMsg.Type())
			assert.Equal(t, tt.expectedSigner, sendMsg.GetSigner())
			assert.Equal(t, tt.expectedAmount, sendMsg.Amount)
			assert.Equal(t, tt.expectedMemo, sendMsg.Memo)

			counterparty := sendMsg.GetCounterparty()
			require.NotNil(t, counterparty)
			assert.Equal(t, tt.expectedTo, *counterparty)

			amount := sendMsg.GetAmount()
			require.NotNil(t, amount)
			assert.Equal(t, tt.expectedAmount, *amount)
		})
	}
}

// TestParseDelegateMessage tests parsing of delegate transactions
func TestParseDelegateMessage(t *testing.T) {
	tests := []struct {
		name              string
		msgType           string
		msgData           map[string]interface{}
		expectedDelegator string
		expectedValidator string
		expectedAmount    uint64
	}{
		{
			name:    "explicit delegate type",
			msgType: "delegate",
			msgData: map[string]interface{}{
				"delegator":        "delegator1",
				"validatorAddress": "validator1",
				"amount":           10000,
				"memo":             "delegation",
			},
			expectedDelegator: "delegator1",
			expectedValidator: "validator1",
			expectedAmount:    10000,
		},
		{
			name:    "DELEGATE uppercase",
			msgType: "DELEGATE",
			msgData: map[string]interface{}{
				"delegator":        "user123",
				"validatorAddress": "val456",
				"amount":           int64(5000),
			},
			expectedDelegator: "user123",
			expectedValidator: "val456",
			expectedAmount:    5000,
		},
		{
			name:    "inferred from fields",
			msgType: "unknown",
			msgData: map[string]interface{}{
				"delegator":        "inferredDelegator",
				"validatorAddress": "inferredValidator",
				"amount":           7500,
			},
			expectedDelegator: "inferredDelegator",
			expectedValidator: "inferredValidator",
			expectedAmount:    7500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := rpc.ParseMessage(tt.msgType, tt.msgData)
			require.NoError(t, err)

			delegateMsg, ok := msg.(*rpc.DelegateMessage)
			require.True(t, ok, "expected DelegateMessage type")

			assert.Equal(t, rpc.MsgTypeDelegate, delegateMsg.Type())
			assert.Equal(t, tt.expectedDelegator, delegateMsg.GetSigner())
			assert.Equal(t, tt.expectedAmount, delegateMsg.Amount)

			counterparty := delegateMsg.GetCounterparty()
			require.NotNil(t, counterparty)
			assert.Equal(t, tt.expectedValidator, *counterparty)
		})
	}
}

// TestParseUndelegateMessage tests parsing of undelegate transactions
func TestParseUndelegateMessage(t *testing.T) {
	msgData := map[string]interface{}{
		"delegator":        "undelegator1",
		"validatorAddress": "validator2",
		"amount":           3000,
	}

	msg, err := rpc.ParseMessage("undelegate", msgData)
	require.NoError(t, err)

	undelegateMsg, ok := msg.(*rpc.UndelegateMessage)
	require.True(t, ok)

	assert.Equal(t, rpc.MsgTypeUndelegate, undelegateMsg.Type())
	assert.Equal(t, "undelegator1", undelegateMsg.GetSigner())
	assert.Equal(t, uint64(3000), undelegateMsg.Amount)

	counterparty := undelegateMsg.GetCounterparty()
	require.NotNil(t, counterparty)
	assert.Equal(t, "validator2", *counterparty)

	amount := undelegateMsg.GetAmount()
	require.NotNil(t, amount)
	assert.Equal(t, uint64(3000), *amount)
}

// TestParseStakeMessage tests parsing of stake transactions
func TestParseStakeMessage(t *testing.T) {
	tests := []struct {
		name               string
		msgData            map[string]interface{}
		expectedLockPeriod *uint32
	}{
		{
			name: "stake with lock period",
			msgData: map[string]interface{}{
				"staker":     "staker1",
				"pool":       "pool1",
				"amount":     5000,
				"lockPeriod": 30,
			},
			expectedLockPeriod: func() *uint32 { v := uint32(30); return &v }(),
		},
		{
			name: "stake without lock period",
			msgData: map[string]interface{}{
				"staker": "staker2",
				"pool":   "pool2",
				"amount": 8000,
			},
			expectedLockPeriod: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := rpc.ParseMessage("stake", tt.msgData)
			require.NoError(t, err)

			stakeMsg, ok := msg.(*rpc.StakeMessage)
			require.True(t, ok)

			assert.Equal(t, rpc.MsgTypeStake, stakeMsg.Type())

			if tt.expectedLockPeriod != nil {
				require.NotNil(t, stakeMsg.LockPeriod)
				assert.Equal(t, *tt.expectedLockPeriod, *stakeMsg.LockPeriod)
			} else {
				assert.Nil(t, stakeMsg.LockPeriod)
			}
		})
	}
}

// TestParseVoteMessage tests parsing of vote transactions
func TestParseVoteMessage(t *testing.T) {
	msgData := map[string]interface{}{
		"voter":      "voter1",
		"proposalId": 42,
		"option":     "yes",
		"memo":       "I support this",
	}

	msg, err := rpc.ParseMessage("vote", msgData)
	require.NoError(t, err)

	voteMsg, ok := msg.(*rpc.VoteMessage)
	require.True(t, ok)

	assert.Equal(t, rpc.MsgTypeVote, voteMsg.Type())
	assert.Equal(t, "voter1", voteMsg.GetSigner())
	assert.Equal(t, uint64(42), voteMsg.ProposalID)
	assert.Equal(t, "yes", voteMsg.Option)
	assert.Equal(t, "I support this", voteMsg.Memo)

	// Votes have no counterparty or amount
	assert.Nil(t, voteMsg.GetCounterparty())
	assert.Nil(t, voteMsg.GetAmount())
}

// TestParseProposalMessage tests parsing of proposal transactions
func TestParseProposalMessage(t *testing.T) {
	msgData := map[string]interface{}{
		"proposer":    "proposer1",
		"title":       "Increase block size",
		"description": "This proposal increases the block size to 2MB",
		"deposit":     1000000,
	}

	msg, err := rpc.ParseMessage("proposal", msgData)
	require.NoError(t, err)

	proposalMsg, ok := msg.(*rpc.ProposalMessage)
	require.True(t, ok)

	assert.Equal(t, rpc.MsgTypeProposal, proposalMsg.Type())
	assert.Equal(t, "proposer1", proposalMsg.GetSigner())
	assert.Equal(t, "Increase block size", proposalMsg.Title)
	assert.Equal(t, "This proposal increases the block size to 2MB", proposalMsg.Description)
	assert.Equal(t, uint64(1000000), proposalMsg.Deposit)

	// Proposals have deposit as amount but no counterparty
	assert.Nil(t, proposalMsg.GetCounterparty())
	amount := proposalMsg.GetAmount()
	require.NotNil(t, amount)
	assert.Equal(t, uint64(1000000), *amount)
}

// TestParseContractMessage tests parsing of contract call transactions
func TestParseContractMessage(t *testing.T) {
	tests := []struct {
		name          string
		msgData       map[string]interface{}
		expectedValue *uint64
	}{
		{
			name: "contract call with value",
			msgData: map[string]interface{}{
				"caller":          "caller1",
				"contractAddress": "0xcontract123",
				"method":          "transfer",
				"callData":        "{\"to\":\"addr1\",\"amount\":100}",
				"value":           500,
			},
			expectedValue: func() *uint64 { v := uint64(500); return &v }(),
		},
		{
			name: "contract call without value",
			msgData: map[string]interface{}{
				"caller":          "caller2",
				"contractAddress": "0xcontract456",
				"method":          "balanceOf",
				"callData":        "{\"address\":\"addr2\"}",
			},
			expectedValue: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := rpc.ParseMessage("contract", tt.msgData)
			require.NoError(t, err)

			contractMsg, ok := msg.(*rpc.ContractMessage)
			require.True(t, ok)

			assert.Equal(t, rpc.MsgTypeContract, contractMsg.Type())

			if tt.expectedValue != nil {
				require.NotNil(t, contractMsg.Value)
				assert.Equal(t, *tt.expectedValue, *contractMsg.Value)

				amount := contractMsg.GetAmount()
				require.NotNil(t, amount)
				assert.Equal(t, *tt.expectedValue, *amount)
			} else {
				assert.Nil(t, contractMsg.Value)
				assert.Nil(t, contractMsg.GetAmount())
			}
		})
	}
}

// TestParseSystemMessage tests parsing of system transactions
func TestParseSystemMessage(t *testing.T) {
	msgData := map[string]interface{}{
		"executor": "admin1",
		"action":   "upgrade_protocol",
		"params": map[string]interface{}{
			"version": "2.0",
			"height":  100000,
		},
	}

	msg, err := rpc.ParseMessage("system", msgData)
	require.NoError(t, err)

	systemMsg, ok := msg.(*rpc.SystemMessage)
	require.True(t, ok)

	assert.Equal(t, rpc.MsgTypeSystem, systemMsg.Type())
	assert.Equal(t, "admin1", systemMsg.GetSigner())
	assert.Equal(t, "upgrade_protocol", systemMsg.Action)
	assert.NotNil(t, systemMsg.Params)
	assert.Equal(t, "2.0", systemMsg.Params["version"])

	// System messages have no counterparty or amount
	assert.Nil(t, systemMsg.GetCounterparty())
	assert.Nil(t, systemMsg.GetAmount())
}

// TestParseUnknownMessage tests handling of unknown transaction types
func TestParseUnknownMessage(t *testing.T) {
	tests := []struct {
		name           string
		msgData        map[string]interface{}
		expectedSigner string
	}{
		{
			name: "unknown with sender field",
			msgData: map[string]interface{}{
				"sender":      "unknown_sender",
				"unknownData": "some value",
			},
			expectedSigner: "unknown_sender",
		},
		{
			name: "unknown with from field",
			msgData: map[string]interface{}{
				"from":        "from_address",
				"unknownData": "some value",
			},
			expectedSigner: "from_address",
		},
		{
			name: "unknown with fromAddress field",
			msgData: map[string]interface{}{
				"fromAddress": "from_addr",
				"unknownData": "some value",
			},
			expectedSigner: "from_addr",
		},
		{
			name: "unknown with no signer field",
			msgData: map[string]interface{}{
				"randomField": "random value",
			},
			expectedSigner: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := rpc.ParseMessage("completely_unknown_type", tt.msgData)
			require.NoError(t, err)

			unknownMsg, ok := msg.(*rpc.UnknownMessage)
			require.True(t, ok)

			assert.Equal(t, rpc.MsgTypeUnknown, unknownMsg.Type())
			assert.Equal(t, tt.expectedSigner, unknownMsg.GetSigner())
			assert.Nil(t, unknownMsg.GetCounterparty())
			assert.Nil(t, unknownMsg.GetAmount())
			assert.NotNil(t, unknownMsg.Data)
		})
	}
}

// TestDetectMessageType tests message type detection logic
func TestDetectMessageType(t *testing.T) {
	tests := []struct {
		name         string
		msgType      string
		msgData      map[string]interface{}
		expectedType rpc.MessageType
	}{
		{
			name:         "explicit send",
			msgType:      "send",
			msgData:      map[string]interface{}{},
			expectedType: rpc.MsgTypeSend,
		},
		{
			name:    "infer send from toAddress",
			msgType: "unknown",
			msgData: map[string]interface{}{
				"toAddress": "addr1",
			},
			expectedType: rpc.MsgTypeSend,
		},
		{
			name:    "infer delegate from validatorAddress and delegator",
			msgType: "unknown",
			msgData: map[string]interface{}{
				"validatorAddress": "val1",
				"delegator":        "del1",
			},
			expectedType: rpc.MsgTypeDelegate,
		},
		{
			name:    "infer undelegate from validatorAddress without delegator",
			msgType: "unknown",
			msgData: map[string]interface{}{
				"validatorAddress": "val1",
			},
			expectedType: rpc.MsgTypeUndelegate,
		},
		{
			name:    "infer stake from pool and staker",
			msgType: "unknown",
			msgData: map[string]interface{}{
				"pool":   "pool1",
				"staker": "staker1",
			},
			expectedType: rpc.MsgTypeStake,
		},
		{
			name:    "infer unstake from pool without staker",
			msgType: "unknown",
			msgData: map[string]interface{}{
				"pool": "pool1",
			},
			expectedType: rpc.MsgTypeUnstake,
		},
		{
			name:    "infer vote from proposalId and voter",
			msgType: "unknown",
			msgData: map[string]interface{}{
				"proposalId": 1,
				"voter":      "voter1",
			},
			expectedType: rpc.MsgTypeVote,
		},
		{
			name:         "unknown type with no inference clues",
			msgType:      "unknown",
			msgData:      map[string]interface{}{},
			expectedType: rpc.MsgTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected := rpc.DetectMessageType(tt.msgType, tt.msgData)
			assert.Equal(t, tt.expectedType, detected)
		})
	}
}

// TestMessageInterfaceMethods tests that all message types implement interface methods correctly
func TestMessageInterfaceMethods(t *testing.T) {
	tests := []struct {
		name               string
		msg                rpc.Message
		expectedType       rpc.MessageType
		expectedSigner     string
		expectCounterparty bool
		expectedCounterpty string
		expectAmount       bool
		expectedAmount     uint64
	}{
		{
			name: "SendMessage",
			msg: &rpc.SendMessage{
				FromAddress: "sender",
				ToAddress:   "receiver",
				Amount:      1000,
			},
			expectedType:       rpc.MsgTypeSend,
			expectedSigner:     "sender",
			expectCounterparty: true,
			expectedCounterpty: "receiver",
			expectAmount:       true,
			expectedAmount:     1000,
		},
		{
			name: "DelegateMessage",
			msg: &rpc.DelegateMessage{
				Delegator:        "delegator",
				ValidatorAddress: "validator",
				Amount:           2000,
			},
			expectedType:       rpc.MsgTypeDelegate,
			expectedSigner:     "delegator",
			expectCounterparty: true,
			expectedCounterpty: "validator",
			expectAmount:       true,
			expectedAmount:     2000,
		},
		{
			name: "VoteMessage",
			msg: &rpc.VoteMessage{
				Voter:      "voter",
				ProposalID: 5,
				Option:     "yes",
			},
			expectedType:       rpc.MsgTypeVote,
			expectedSigner:     "voter",
			expectCounterparty: false,
			expectAmount:       false,
		},
		{
			name: "ProposalMessage",
			msg: &rpc.ProposalMessage{
				Proposer:    "proposer",
				Title:       "Test",
				Description: "Test proposal",
				Deposit:     5000,
			},
			expectedType:       rpc.MsgTypeProposal,
			expectedSigner:     "proposer",
			expectCounterparty: false,
			expectAmount:       true,
			expectedAmount:     5000,
		},
		{
			name: "ContractMessage with value",
			msg: &rpc.ContractMessage{
				Caller:          "caller",
				ContractAddress: "contract",
				Method:          "call",
				Value:           func() *uint64 { v := uint64(300); return &v }(),
			},
			expectedType:       rpc.MsgTypeContract,
			expectedSigner:     "caller",
			expectCounterparty: true,
			expectedCounterpty: "contract",
			expectAmount:       true,
			expectedAmount:     300,
		},
		{
			name: "ContractMessage without value",
			msg: &rpc.ContractMessage{
				Caller:          "caller",
				ContractAddress: "contract",
				Method:          "query",
			},
			expectedType:       rpc.MsgTypeContract,
			expectedSigner:     "caller",
			expectCounterparty: true,
			expectedCounterpty: "contract",
			expectAmount:       false,
		},
		{
			name: "SystemMessage",
			msg: &rpc.SystemMessage{
				Executor: "admin",
				Action:   "upgrade",
			},
			expectedType:       rpc.MsgTypeSystem,
			expectedSigner:     "admin",
			expectCounterparty: false,
			expectAmount:       false,
		},
		{
			name: "UnknownMessage",
			msg: &rpc.UnknownMessage{
				Signer: "unknown_user",
			},
			expectedType:       rpc.MsgTypeUnknown,
			expectedSigner:     "unknown_user",
			expectCounterparty: false,
			expectAmount:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedType, tt.msg.Type())
			assert.Equal(t, tt.expectedSigner, tt.msg.GetSigner())

			counterparty := tt.msg.GetCounterparty()
			if tt.expectCounterparty {
				require.NotNil(t, counterparty)
				assert.Equal(t, tt.expectedCounterpty, *counterparty)
			} else {
				assert.Nil(t, counterparty)
			}

			amount := tt.msg.GetAmount()
			if tt.expectAmount {
				require.NotNil(t, amount)
				assert.Equal(t, tt.expectedAmount, *amount)
			} else {
				assert.Nil(t, amount)
			}
		})
	}
}

// TestMessageJSONMarshaling tests that messages can be marshaled to JSON and back
func TestMessageJSONMarshaling(t *testing.T) {
	tests := []struct {
		name string
		msg  rpc.Message
	}{
		{
			name: "SendMessage",
			msg: &rpc.SendMessage{
				FromAddress: "alice",
				ToAddress:   "bob",
				Amount:      1000,
				Memo:        "payment",
			},
		},
		{
			name: "VoteMessage",
			msg: &rpc.VoteMessage{
				Voter:      "voter1",
				ProposalID: 42,
				Option:     "yes",
			},
		},
		{
			name: "ContractMessage",
			msg: &rpc.ContractMessage{
				Caller:          "caller1",
				ContractAddress: "0xcontract",
				Method:          "transfer",
				CallData:        "{}",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			jsonBytes, err := json.Marshal(tt.msg)
			require.NoError(t, err)
			assert.NotEmpty(t, jsonBytes)

			// Verify JSON is valid
			var result map[string]interface{}
			err = json.Unmarshal(jsonBytes, &result)
			require.NoError(t, err)
			assert.NotEmpty(t, result)
		})
	}
}

// TestToTransaction tests the Transaction.ToTransaction() method
func TestToTransaction(t *testing.T) {
	now := time.Now()
	nowMicro := now.UnixMicro()

	rpcTx := &rpc.Transaction{
		Sender:      "sender1",
		Recipient:   "recipient1",
		MessageType: "send",
		Height:      100,
		TxHash:      "hash123",
	}

	rpcTx.Transaction.Type = "send"
	rpcTx.Transaction.Msg.FromAddress = "sender1"
	rpcTx.Transaction.Msg.ToAddress = "recipient1"
	rpcTx.Transaction.Msg.Amount = 5000
	rpcTx.Transaction.Signature.PublicKey = "pubkey123"
	rpcTx.Transaction.Signature.Signature = "sig123"
	rpcTx.Transaction.Time = nowMicro
	rpcTx.Transaction.CreatedHeight = 99
	rpcTx.Transaction.Fee = 10
	rpcTx.Transaction.NetworkID = 1
	rpcTx.Transaction.ChainID = 1

	tx, err := rpcTx.ToTransaction()
	require.NoError(t, err)
	require.NotNil(t, tx)

	assert.Equal(t, uint64(100), tx.Height)
	assert.Equal(t, "hash123", tx.TxHash)
	assert.Equal(t, "send", tx.MessageType)
	assert.Equal(t, "sender1", tx.Signer)
	assert.Equal(t, uint64(10), tx.Fee)

	// Check counterparty
	require.NotNil(t, tx.Counterparty)
	assert.Equal(t, "recipient1", *tx.Counterparty)

	// Check amount
	require.NotNil(t, tx.Amount)
	assert.Equal(t, uint64(5000), *tx.Amount)

	// Check signature fields
	require.NotNil(t, tx.PublicKey)
	assert.Equal(t, "pubkey123", *tx.PublicKey)
	require.NotNil(t, tx.Signature)
	assert.Equal(t, "sig123", *tx.Signature)

	// Check msg field contains JSON
	assert.NotEmpty(t, tx.Msg)
	var msgData map[string]interface{}
	err = json.Unmarshal([]byte(tx.Msg), &msgData)
	require.NoError(t, err)
	assert.Equal(t, "sender1", msgData["from_address"])
	assert.Equal(t, "recipient1", msgData["to_address"])
}

// TestToTransactionWithNilSignature tests ToTransaction with missing signature fields
func TestToTransactionWithNilSignature(t *testing.T) {
	rpcTx := &rpc.Transaction{
		Sender:      "sender1",
		MessageType: "vote",
		Height:      200,
		TxHash:      "votehash",
	}

	rpcTx.Transaction.Type = "vote"
	rpcTx.Transaction.Msg.Voter = "voter1"
	rpcTx.Transaction.Msg.ProposalID = 5
	rpcTx.Transaction.Msg.Option = "yes"
	rpcTx.Transaction.Time = time.Now().UnixMicro()
	rpcTx.Transaction.CreatedHeight = 199
	rpcTx.Transaction.Fee = 5
	// No signature fields set

	tx, err := rpcTx.ToTransaction()
	require.NoError(t, err)
	require.NotNil(t, tx)

	assert.Equal(t, "vote", tx.MessageType)
	assert.Equal(t, "voter1", tx.Signer)

	// Vote has no counterparty or amount
	assert.Nil(t, tx.Counterparty)
	assert.Nil(t, tx.Amount)

	// Signature fields should be nil
	assert.Nil(t, tx.PublicKey)
	assert.Nil(t, tx.Signature)
}

// TestHelperFunctions tests the helper functions for extracting fields
func TestHelperFunctions(t *testing.T) {
	t.Run("rpc.GetStringField", func(t *testing.T) {
		m := map[string]interface{}{
			"str":   "value",
			"int":   123,
			"empty": "",
		}

		assert.Equal(t, "value", rpc.GetStringField(m, "str"))
		assert.Equal(t, "", rpc.GetStringField(m, "int"))     // Not a string
		assert.Equal(t, "", rpc.GetStringField(m, "empty"))   // Empty string
		assert.Equal(t, "", rpc.GetStringField(m, "missing")) // Missing key
	})

	t.Run("rpc.GetIntField", func(t *testing.T) {
		m := map[string]interface{}{
			"int":     123,
			"int64":   int64(456),
			"float64": float64(789),
			"string":  "999",
		}

		assert.Equal(t, 123, rpc.GetIntField(m, "int"))
		assert.Equal(t, 456, rpc.GetIntField(m, "int64"))
		assert.Equal(t, 789, rpc.GetIntField(m, "float64"))
		assert.Equal(t, 0, rpc.GetIntField(m, "string"))  // Not a number type
		assert.Equal(t, 0, rpc.GetIntField(m, "missing")) // Missing key
	})

	t.Run("rpc.GetOptionalUint32Field", func(t *testing.T) {
		m := map[string]interface{}{
			"int":     30,
			"int64":   int64(40),
			"float64": float64(50),
			"uint32":  uint32(60),
			"string":  "70",
		}

		val := rpc.GetOptionalUint32Field(m, "int")
		require.NotNil(t, val)
		assert.Equal(t, uint32(30), *val)

		val = rpc.GetOptionalUint32Field(m, "int64")
		require.NotNil(t, val)
		assert.Equal(t, uint32(40), *val)

		val = rpc.GetOptionalUint32Field(m, "float64")
		require.NotNil(t, val)
		assert.Equal(t, uint32(50), *val)

		val = rpc.GetOptionalUint32Field(m, "uint32")
		require.NotNil(t, val)
		assert.Equal(t, uint32(60), *val)

		val = rpc.GetOptionalUint32Field(m, "string")
		assert.Nil(t, val) // Not a number type

		val = rpc.GetOptionalUint32Field(m, "missing")
		assert.Nil(t, val) // Missing key
	})

	t.Run("rpc.GetOptionalUint64Field", func(t *testing.T) {
		m := map[string]interface{}{
			"int":     100,
			"int64":   int64(200),
			"float64": float64(300),
			"uint64":  uint64(400),
		}

		val := rpc.GetOptionalUint64Field(m, "int")
		require.NotNil(t, val)
		assert.Equal(t, uint64(100), *val)

		val = rpc.GetOptionalUint64Field(m, "int64")
		require.NotNil(t, val)
		assert.Equal(t, uint64(200), *val)

		val = rpc.GetOptionalUint64Field(m, "float64")
		require.NotNil(t, val)
		assert.Equal(t, uint64(300), *val)

		val = rpc.GetOptionalUint64Field(m, "uint64")
		require.NotNil(t, val)
		assert.Equal(t, uint64(400), *val)

		val = rpc.GetOptionalUint64Field(m, "missing")
		assert.Nil(t, val)
	})
}

// TestAllMessageTypes verifies all currently implemented message types can be parsed
// Note: EditStake is not yet implemented in rpc.ParseMessage(), so it's excluded
func TestAllMessageTypes(t *testing.T) {
	messageTypes := []struct {
		msgType  string
		expected rpc.MessageType
	}{
		{"send", rpc.MsgTypeSend},
		{"delegate", rpc.MsgTypeDelegate},
		{"undelegate", rpc.MsgTypeUndelegate},
		{"stake", rpc.MsgTypeStake},
		{"unstake", rpc.MsgTypeUnstake},
		// {"edit_stake", MsgTypeEditStake}, // Not yet implemented in rpc.ParseMessage()
		{"vote", rpc.MsgTypeVote},
		{"proposal", rpc.MsgTypeProposal},
		{"contract", rpc.MsgTypeContract},
		{"system", rpc.MsgTypeSystem},
		{"unknown_type", rpc.MsgTypeUnknown},
	}

	for _, tt := range messageTypes {
		t.Run(tt.msgType, func(t *testing.T) {
			detected := rpc.DetectMessageType(tt.msgType, map[string]interface{}{})
			assert.Equal(t, tt.expected, detected)
		})
	}
}

// TestEditStakeMessage - SKIPPED: EditStake parsing is not yet implemented in rpc.ParseMessage()
// This test is included for completeness but currently skipped
func TestEditStakeMessage(t *testing.T) {
	t.Skip("EditStake message parsing not yet implemented in rpc.ParseMessage()")
}

// TestUnstakeMessage tests the Unstake message type
func TestUnstakeMessage(t *testing.T) {
	msgData := map[string]interface{}{
		"staker": "unstaker1",
		"pool":   "pool1",
		"amount": 3000,
	}

	msg, err := rpc.ParseMessage("unstake", msgData)
	require.NoError(t, err)

	unstakeMsg, ok := msg.(*rpc.UnstakeMessage)
	require.True(t, ok)

	assert.Equal(t, rpc.MsgTypeUnstake, unstakeMsg.Type())
	assert.Equal(t, "unstaker1", unstakeMsg.GetSigner())
	assert.Equal(t, uint64(3000), unstakeMsg.Amount)

	counterparty := unstakeMsg.GetCounterparty()
	require.NotNil(t, counterparty)
	assert.Equal(t, "pool1", *counterparty)
}

// TestParsePauseMessage tests parsing of pause transactions
func TestParsePauseMessage(t *testing.T) {
	t.Run("parses pause message correctly", func(t *testing.T) {
		msgMap := map[string]interface{}{
			"address": "0x123",
		}

		msg, err := rpc.ParseMessage("pause", msgMap)
		require.NoError(t, err)
		require.NotNil(t, msg)

		pauseMsg, ok := msg.(*rpc.PauseMessage)
		require.True(t, ok, "expected PauseMessage type")

		// Verify message type
		assert.Equal(t, rpc.MsgTypePause, pauseMsg.Type())

		// Verify signer is extracted correctly
		assert.Equal(t, "0x123", pauseMsg.GetSigner())

		// Verify non-applicable getters return nil
		assert.Nil(t, pauseMsg.GetCounterparty())
		assert.Nil(t, pauseMsg.GetAmount())
		assert.Nil(t, pauseMsg.GetValidatorAddress())
		assert.Nil(t, pauseMsg.GetChainID())
		assert.Nil(t, pauseMsg.GetCommitteeID())
		assert.Nil(t, pauseMsg.GetOrderID())
		assert.Nil(t, pauseMsg.GetRecipient())
	})
}

// TestParseUnpauseMessage tests parsing of unpause transactions
func TestParseUnpauseMessage(t *testing.T) {
	t.Run("parses unpause message correctly", func(t *testing.T) {
		msgMap := map[string]interface{}{
			"address": "0x456",
		}

		msg, err := rpc.ParseMessage("unpause", msgMap)
		require.NoError(t, err)
		require.NotNil(t, msg)

		unpauseMsg, ok := msg.(*rpc.UnpauseMessage)
		require.True(t, ok, "expected UnpauseMessage type")

		// Verify message type
		assert.Equal(t, rpc.MsgTypeUnpause, unpauseMsg.Type())

		// Verify signer is extracted correctly
		assert.Equal(t, "0x456", unpauseMsg.GetSigner())

		// Verify non-applicable getters return nil
		assert.Nil(t, unpauseMsg.GetCounterparty())
		assert.Nil(t, unpauseMsg.GetAmount())
		assert.Nil(t, unpauseMsg.GetValidatorAddress())
		assert.Nil(t, unpauseMsg.GetChainID())
		assert.Nil(t, unpauseMsg.GetCommitteeID())
		assert.Nil(t, unpauseMsg.GetOrderID())
		assert.Nil(t, unpauseMsg.GetRecipient())
	})
}

// TestParseChangeParameterMessage tests parsing of change parameter transactions
func TestParseChangeParameterMessage(t *testing.T) {
	t.Run("parses changeParameter message correctly", func(t *testing.T) {
		msgMap := map[string]interface{}{
			"param_key":   "maxBlockSize",
			"param_value": "1000000",
			"signer":      "0x123",
		}

		msg, err := rpc.ParseMessage("changeParameter", msgMap)
		require.NoError(t, err)
		require.NotNil(t, msg)

		changeParamMsg, ok := msg.(*rpc.ChangeParameterMessage)
		require.True(t, ok, "expected ChangeParameterMessage type")

		// Verify message type
		assert.Equal(t, rpc.MsgTypeChangeParameter, changeParamMsg.Type())

		// Verify all relevant fields are extracted correctly
		assert.Equal(t, "0x123", changeParamMsg.GetSigner())

		paramKey := changeParamMsg.GetParamKey()
		require.NotNil(t, paramKey)
		assert.Equal(t, "maxBlockSize", *paramKey)

		paramValue := changeParamMsg.GetParamValue()
		require.NotNil(t, paramValue)
		assert.Equal(t, "1000000", *paramValue)

		// Verify non-applicable getters return nil
		assert.Nil(t, changeParamMsg.GetCounterparty())
		assert.Nil(t, changeParamMsg.GetAmount())
		assert.Nil(t, changeParamMsg.GetValidatorAddress())
		assert.Nil(t, changeParamMsg.GetChainID())
		assert.Nil(t, changeParamMsg.GetCommitteeID())
		assert.Nil(t, changeParamMsg.GetOrderID())
		assert.Nil(t, changeParamMsg.GetRecipient())
	})
}

// TestParseDAOTransferMessage tests parsing of DAO transfer transactions
func TestParseDAOTransferMessage(t *testing.T) {
	t.Run("parses daoTransfer message correctly", func(t *testing.T) {
		msgMap := map[string]interface{}{
			"from_address": "0x123",
			"to_address":   "0x456",
			"amount":       float64(5000),
		}

		msg, err := rpc.ParseMessage("daoTransfer", msgMap)
		require.NoError(t, err)
		require.NotNil(t, msg)

		daoTransferMsg, ok := msg.(*rpc.DAOTransferMessage)
		require.True(t, ok, "expected DAOTransferMessage type")

		// Verify message type
		assert.Equal(t, rpc.MsgTypeDAOTransfer, daoTransferMsg.Type())

		// Verify all relevant fields are extracted correctly
		assert.Equal(t, "0x123", daoTransferMsg.GetSigner())

		recipient := daoTransferMsg.GetRecipient()
		require.NotNil(t, recipient)
		assert.Equal(t, "0x456", *recipient)

		amount := daoTransferMsg.GetAmount()
		require.NotNil(t, amount)
		assert.Equal(t, uint64(5000), *amount)

		// Verify non-applicable getters return nil
		assert.Nil(t, daoTransferMsg.GetCounterparty())
		assert.Nil(t, daoTransferMsg.GetValidatorAddress())
		assert.Nil(t, daoTransferMsg.GetChainID())
		assert.Nil(t, daoTransferMsg.GetCommitteeID())
		assert.Nil(t, daoTransferMsg.GetOrderID())
		assert.Nil(t, daoTransferMsg.GetParamKey())
		assert.Nil(t, daoTransferMsg.GetParamValue())
	})
}

// TestParseCertificateResultsMessage tests parsing of certificate results transactions
func TestParseCertificateResultsMessage(t *testing.T) {
	t.Run("parses certificateResults message correctly", func(t *testing.T) {
		msgMap := map[string]interface{}{
			"signer":           "0x123",
			"certificate_data": "cert123",
		}

		msg, err := rpc.ParseMessage("certificateResults", msgMap)
		require.NoError(t, err)
		require.NotNil(t, msg)

		certResultsMsg, ok := msg.(*rpc.CertificateResultsMessage)
		require.True(t, ok, "expected CertificateResultsMessage type")

		// Verify message type
		assert.Equal(t, rpc.MsgTypeCertificateResults, certResultsMsg.Type())

		// Verify signer is extracted correctly
		assert.Equal(t, "0x123", certResultsMsg.GetSigner())

		// Verify certificate data is stored in the struct
		assert.Equal(t, "cert123", certResultsMsg.CertificateData)

		// Verify non-applicable getters return nil
		assert.Nil(t, certResultsMsg.GetCounterparty())
		assert.Nil(t, certResultsMsg.GetAmount())
		assert.Nil(t, certResultsMsg.GetValidatorAddress())
		assert.Nil(t, certResultsMsg.GetChainID())
		assert.Nil(t, certResultsMsg.GetCommitteeID())
		assert.Nil(t, certResultsMsg.GetOrderID())
		assert.Nil(t, certResultsMsg.GetRecipient())
		assert.Nil(t, certResultsMsg.GetParamKey())
		assert.Nil(t, certResultsMsg.GetParamValue())
	})
}

// TestParseSubsidyMessage tests parsing of subsidy transactions
func TestParseSubsidyMessage(t *testing.T) {
	t.Run("parses subsidy message correctly", func(t *testing.T) {
		msgMap := map[string]interface{}{
			"from_address": "0x123",
			"to_address":   "0x456",
			"amount":       float64(3000),
			"committee_id": float64(5),
		}

		msg, err := rpc.ParseMessage("subsidy", msgMap)
		require.NoError(t, err)
		require.NotNil(t, msg)

		subsidyMsg, ok := msg.(*rpc.SubsidyMessage)
		require.True(t, ok, "expected SubsidyMessage type")

		// Verify message type
		assert.Equal(t, rpc.MsgTypeSubsidy, subsidyMsg.Type())

		// Verify all relevant fields are extracted correctly
		assert.Equal(t, "0x123", subsidyMsg.GetSigner())

		recipient := subsidyMsg.GetRecipient()
		require.NotNil(t, recipient)
		assert.Equal(t, "0x456", *recipient)

		amount := subsidyMsg.GetAmount()
		require.NotNil(t, amount)
		assert.Equal(t, uint64(3000), *amount)

		committeeID := subsidyMsg.GetCommitteeID()
		require.NotNil(t, committeeID)
		assert.Equal(t, uint64(5), *committeeID)

		// Verify non-applicable getters return nil
		assert.Nil(t, subsidyMsg.GetCounterparty())
		assert.Nil(t, subsidyMsg.GetValidatorAddress())
		assert.Nil(t, subsidyMsg.GetChainID())
		assert.Nil(t, subsidyMsg.GetOrderID())
		assert.Nil(t, subsidyMsg.GetParamKey())
		assert.Nil(t, subsidyMsg.GetParamValue())
	})
}

// TestParseCreateOrderMessage tests parsing of create order transactions
func TestParseCreateOrderMessage(t *testing.T) {
	t.Run("parses createOrder message correctly", func(t *testing.T) {
		msgMap := map[string]interface{}{
			"signer":      "0x123",
			"order_id":    "order123",
			"chain_id":    float64(2),
			"sell_amount": float64(1000),
			"buy_amount":  float64(900),
			"price":       0.9,
		}

		msg, err := rpc.ParseMessage("createOrder", msgMap)
		require.NoError(t, err)
		require.NotNil(t, msg)

		createOrderMsg, ok := msg.(*rpc.CreateOrderMessage)
		require.True(t, ok, "expected CreateOrderMessage type")

		// Verify message type
		assert.Equal(t, rpc.MsgTypeCreateOrder, createOrderMsg.Type())

		// Verify all relevant fields are extracted correctly
		assert.Equal(t, "0x123", createOrderMsg.GetSigner())

		orderID := createOrderMsg.GetOrderID()
		require.NotNil(t, orderID)
		assert.Equal(t, "order123", *orderID)

		chainID := createOrderMsg.GetChainID()
		require.NotNil(t, chainID)
		assert.Equal(t, uint64(2), *chainID)

		sellAmount := createOrderMsg.GetSellAmount()
		require.NotNil(t, sellAmount)
		assert.Equal(t, uint64(1000), *sellAmount)

		buyAmount := createOrderMsg.GetBuyAmount()
		require.NotNil(t, buyAmount)
		assert.Equal(t, uint64(900), *buyAmount)

		price := createOrderMsg.GetPrice()
		require.NotNil(t, price)
		assert.Equal(t, 0.9, *price)

		// Verify non-applicable getters return nil
		assert.Nil(t, createOrderMsg.GetCounterparty())
		assert.Nil(t, createOrderMsg.GetAmount())
		assert.Nil(t, createOrderMsg.GetValidatorAddress())
		assert.Nil(t, createOrderMsg.GetCommitteeID())
		assert.Nil(t, createOrderMsg.GetRecipient())
		assert.Nil(t, createOrderMsg.GetParamKey())
		assert.Nil(t, createOrderMsg.GetParamValue())
		assert.Nil(t, createOrderMsg.GetLiquidityAmount())
	})
}

// TestParseEditOrderMessage tests parsing of edit order transactions
func TestParseEditOrderMessage(t *testing.T) {
	t.Run("parses editOrder message correctly", func(t *testing.T) {
		msgMap := map[string]interface{}{
			"signer":   "0x123",
			"order_id": "order123",
			"price":    0.95,
		}

		msg, err := rpc.ParseMessage("editOrder", msgMap)
		require.NoError(t, err)
		require.NotNil(t, msg)

		editOrderMsg, ok := msg.(*rpc.EditOrderMessage)
		require.True(t, ok, "expected EditOrderMessage type")

		// Verify message type
		assert.Equal(t, rpc.MsgTypeEditOrder, editOrderMsg.Type())

		// Verify all relevant fields are extracted correctly
		assert.Equal(t, "0x123", editOrderMsg.GetSigner())

		orderID := editOrderMsg.GetOrderID()
		require.NotNil(t, orderID)
		assert.Equal(t, "order123", *orderID)

		price := editOrderMsg.GetPrice()
		require.NotNil(t, price)
		assert.Equal(t, 0.95, *price)

		// Verify non-applicable getters return nil
		assert.Nil(t, editOrderMsg.GetCounterparty())
		assert.Nil(t, editOrderMsg.GetAmount())
		assert.Nil(t, editOrderMsg.GetValidatorAddress())
		assert.Nil(t, editOrderMsg.GetChainID())
		assert.Nil(t, editOrderMsg.GetSellAmount())
		assert.Nil(t, editOrderMsg.GetBuyAmount())
		assert.Nil(t, editOrderMsg.GetCommitteeID())
		assert.Nil(t, editOrderMsg.GetRecipient())
		assert.Nil(t, editOrderMsg.GetParamKey())
		assert.Nil(t, editOrderMsg.GetParamValue())
		assert.Nil(t, editOrderMsg.GetLiquidityAmount())
	})
}

// TestParseDeleteOrderMessage tests parsing of delete order transactions
func TestParseDeleteOrderMessage(t *testing.T) {
	t.Run("parses deleteOrder message correctly", func(t *testing.T) {
		msgMap := map[string]interface{}{
			"signer":   "0x123",
			"order_id": "order123",
		}

		msg, err := rpc.ParseMessage("deleteOrder", msgMap)
		require.NoError(t, err)
		require.NotNil(t, msg)

		deleteOrderMsg, ok := msg.(*rpc.DeleteOrderMessage)
		require.True(t, ok, "expected DeleteOrderMessage type")

		// Verify message type
		assert.Equal(t, rpc.MsgTypeDeleteOrder, deleteOrderMsg.Type())

		// Verify all relevant fields are extracted correctly
		assert.Equal(t, "0x123", deleteOrderMsg.GetSigner())

		orderID := deleteOrderMsg.GetOrderID()
		require.NotNil(t, orderID)
		assert.Equal(t, "order123", *orderID)

		// Verify non-applicable getters return nil
		assert.Nil(t, deleteOrderMsg.GetCounterparty())
		assert.Nil(t, deleteOrderMsg.GetAmount())
		assert.Nil(t, deleteOrderMsg.GetValidatorAddress())
		assert.Nil(t, deleteOrderMsg.GetChainID())
		assert.Nil(t, deleteOrderMsg.GetSellAmount())
		assert.Nil(t, deleteOrderMsg.GetBuyAmount())
		assert.Nil(t, deleteOrderMsg.GetPrice())
		assert.Nil(t, deleteOrderMsg.GetCommitteeID())
		assert.Nil(t, deleteOrderMsg.GetRecipient())
		assert.Nil(t, deleteOrderMsg.GetParamKey())
		assert.Nil(t, deleteOrderMsg.GetParamValue())
		assert.Nil(t, deleteOrderMsg.GetLiquidityAmount())
	})
}

// TestParseDexLimitOrderMessage tests parsing of DEX limit order transactions
func TestParseDexLimitOrderMessage(t *testing.T) {
	t.Run("parses dexLimitOrder message correctly", func(t *testing.T) {
		msgMap := map[string]interface{}{
			"from":        "0x123",
			"chain_id":    float64(2),
			"sell_amount": float64(1000),
			"buy_amount":  float64(950),
			"price":       0.95,
		}

		msg, err := rpc.ParseMessage("dexLimitOrder", msgMap)
		require.NoError(t, err)
		require.NotNil(t, msg)

		dexLimitOrderMsg, ok := msg.(*rpc.DexLimitOrderMessage)
		require.True(t, ok, "expected DexLimitOrderMessage type")

		// Verify message type
		assert.Equal(t, rpc.MsgTypeDexLimitOrder, dexLimitOrderMsg.Type())

		// Verify all relevant fields are extracted correctly
		assert.Equal(t, "0x123", dexLimitOrderMsg.GetSigner())

		chainID := dexLimitOrderMsg.GetChainID()
		require.NotNil(t, chainID)
		assert.Equal(t, uint64(2), *chainID)

		sellAmount := dexLimitOrderMsg.GetSellAmount()
		require.NotNil(t, sellAmount)
		assert.Equal(t, uint64(1000), *sellAmount)

		buyAmount := dexLimitOrderMsg.GetBuyAmount()
		require.NotNil(t, buyAmount)
		assert.Equal(t, uint64(950), *buyAmount)

		price := dexLimitOrderMsg.GetPrice()
		require.NotNil(t, price)
		assert.Equal(t, 0.95, *price)

		// Verify non-applicable getters return nil
		assert.Nil(t, dexLimitOrderMsg.GetCounterparty())
		assert.Nil(t, dexLimitOrderMsg.GetAmount())
		assert.Nil(t, dexLimitOrderMsg.GetValidatorAddress())
		assert.Nil(t, dexLimitOrderMsg.GetOrderID())
		assert.Nil(t, dexLimitOrderMsg.GetCommitteeID())
		assert.Nil(t, dexLimitOrderMsg.GetRecipient())
		assert.Nil(t, dexLimitOrderMsg.GetParamKey())
		assert.Nil(t, dexLimitOrderMsg.GetParamValue())
		assert.Nil(t, dexLimitOrderMsg.GetLiquidityAmount())
	})
}

// TestParseDexLiquidityDepositMessage tests parsing of DEX liquidity deposit transactions
func TestParseDexLiquidityDepositMessage(t *testing.T) {
	t.Run("parses dexLiquidityDeposit message correctly", func(t *testing.T) {
		msgMap := map[string]interface{}{
			"from":     "0x123",
			"chain_id": float64(2),
			"amount":   float64(5000),
		}

		msg, err := rpc.ParseMessage("dexLiquidityDeposit", msgMap)
		require.NoError(t, err)
		require.NotNil(t, msg)

		dexDepositMsg, ok := msg.(*rpc.DexLiquidityDepositMessage)
		require.True(t, ok, "expected DexLiquidityDepositMessage type")

		// Verify message type
		assert.Equal(t, rpc.MsgTypeDexLiquidityDeposit, dexDepositMsg.Type())

		// Verify all relevant fields are extracted correctly
		assert.Equal(t, "0x123", dexDepositMsg.GetSigner())

		chainID := dexDepositMsg.GetChainID()
		require.NotNil(t, chainID)
		assert.Equal(t, uint64(2), *chainID)

		liquidityAmount := dexDepositMsg.GetLiquidityAmount()
		require.NotNil(t, liquidityAmount)
		assert.Equal(t, uint64(5000), *liquidityAmount)

		// Verify non-applicable getters return nil
		assert.Nil(t, dexDepositMsg.GetCounterparty())
		assert.Nil(t, dexDepositMsg.GetAmount())
		assert.Nil(t, dexDepositMsg.GetValidatorAddress())
		assert.Nil(t, dexDepositMsg.GetSellAmount())
		assert.Nil(t, dexDepositMsg.GetBuyAmount())
		assert.Nil(t, dexDepositMsg.GetOrderID())
		assert.Nil(t, dexDepositMsg.GetPrice())
		assert.Nil(t, dexDepositMsg.GetCommitteeID())
		assert.Nil(t, dexDepositMsg.GetRecipient())
		assert.Nil(t, dexDepositMsg.GetParamKey())
		assert.Nil(t, dexDepositMsg.GetParamValue())
	})
}

// TestParseDexLiquidityWithdrawMessage tests parsing of DEX liquidity withdraw transactions
func TestParseDexLiquidityWithdrawMessage(t *testing.T) {
	t.Run("parses dexLiquidityWithdraw message correctly", func(t *testing.T) {
		msgMap := map[string]interface{}{
			"from":     "0x123",
			"chain_id": float64(2),
			"amount":   float64(5000),
		}

		msg, err := rpc.ParseMessage("dexLiquidityWithdraw", msgMap)
		require.NoError(t, err)
		require.NotNil(t, msg)

		dexWithdrawMsg, ok := msg.(*rpc.DexLiquidityWithdrawMessage)
		require.True(t, ok, "expected DexLiquidityWithdrawMessage type")

		// Verify message type
		assert.Equal(t, rpc.MsgTypeDexLiquidityWithdraw, dexWithdrawMsg.Type())

		// Verify all relevant fields are extracted correctly
		assert.Equal(t, "0x123", dexWithdrawMsg.GetSigner())

		chainID := dexWithdrawMsg.GetChainID()
		require.NotNil(t, chainID)
		assert.Equal(t, uint64(2), *chainID)

		liquidityAmount := dexWithdrawMsg.GetLiquidityAmount()
		require.NotNil(t, liquidityAmount)
		assert.Equal(t, uint64(5000), *liquidityAmount)

		// Verify non-applicable getters return nil
		assert.Nil(t, dexWithdrawMsg.GetCounterparty())
		assert.Nil(t, dexWithdrawMsg.GetAmount())
		assert.Nil(t, dexWithdrawMsg.GetValidatorAddress())
		assert.Nil(t, dexWithdrawMsg.GetSellAmount())
		assert.Nil(t, dexWithdrawMsg.GetBuyAmount())
		assert.Nil(t, dexWithdrawMsg.GetOrderID())
		assert.Nil(t, dexWithdrawMsg.GetPrice())
		assert.Nil(t, dexWithdrawMsg.GetCommitteeID())
		assert.Nil(t, dexWithdrawMsg.GetRecipient())
		assert.Nil(t, dexWithdrawMsg.GetParamKey())
		assert.Nil(t, dexWithdrawMsg.GetParamValue())
	})
}

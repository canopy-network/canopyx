package rpc

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseSendMessage tests parsing of send transactions
func TestParseSendMessage(t *testing.T) {
	tests := []struct {
		name            string
		msgType         string
		msgData         map[string]interface{}
		expectedSigner  string
		expectedTo      string
		expectedAmount  uint64
		expectedMemo    string
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
			msg, err := parseMessage(tt.msgType, tt.msgData)
			require.NoError(t, err)
			require.NotNil(t, msg)

			sendMsg, ok := msg.(*SendMessage)
			require.True(t, ok, "expected SendMessage type")

			assert.Equal(t, MsgTypeSend, sendMsg.Type())
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
			msg, err := parseMessage(tt.msgType, tt.msgData)
			require.NoError(t, err)

			delegateMsg, ok := msg.(*DelegateMessage)
			require.True(t, ok, "expected DelegateMessage type")

			assert.Equal(t, MsgTypeDelegate, delegateMsg.Type())
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

	msg, err := parseMessage("undelegate", msgData)
	require.NoError(t, err)

	undelegateMsg, ok := msg.(*UndelegateMessage)
	require.True(t, ok)

	assert.Equal(t, MsgTypeUndelegate, undelegateMsg.Type())
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
			msg, err := parseMessage("stake", tt.msgData)
			require.NoError(t, err)

			stakeMsg, ok := msg.(*StakeMessage)
			require.True(t, ok)

			assert.Equal(t, MsgTypeStake, stakeMsg.Type())

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

	msg, err := parseMessage("vote", msgData)
	require.NoError(t, err)

	voteMsg, ok := msg.(*VoteMessage)
	require.True(t, ok)

	assert.Equal(t, MsgTypeVote, voteMsg.Type())
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

	msg, err := parseMessage("proposal", msgData)
	require.NoError(t, err)

	proposalMsg, ok := msg.(*ProposalMessage)
	require.True(t, ok)

	assert.Equal(t, MsgTypeProposal, proposalMsg.Type())
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
			msg, err := parseMessage("contract", tt.msgData)
			require.NoError(t, err)

			contractMsg, ok := msg.(*ContractMessage)
			require.True(t, ok)

			assert.Equal(t, MsgTypeContract, contractMsg.Type())

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

	msg, err := parseMessage("system", msgData)
	require.NoError(t, err)

	systemMsg, ok := msg.(*SystemMessage)
	require.True(t, ok)

	assert.Equal(t, MsgTypeSystem, systemMsg.Type())
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
			msg, err := parseMessage("completely_unknown_type", tt.msgData)
			require.NoError(t, err)

			unknownMsg, ok := msg.(*UnknownMessage)
			require.True(t, ok)

			assert.Equal(t, MsgTypeUnknown, unknownMsg.Type())
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
		expectedType MessageType
	}{
		{
			name:         "explicit send",
			msgType:      "send",
			msgData:      map[string]interface{}{},
			expectedType: MsgTypeSend,
		},
		{
			name:    "infer send from toAddress",
			msgType: "unknown",
			msgData: map[string]interface{}{
				"toAddress": "addr1",
			},
			expectedType: MsgTypeSend,
		},
		{
			name:    "infer delegate from validatorAddress and delegator",
			msgType: "unknown",
			msgData: map[string]interface{}{
				"validatorAddress": "val1",
				"delegator":        "del1",
			},
			expectedType: MsgTypeDelegate,
		},
		{
			name:    "infer undelegate from validatorAddress without delegator",
			msgType: "unknown",
			msgData: map[string]interface{}{
				"validatorAddress": "val1",
			},
			expectedType: MsgTypeUndelegate,
		},
		{
			name:    "infer stake from pool and staker",
			msgType: "unknown",
			msgData: map[string]interface{}{
				"pool":   "pool1",
				"staker": "staker1",
			},
			expectedType: MsgTypeStake,
		},
		{
			name:    "infer unstake from pool without staker",
			msgType: "unknown",
			msgData: map[string]interface{}{
				"pool": "pool1",
			},
			expectedType: MsgTypeUnstake,
		},
		{
			name:    "infer vote from proposalId and voter",
			msgType: "unknown",
			msgData: map[string]interface{}{
				"proposalId": 1,
				"voter":      "voter1",
			},
			expectedType: MsgTypeVote,
		},
		{
			name:         "unknown type with no inference clues",
			msgType:      "unknown",
			msgData:      map[string]interface{}{},
			expectedType: MsgTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected := detectMessageType(tt.msgType, tt.msgData)
			assert.Equal(t, tt.expectedType, detected)
		})
	}
}

// TestMessageInterfaceMethods tests that all message types implement interface methods correctly
func TestMessageInterfaceMethods(t *testing.T) {
	tests := []struct {
		name               string
		msg                Message
		expectedType       MessageType
		expectedSigner     string
		expectCounterparty bool
		expectedCounterpty string
		expectAmount       bool
		expectedAmount     uint64
	}{
		{
			name: "SendMessage",
			msg: &SendMessage{
				FromAddress: "sender",
				ToAddress:   "receiver",
				Amount:      1000,
			},
			expectedType:       MsgTypeSend,
			expectedSigner:     "sender",
			expectCounterparty: true,
			expectedCounterpty: "receiver",
			expectAmount:       true,
			expectedAmount:     1000,
		},
		{
			name: "DelegateMessage",
			msg: &DelegateMessage{
				Delegator:        "delegator",
				ValidatorAddress: "validator",
				Amount:           2000,
			},
			expectedType:       MsgTypeDelegate,
			expectedSigner:     "delegator",
			expectCounterparty: true,
			expectedCounterpty: "validator",
			expectAmount:       true,
			expectedAmount:     2000,
		},
		{
			name: "VoteMessage",
			msg: &VoteMessage{
				Voter:      "voter",
				ProposalID: 5,
				Option:     "yes",
			},
			expectedType:       MsgTypeVote,
			expectedSigner:     "voter",
			expectCounterparty: false,
			expectAmount:       false,
		},
		{
			name: "ProposalMessage",
			msg: &ProposalMessage{
				Proposer:    "proposer",
				Title:       "Test",
				Description: "Test proposal",
				Deposit:     5000,
			},
			expectedType:       MsgTypeProposal,
			expectedSigner:     "proposer",
			expectCounterparty: false,
			expectAmount:       true,
			expectedAmount:     5000,
		},
		{
			name: "ContractMessage with value",
			msg: &ContractMessage{
				Caller:          "caller",
				ContractAddress: "contract",
				Method:          "call",
				Value:           func() *uint64 { v := uint64(300); return &v }(),
			},
			expectedType:       MsgTypeContract,
			expectedSigner:     "caller",
			expectCounterparty: true,
			expectedCounterpty: "contract",
			expectAmount:       true,
			expectedAmount:     300,
		},
		{
			name: "ContractMessage without value",
			msg: &ContractMessage{
				Caller:          "caller",
				ContractAddress: "contract",
				Method:          "query",
			},
			expectedType:       MsgTypeContract,
			expectedSigner:     "caller",
			expectCounterparty: true,
			expectedCounterpty: "contract",
			expectAmount:       false,
		},
		{
			name: "SystemMessage",
			msg: &SystemMessage{
				Executor: "admin",
				Action:   "upgrade",
			},
			expectedType:       MsgTypeSystem,
			expectedSigner:     "admin",
			expectCounterparty: false,
			expectAmount:       false,
		},
		{
			name: "UnknownMessage",
			msg: &UnknownMessage{
				Signer: "unknown_user",
			},
			expectedType:       MsgTypeUnknown,
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
		msg  Message
	}{
		{
			name: "SendMessage",
			msg: &SendMessage{
				FromAddress: "alice",
				ToAddress:   "bob",
				Amount:      1000,
				Memo:        "payment",
			},
		},
		{
			name: "VoteMessage",
			msg: &VoteMessage{
				Voter:      "voter1",
				ProposalID: 42,
				Option:     "yes",
			},
		},
		{
			name: "ContractMessage",
			msg: &ContractMessage{
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

	rpcTx := &Transaction{
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
	assert.Equal(t, uint64(99), tx.CreatedHeight)

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
	rpcTx := &Transaction{
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
	t.Run("getStringField", func(t *testing.T) {
		m := map[string]interface{}{
			"str":   "value",
			"int":   123,
			"empty": "",
		}

		assert.Equal(t, "value", getStringField(m, "str"))
		assert.Equal(t, "", getStringField(m, "int"))      // Not a string
		assert.Equal(t, "", getStringField(m, "empty"))    // Empty string
		assert.Equal(t, "", getStringField(m, "missing"))  // Missing key
	})

	t.Run("getIntField", func(t *testing.T) {
		m := map[string]interface{}{
			"int":     123,
			"int64":   int64(456),
			"float64": float64(789),
			"string":  "999",
		}

		assert.Equal(t, 123, getIntField(m, "int"))
		assert.Equal(t, 456, getIntField(m, "int64"))
		assert.Equal(t, 789, getIntField(m, "float64"))
		assert.Equal(t, 0, getIntField(m, "string"))   // Not a number type
		assert.Equal(t, 0, getIntField(m, "missing"))  // Missing key
	})

	t.Run("getOptionalUint32Field", func(t *testing.T) {
		m := map[string]interface{}{
			"int":     30,
			"int64":   int64(40),
			"float64": float64(50),
			"uint32":  uint32(60),
			"string":  "70",
		}

		val := getOptionalUint32Field(m, "int")
		require.NotNil(t, val)
		assert.Equal(t, uint32(30), *val)

		val = getOptionalUint32Field(m, "int64")
		require.NotNil(t, val)
		assert.Equal(t, uint32(40), *val)

		val = getOptionalUint32Field(m, "float64")
		require.NotNil(t, val)
		assert.Equal(t, uint32(50), *val)

		val = getOptionalUint32Field(m, "uint32")
		require.NotNil(t, val)
		assert.Equal(t, uint32(60), *val)

		val = getOptionalUint32Field(m, "string")
		assert.Nil(t, val) // Not a number type

		val = getOptionalUint32Field(m, "missing")
		assert.Nil(t, val) // Missing key
	})

	t.Run("getOptionalUint64Field", func(t *testing.T) {
		m := map[string]interface{}{
			"int":     100,
			"int64":   int64(200),
			"float64": float64(300),
			"uint64":  uint64(400),
		}

		val := getOptionalUint64Field(m, "int")
		require.NotNil(t, val)
		assert.Equal(t, uint64(100), *val)

		val = getOptionalUint64Field(m, "int64")
		require.NotNil(t, val)
		assert.Equal(t, uint64(200), *val)

		val = getOptionalUint64Field(m, "float64")
		require.NotNil(t, val)
		assert.Equal(t, uint64(300), *val)

		val = getOptionalUint64Field(m, "uint64")
		require.NotNil(t, val)
		assert.Equal(t, uint64(400), *val)

		val = getOptionalUint64Field(m, "missing")
		assert.Nil(t, val)
	})
}

// TestAllMessageTypes verifies all currently implemented message types can be parsed
// Note: EditStake is not yet implemented in parseMessage(), so it's excluded
func TestAllMessageTypes(t *testing.T) {
	messageTypes := []struct {
		msgType  string
		expected MessageType
	}{
		{"send", MsgTypeSend},
		{"delegate", MsgTypeDelegate},
		{"undelegate", MsgTypeUndelegate},
		{"stake", MsgTypeStake},
		{"unstake", MsgTypeUnstake},
		// {"edit_stake", MsgTypeEditStake}, // Not yet implemented in parseMessage()
		{"vote", MsgTypeVote},
		{"proposal", MsgTypeProposal},
		{"contract", MsgTypeContract},
		{"system", MsgTypeSystem},
		{"unknown_type", MsgTypeUnknown},
	}

	for _, tt := range messageTypes {
		t.Run(tt.msgType, func(t *testing.T) {
			detected := detectMessageType(tt.msgType, map[string]interface{}{})
			assert.Equal(t, tt.expected, detected)
		})
	}
}

// TestEditStakeMessage - SKIPPED: EditStake parsing is not yet implemented in parseMessage()
// This test is included for completeness but currently skipped
func TestEditStakeMessage(t *testing.T) {
	t.Skip("EditStake message parsing not yet implemented in parseMessage()")
}

// TestUnstakeMessage tests the Unstake message type
func TestUnstakeMessage(t *testing.T) {
	msgData := map[string]interface{}{
		"staker": "unstaker1",
		"pool":   "pool1",
		"amount": 3000,
	}

	msg, err := parseMessage("unstake", msgData)
	require.NoError(t, err)

	unstakeMsg, ok := msg.(*UnstakeMessage)
	require.True(t, ok)

	assert.Equal(t, MsgTypeUnstake, unstakeMsg.Type())
	assert.Equal(t, "unstaker1", unstakeMsg.GetSigner())
	assert.Equal(t, uint64(3000), unstakeMsg.Amount)

	counterparty := unstakeMsg.GetCounterparty()
	require.NotNil(t, counterparty)
	assert.Equal(t, "pool1", *counterparty)
}
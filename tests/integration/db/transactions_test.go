package db

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/tests/integration/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInsertTransactions tests inserting transactions into the single table
func TestInsertTransactions(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainDB, err := helpers.NewChainDb(ctx, "test_chain_txs")
	require.NoError(t, err)
	defer chainDB.Close()

	err = chainDB.InitializeDB(ctx)
	require.NoError(t, err)

	blockTime := time.Now().UTC()

	// Create test transactions of various types
	txs := []*indexer.Transaction{
		{
			Height:        100,
			TxHash:        "send_tx_1",
			Time:          blockTime,
			HeightTime:    blockTime,
			MessageType:   "send",
			Signer:        "alice",
			Counterparty:  strPtr("bob"),
			Amount:        uint64Ptr(1000),
			Fee:           10,
			Msg:           `{"from_address":"alice","to_address":"bob","amount":1000}`,
			PublicKey:     strPtr("pubkey_alice"),
			Signature:     strPtr("sig_alice"),
			CreatedHeight: 99,
		},
		{
			Height:        100,
			TxHash:        "delegate_tx_1",
			Time:          blockTime,
			HeightTime:    blockTime,
			MessageType:   "delegate",
			Signer:        "charlie",
			Counterparty:  strPtr("validator1"),
			Amount:        uint64Ptr(5000),
			Fee:           15,
			Msg:           `{"delegator":"charlie","validator_address":"validator1","amount":5000}`,
			PublicKey:     strPtr("pubkey_charlie"),
			Signature:     strPtr("sig_charlie"),
			CreatedHeight: 99,
		},
		{
			Height:        100,
			TxHash:        "vote_tx_1",
			Time:          blockTime,
			HeightTime:    blockTime,
			MessageType:   "vote",
			Signer:        "david",
			Counterparty:  nil, // Votes have no counterparty
			Amount:        nil, // Votes have no amount
			Fee:           5,
			Msg:           `{"voter":"david","proposal_id":42,"option":"yes"}`,
			PublicKey:     strPtr("pubkey_david"),
			Signature:     strPtr("sig_david"),
			CreatedHeight: 99,
		},
	}

	err = chainDB.InsertTransactions(ctx, txs)
	require.NoError(t, err)

	// Verify transactions were inserted by querying
	results, err := chainDB.QueryTransactions(ctx, 0, 10, false)
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

// TestGetTransactionByHash tests retrieving a transaction by its hash
func TestGetTransactionByHash(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainDB, err := helpers.NewChainDb(ctx, "test_chain_get_tx")
	require.NoError(t, err)
	defer chainDB.Close()

	err = chainDB.InitializeDB(ctx)
	require.NoError(t, err)

	blockTime := time.Now().UTC()

	// Insert a transaction
	tx := &indexer.Transaction{
		Height:        200,
		TxHash:        "unique_hash_123",
		Time:          blockTime,
		HeightTime:    blockTime,
		MessageType:   "contract",
		Signer:        "contract_caller",
		Counterparty:  strPtr("0xcontract123"),
		Amount:        uint64Ptr(500),
		Fee:           20,
		Msg:           `{"caller":"contract_caller","contract_address":"0xcontract123","method":"transfer","value":500}`,
		PublicKey:     strPtr("pubkey_contract"),
		Signature:     strPtr("sig_contract"),
		CreatedHeight: 199,
	}

	err = chainDB.InsertTransactions(ctx, []*indexer.Transaction{tx})
	require.NoError(t, err)

	// Retrieve by hash
	retrieved, err := chainDB.GetTransactionByHash(ctx, "unique_hash_123")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, uint64(200), retrieved.Height)
	assert.Equal(t, "unique_hash_123", retrieved.TxHash)
	assert.Equal(t, "contract", retrieved.MessageType)
	assert.Equal(t, "contract_caller", retrieved.Signer)

	require.NotNil(t, retrieved.Counterparty)
	assert.Equal(t, "0xcontract123", *retrieved.Counterparty)

	require.NotNil(t, retrieved.Amount)
	assert.Equal(t, uint64(500), *retrieved.Amount)

	assert.Equal(t, uint64(20), retrieved.Fee)

	// Verify msg field contains JSON
	assert.NotEmpty(t, retrieved.Msg)
	var msgData map[string]interface{}
	err = json.Unmarshal([]byte(retrieved.Msg), &msgData)
	require.NoError(t, err)
	assert.Equal(t, "contract_caller", msgData["caller"])
}

// TestQueryTransactions tests querying transactions with pagination
func TestQueryTransactions(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainDB, err := helpers.NewChainDb(ctx, "test_chain_query_txs")
	require.NoError(t, err)
	defer chainDB.Close()

	err = chainDB.InitializeDB(ctx)
	require.NoError(t, err)

	blockTime := time.Now().UTC()

	// Insert transactions at different heights
	txs := []*indexer.Transaction{
		{Height: 100, TxHash: "tx_100_1", Time: blockTime, HeightTime: blockTime, MessageType: "send", Signer: "user1", Fee: 10, Msg: `{}`, CreatedHeight: 99},
		{Height: 100, TxHash: "tx_100_2", Time: blockTime, HeightTime: blockTime, MessageType: "send", Signer: "user2", Fee: 10, Msg: `{}`, CreatedHeight: 99},
		{Height: 101, TxHash: "tx_101_1", Time: blockTime, HeightTime: blockTime, MessageType: "vote", Signer: "user3", Fee: 5, Msg: `{}`, CreatedHeight: 100},
		{Height: 102, TxHash: "tx_102_1", Time: blockTime, HeightTime: blockTime, MessageType: "delegate", Signer: "user4", Fee: 15, Msg: `{}`, CreatedHeight: 101},
		{Height: 103, TxHash: "tx_103_1", Time: blockTime, HeightTime: blockTime, MessageType: "send", Signer: "user5", Fee: 10, Msg: `{}`, CreatedHeight: 102},
	}

	err = chainDB.InsertTransactions(ctx, txs)
	require.NoError(t, err)

	tests := []struct {
		name             string
		cursor           uint64
		limit            int
		sortDesc         bool
		expectedMinCount int
		expectedMaxCount int
	}{
		{
			name:             "get all ascending",
			cursor:           0,
			limit:            10,
			sortDesc:         false,
			expectedMinCount: 5,
			expectedMaxCount: 5,
		},
		{
			name:             "get all descending",
			cursor:           0,
			limit:            10,
			sortDesc:         true,
			expectedMinCount: 5,
			expectedMaxCount: 5,
		},
		{
			name:             "cursor pagination ascending",
			cursor:           100,
			limit:            10,
			sortDesc:         false,
			expectedMinCount: 3, // Heights 101, 102, 103
			expectedMaxCount: 3,
		},
		{
			name:             "cursor pagination descending",
			cursor:           103,
			limit:            10,
			sortDesc:         true,
			expectedMinCount: 4, // Heights 102, 101, 100
			expectedMaxCount: 4,
		},
		{
			name:             "limit results",
			cursor:           0,
			limit:            2,
			sortDesc:         false,
			expectedMinCount: 2,
			expectedMaxCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := chainDB.QueryTransactions(ctx, tt.cursor, tt.limit, tt.sortDesc)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(results), tt.expectedMinCount)
			assert.LessOrEqual(t, len(results), tt.expectedMaxCount)

			// Verify ordering
			if len(results) > 1 {
				for i := 1; i < len(results); i++ {
					if tt.sortDesc {
						assert.GreaterOrEqual(t, results[i-1].Height, results[i].Height,
							"expected descending order")
					} else {
						assert.LessOrEqual(t, results[i-1].Height, results[i].Height,
							"expected ascending order")
					}
				}
			}
		})
	}
}

// TestQueryTransactionsWithTypeFilter tests filtering transactions by message type
func TestQueryTransactionsWithTypeFilter(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainDB, err := helpers.NewChainDb(ctx, "test_chain_filter_txs")
	require.NoError(t, err)
	defer chainDB.Close()

	err = chainDB.InitializeDB(ctx)
	require.NoError(t, err)

	blockTime := time.Now().UTC()

	// Insert transactions of different types
	txs := []*indexer.Transaction{
		{Height: 100, TxHash: "send_1", Time: blockTime, HeightTime: blockTime, MessageType: "send", Signer: "user1", Fee: 10, Msg: `{}`, CreatedHeight: 99},
		{Height: 101, TxHash: "send_2", Time: blockTime, HeightTime: blockTime, MessageType: "send", Signer: "user2", Fee: 10, Msg: `{}`, CreatedHeight: 100},
		{Height: 102, TxHash: "vote_1", Time: blockTime, HeightTime: blockTime, MessageType: "vote", Signer: "user3", Fee: 5, Msg: `{}`, CreatedHeight: 101},
		{Height: 103, TxHash: "delegate_1", Time: blockTime, HeightTime: blockTime, MessageType: "delegate", Signer: "user4", Fee: 15, Msg: `{}`, CreatedHeight: 102},
		{Height: 104, TxHash: "send_3", Time: blockTime, HeightTime: blockTime, MessageType: "send", Signer: "user5", Fee: 10, Msg: `{}`, CreatedHeight: 103},
		{Height: 105, TxHash: "vote_2", Time: blockTime, HeightTime: blockTime, MessageType: "vote", Signer: "user6", Fee: 5, Msg: `{}`, CreatedHeight: 104},
	}

	err = chainDB.InsertTransactions(ctx, txs)
	require.NoError(t, err)

	tests := []struct {
		name          string
		messageType   string
		expectedCount int
	}{
		{
			name:          "filter send transactions",
			messageType:   "send",
			expectedCount: 3,
		},
		{
			name:          "filter vote transactions",
			messageType:   "vote",
			expectedCount: 2,
		},
		{
			name:          "filter delegate transactions",
			messageType:   "delegate",
			expectedCount: 1,
		},
		{
			name:          "filter unknown type",
			messageType:   "nonexistent",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := chainDB.QueryTransactionsWithFilter(ctx, 0, 100, false, tt.messageType)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedCount, len(results))

			// Verify all results match the filter
			for _, tx := range results {
				assert.Equal(t, tt.messageType, tx.MessageType)
			}
		})
	}
}

// TestTransactionCompression verifies that ZSTD compression is configured (we can't directly test compression)
func TestTransactionCompression(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainDB, err := helpers.NewChainDb(ctx, "test_chain_compression")
	require.NoError(t, err)
	defer chainDB.Close()

	err = chainDB.InitializeDB(ctx)
	require.NoError(t, err)

	// Check table structure includes codec specifications
	columns, err := chainDB.DescribeTable(ctx, "txs")
	require.NoError(t, err)

	// Find the msg, public_key, and signature columns
	var msgCol, pubKeyCol, sigCol *string
	for _, col := range columns {
		if col.Name == "msg" {
			msgCol = &col.Type
		}
		if col.Name == "public_key" {
			pubKeyCol = &col.Type
		}
		if col.Name == "signature" {
			sigCol = &col.Type
		}
	}

	// Verify columns exist (compression codec is applied at create time)
	assert.NotNil(t, msgCol, "msg column should exist")
	assert.NotNil(t, pubKeyCol, "public_key column should exist")
	assert.NotNil(t, sigCol, "signature column should exist")

	// Insert a transaction with large JSON to verify it can be stored and retrieved
	blockTime := time.Now().UTC()
	largeMsg := `{"caller":"user","contract":"0x123","data":"` + string(make([]byte, 1000)) + `"}`

	tx := &indexer.Transaction{
		Height:        300,
		TxHash:        "large_msg_tx",
		Time:          blockTime,
		HeightTime:    blockTime,
		MessageType:   "contract",
		Signer:        "user",
		Fee:           25,
		Msg:           largeMsg,
		PublicKey:     strPtr("very_long_public_key_" + string(make([]byte, 100))),
		Signature:     strPtr("very_long_signature_" + string(make([]byte, 100))),
		CreatedHeight: 299,
	}

	err = chainDB.InsertTransactions(ctx, []*indexer.Transaction{tx})
	require.NoError(t, err)

	// Retrieve and verify
	retrieved, err := chainDB.GetTransactionByHash(ctx, "large_msg_tx")
	require.NoError(t, err)
	assert.Equal(t, largeMsg, retrieved.Msg)
	require.NotNil(t, retrieved.PublicKey)
	require.NotNil(t, retrieved.Signature)
}

// TestTransactionDeduplication tests ReplacingMergeTree deduplication behavior
func TestTransactionDeduplication(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainDB, err := helpers.NewChainDb(ctx, "test_chain_dedup")
	require.NoError(t, err)
	defer chainDB.Close()

	err = chainDB.InitializeDB(ctx)
	require.NoError(t, err)

	blockTime := time.Now().UTC()

	// Insert a transaction
	tx1 := &indexer.Transaction{
		Height:        400,
		TxHash:        "dedup_tx",
		Time:          blockTime,
		HeightTime:    blockTime,
		MessageType:   "send",
		Signer:        "alice",
		Fee:           10,
		Msg:           `{"version":"v1"}`,
		CreatedHeight: 399,
	}

	err = chainDB.InsertTransactions(ctx, []*indexer.Transaction{tx1})
	require.NoError(t, err)

	// Insert duplicate with higher CreatedHeight (should replace)
	tx2 := &indexer.Transaction{
		Height:        400,
		TxHash:        "dedup_tx", // Same primary key
		Time:          blockTime,
		HeightTime:    blockTime,
		MessageType:   "send",
		Signer:        "alice",
		Fee:           15, // Different fee
		Msg:           `{"version":"v2"}`,
		CreatedHeight: 400, // Higher version
	}

	err = chainDB.InsertTransactions(ctx, []*indexer.Transaction{tx2})
	require.NoError(t, err)

	// Query with FINAL to get deduplicated result
	retrieved, err := chainDB.GetTransactionByHash(ctx, "dedup_tx")
	require.NoError(t, err)

	// Should get the version with higher CreatedHeight
	assert.Equal(t, uint64(400), retrieved.CreatedHeight)
	assert.Equal(t, uint64(15), retrieved.Fee)
	assert.Contains(t, retrieved.Msg, "v2")
}

// TestTransactionAllMessageTypes tests inserting and retrieving all 11 message types
func TestTransactionAllMessageTypes(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainDB, err := helpers.NewChainDb(ctx, "test_chain_all_types")
	require.NoError(t, err)
	defer chainDB.Close()

	err = chainDB.InitializeDB(ctx)
	require.NoError(t, err)

	blockTime := time.Now().UTC()

	// Create one transaction of each type
	txs := []*indexer.Transaction{
		{
			Height: 500, TxHash: "send_tx", Time: blockTime, HeightTime: blockTime,
			MessageType: "send", Signer: "alice", Counterparty: strPtr("bob"),
			Amount: uint64Ptr(1000), Fee: 10,
			Msg: `{"from_address":"alice","to_address":"bob","amount":1000}`,
			CreatedHeight: 499,
		},
		{
			Height: 500, TxHash: "delegate_tx", Time: blockTime, HeightTime: blockTime,
			MessageType: "delegate", Signer: "charlie", Counterparty: strPtr("validator1"),
			Amount: uint64Ptr(5000), Fee: 15,
			Msg: `{"delegator":"charlie","validator_address":"validator1","amount":5000}`,
			CreatedHeight: 499,
		},
		{
			Height: 500, TxHash: "undelegate_tx", Time: blockTime, HeightTime: blockTime,
			MessageType: "undelegate", Signer: "dave", Counterparty: strPtr("validator2"),
			Amount: uint64Ptr(2000), Fee: 12,
			Msg: `{"delegator":"dave","validator_address":"validator2","amount":2000}`,
			CreatedHeight: 499,
		},
		{
			Height: 500, TxHash: "stake_tx", Time: blockTime, HeightTime: blockTime,
			MessageType: "stake", Signer: "eve", Counterparty: strPtr("pool1"),
			Amount: uint64Ptr(3000), Fee: 18,
			Msg: `{"staker":"eve","pool":"pool1","amount":3000}`,
			CreatedHeight: 499,
		},
		{
			Height: 500, TxHash: "unstake_tx", Time: blockTime, HeightTime: blockTime,
			MessageType: "unstake", Signer: "frank", Counterparty: strPtr("pool2"),
			Amount: uint64Ptr(1500), Fee: 14,
			Msg: `{"staker":"frank","pool":"pool2","amount":1500}`,
			CreatedHeight: 499,
		},
		{
			Height: 500, TxHash: "edit_stake_tx", Time: blockTime, HeightTime: blockTime,
			MessageType: "edit_stake", Signer: "grace", Counterparty: strPtr("pool3"),
			Amount: uint64Ptr(2500), Fee: 16,
			Msg: `{"staker":"grace","pool":"pool3","amount":2500}`,
			CreatedHeight: 499,
		},
		{
			Height: 500, TxHash: "vote_tx", Time: blockTime, HeightTime: blockTime,
			MessageType: "vote", Signer: "henry", Counterparty: nil,
			Amount: nil, Fee: 5,
			Msg: `{"voter":"henry","proposal_id":1,"option":"yes"}`,
			CreatedHeight: 499,
		},
		{
			Height: 500, TxHash: "proposal_tx", Time: blockTime, HeightTime: blockTime,
			MessageType: "proposal", Signer: "iris", Counterparty: nil,
			Amount: uint64Ptr(10000), Fee: 25,
			Msg: `{"proposer":"iris","title":"Test","deposit":10000}`,
			CreatedHeight: 499,
		},
		{
			Height: 500, TxHash: "contract_tx", Time: blockTime, HeightTime: blockTime,
			MessageType: "contract", Signer: "jack", Counterparty: strPtr("0xcontract"),
			Amount: uint64Ptr(500), Fee: 30,
			Msg: `{"caller":"jack","contract_address":"0xcontract","value":500}`,
			CreatedHeight: 499,
		},
		{
			Height: 500, TxHash: "system_tx", Time: blockTime, HeightTime: blockTime,
			MessageType: "system", Signer: "admin", Counterparty: nil,
			Amount: nil, Fee: 0,
			Msg: `{"executor":"admin","action":"upgrade"}`,
			CreatedHeight: 499,
		},
		{
			Height: 500, TxHash: "unknown_tx", Time: blockTime, HeightTime: blockTime,
			MessageType: "unknown", Signer: "mystery", Counterparty: nil,
			Amount: nil, Fee: 1,
			Msg: `{"signer":"mystery","data":{}}`,
			CreatedHeight: 499,
		},
	}

	err = chainDB.InsertTransactions(ctx, txs)
	require.NoError(t, err)

	// Verify all transactions can be retrieved
	results, err := chainDB.QueryTransactions(ctx, 0, 20, false)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 11)

	// Verify each type exists
	typesSeen := make(map[string]bool)
	for _, tx := range results {
		if tx.Height == 500 {
			typesSeen[tx.MessageType] = true
		}
	}

	expectedTypes := []string{
		"send", "delegate", "undelegate", "stake", "unstake",
		"edit_stake", "vote", "proposal", "contract", "system", "unknown",
	}

	for _, expectedType := range expectedTypes {
		assert.True(t, typesSeen[expectedType], "message type %s not found", expectedType)
	}

	// Verify specific transactions can be retrieved by hash
	for _, tx := range txs {
		retrieved, err := chainDB.GetTransactionByHash(ctx, tx.TxHash)
		require.NoError(t, err, "failed to retrieve %s", tx.TxHash)
		assert.Equal(t, tx.MessageType, retrieved.MessageType)
		assert.Equal(t, tx.Signer, retrieved.Signer)
	}
}

// TestTransactionNullableFields tests that nullable fields work correctly
func TestTransactionNullableFields(t *testing.T) {
	ctx := context.Background()
	if helpers.SkipIfNoDB(ctx, t) {
		return
	}

	chainDB, err := helpers.NewChainDb(ctx, "test_chain_nullable")
	require.NoError(t, err)
	defer chainDB.Close()

	err = chainDB.InitializeDB(ctx)
	require.NoError(t, err)

	blockTime := time.Now().UTC()

	// Transaction with all nullable fields as nil
	tx := &indexer.Transaction{
		Height:        600,
		TxHash:        "nullable_tx",
		Time:          blockTime,
		HeightTime:    blockTime,
		MessageType:   "vote",
		Signer:        "voter",
		Counterparty:  nil, // NULL
		Amount:        nil, // NULL
		Fee:           5,
		Msg:           `{"voter":"voter"}`,
		PublicKey:     nil, // NULL
		Signature:     nil, // NULL
		CreatedHeight: 599,
	}

	err = chainDB.InsertTransactions(ctx, []*indexer.Transaction{tx})
	require.NoError(t, err)

	retrieved, err := chainDB.GetTransactionByHash(ctx, "nullable_tx")
	require.NoError(t, err)

	assert.Nil(t, retrieved.Counterparty)
	assert.Nil(t, retrieved.Amount)
	assert.Nil(t, retrieved.PublicKey)
	assert.Nil(t, retrieved.Signature)
}

// Helper functions

func strPtr(s string) *string {
	return &s
}

func uint64Ptr(u uint64) *uint64 {
	return &u
}
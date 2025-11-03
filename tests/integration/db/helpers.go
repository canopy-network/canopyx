package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	adminpkg "github.com/canopy-network/canopyx/pkg/db/admin"
	chainpkg "github.com/canopy-network/canopyx/pkg/db/chain"
	adminmodels "github.com/canopy-network/canopyx/pkg/db/models/admin"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Test Fixtures and Utilities

// createAdminStore creates a new AdminStore for testing with automatic cleanup.
func createAdminStore(t *testing.T, dbName string) *adminpkg.AdminDB {
	t.Helper()

	// Use context with timeout to prevent hangs
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := testLogger.With(zap.String("test", t.Name()))
	adminDB, err := adminpkg.New(ctx, logger, dbName)
	require.NoError(t, err, "failed to create admin store")

	// Register cleanup: drop database BEFORE closing connection
	t.Cleanup(func() {
		// Drop the database while connection is still open
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()

		dropQuery := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName)
		if err := adminDB.Exec(dropCtx, dropQuery); err != nil {
			t.Logf("failed to drop database %s: %v", dbName, err)
		}

		// Now close the connection
		if err := adminDB.Close(); err != nil {
			t.Logf("failed to close admin store: %v", err)
		}
	})

	return adminDB
}

// createChainStore creates a new ChainStore for testing with automatic cleanup.
func createChainStore(t *testing.T, chainID uint64) *chainpkg.DB {
	t.Helper()

	// Use context with timeout to prevent hangs
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := testLogger.With(
		zap.String("test", t.Name()),
		zap.Uint64("chainID", chainID),
	)
	chainDB, err := chainpkg.New(ctx, logger, chainID)
	require.NoError(t, err, "failed to create chain store")

	// Register cleanup: drop database BEFORE closing connection
	t.Cleanup(func() {
		// Drop the database while connection is still open
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()

		dbName := fmt.Sprintf("%d", chainID)
		dropQuery := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName)
		if err := chainDB.Exec(dropCtx, dropQuery); err != nil {
			t.Logf("failed to drop database %s: %v", dbName, err)
		}

		// Now close the connection
		if err := chainDB.Close(); err != nil {
			t.Logf("failed to close chain store: %v", err)
		}
	})

	return chainDB
}

// cleanupDatabase is deprecated - cleanup is now done in createAdminStore and createChainStore
// using the existing connection before closing it. This prevents connection leaks.
// Keeping this function for backward compatibility but it does nothing.
func cleanupDatabase(t *testing.T, dbName string) {
	t.Helper()
	// No-op: cleanup is now handled in createAdminStore/createChainStore
	// before closing the connection to avoid creating extra connections
}

// Data Generators for Admin Models

// generateChain creates a test Chain with sensible defaults.
func generateChain(chainID uint64) *adminmodels.Chain {
	now := time.Now().UTC()
	return &adminmodels.Chain{
		ChainID:                   chainID,
		ChainName:                 fmt.Sprintf("testchain-%d", chainID),
		RPCEndpoints:              []string{fmt.Sprintf("http://rpc-%d.test:26657", chainID)},
		Paused:                    0,
		Deleted:                   0,
		Image:                     "canopy-indexer:test",
		MinReplicas:               1,
		MaxReplicas:               3,
		Notes:                     fmt.Sprintf("Test chain %d", chainID),
		CreatedAt:                 now,
		UpdatedAt:                 now,
		RPCHealthStatus:           "unknown",
		RPCHealthMessage:          "",
		RPCHealthUpdatedAt:        now,
		QueueHealthStatus:         "unknown",
		QueueHealthMessage:        "",
		QueueHealthUpdatedAt:      now,
		DeploymentHealthStatus:    "unknown",
		DeploymentHealthMessage:   "",
		DeploymentHealthUpdatedAt: now,
		OverallHealthStatus:       "unknown",
		OverallHealthUpdatedAt:    now,
	}
}

// generateIndexProgress creates a test IndexProgress entry.
func generateIndexProgress(chainID, height uint64) *adminmodels.IndexProgress {
	return &adminmodels.IndexProgress{
		ChainID:        chainID,
		Height:         height,
		IndexedAt:      time.Now().UTC(),
		IndexingTime:   0.5,
		IndexingTimeMs: 500.0,
		IndexingDetail: `{"fetch_block":100,"process_txs":200,"insert_db":200}`,
	}
}

// generateReindexRequest creates a test ReindexRequest.
func generateReindexRequest(chainID uint64, height uint64) *adminmodels.ReindexRequest {
	return &adminmodels.ReindexRequest{
		ChainID:     chainID,
		Height:      height,
		RequestedAt: time.Now().UTC(),
		Status:      "pending",
		RequestedBy: "test-user",
	}
}

// Data Generators for Chain/Indexer Models

// generateBlock creates a test Block.
func generateBlock(height uint64) *indexermodels.Block {
	return &indexermodels.Block{
		Height:          height,
		Hash:            fmt.Sprintf("hash-%d", height),
		Time:            time.Now().UTC().Add(-time.Duration(1000-height) * time.Second),
		LastBlockHash:   fmt.Sprintf("hash-%d", height-1),
		ProposerAddress: fmt.Sprintf("proposer-%d", height%10),
		Size:            1024 + int32(height%1000),
	}
}

// generateTransaction creates a test Transaction.
func generateTransaction(height uint64, txIndex int) *indexermodels.Transaction {
	blockTime := time.Now().UTC().Add(-time.Duration(1000-height) * time.Second)
	signer := fmt.Sprintf("signer-%d", txIndex%5)
	counterparty := fmt.Sprintf("recipient-%d", txIndex%7)
	amount := uint64(1000000 + txIndex*1000)

	return &indexermodels.Transaction{
		Height:       height,
		TxHash:       fmt.Sprintf("tx-%d-%d", height, txIndex),
		Time:         blockTime,
		HeightTime:   blockTime,
		MessageType:  "send",
		Signer:       signer,
		Counterparty: &counterparty,
		Amount:       &amount,
		Fee:          1000,
		Msg:          fmt.Sprintf(`{"amount":%d,"to":"%s"}`, amount, counterparty),
	}
}

// generateAccount creates a test Account.
func generateAccount(address string, height uint64, amount uint64) *indexermodels.Account {
	return &indexermodels.Account{
		Address:    address,
		Amount:     amount,
		Height:     height,
		HeightTime: time.Now().UTC().Add(-time.Duration(1000-height) * time.Second),
	}
}

// generateEvent creates a test Event.
func generateEvent(height uint64, eventIndex int) *indexermodels.Event {
	reference := fmt.Sprintf("tx-%d", height)
	return &indexermodels.Event{
		Height:     height,
		ChainID:    1,
		Address:    fmt.Sprintf("addr-%d", eventIndex),
		Reference:  reference,
		EventType:  "transfer",
		Msg:        fmt.Sprintf(`{"sender":"addr1","recipient":"addr2","amount":%d}`, eventIndex*100),
		HeightTime: time.Now().UTC().Add(-time.Duration(1000-height) * time.Second),
	}
}

// generateBlockSummary creates test BlockSummary data.
func generateBlockSummary(height uint64, blockTime time.Time, numTxs uint32) *indexermodels.BlockSummary {
	return &indexermodels.BlockSummary{
		Height:      height,
		HeightTime:  blockTime,
		NumTxs:      numTxs,
		NumTxsSend:  numTxs / 2,
		NumTxsStake: numTxs / 4,
		NumTxsVote:  numTxs / 4,
	}
}

// generatePool creates a test Pool.
func generatePool(poolID uint64, height uint64) *indexermodels.Pool {
	pool := &indexermodels.Pool{
		PoolID:      poolID,
		ChainID:     1,
		Amount:      1000000,
		TotalPoints: 500000,
		LPCount:     10,
		Height:      height,
		HeightTime:  time.Now().UTC().Add(-time.Duration(1000-height) * time.Second),
	}
	pool.CalculatePoolIDs()
	return pool
}

// generateOrder creates a test Order.
func generateOrder(orderID string, height uint64) *indexermodels.Order {
	return &indexermodels.Order{
		OrderID:         orderID,
		Committee:       1,
		AmountForSale:   1000000,
		RequestedAmount: 900000,
		SellerAddress:   "seller-addr",
		Height:          height,
		HeightTime:      time.Now().UTC().Add(-time.Duration(1000-height) * time.Second),
		Status:          "open",
	}
}

// generateDexPrice creates a test DexPrice.
func generateDexPrice(height uint64) *indexermodels.DexPrice {
	return &indexermodels.DexPrice{
		LocalChainID:  1,
		RemoteChainID: 2,
		LocalPool:     1000000,
		RemotePool:    1500000,
		PriceE6:       1500000, // 1.5 scaled by 1e6
		Height:        height,
		HeightTime:    time.Now().UTC().Add(-time.Duration(1000-height) * time.Second),
	}
}

// generateDexOrder creates a test DexOrder.
func generateDexOrder(orderID string, height uint64) *indexermodels.DexOrder {
	return &indexermodels.DexOrder{
		OrderID:         orderID,
		Committee:       1,
		Address:         "owner-addr",
		AmountForSale:   1000000,
		RequestedAmount: 1100000,
		State:           "future",
		Success:         false,
		SoldAmount:      0,
		BoughtAmount:    0,
		LocalOrigin:     true,
		LockedHeight:    0,
		Height:          height,
		HeightTime:      time.Now().UTC().Add(-time.Duration(1000-height) * time.Second),
	}
}

// generateDexDeposit creates a test DexDeposit.
func generateDexDeposit(depositID string, height uint64) *indexermodels.DexDeposit {
	return &indexermodels.DexDeposit{
		OrderID:        depositID,
		Committee:      1,
		Address:        "depositor-addr",
		Amount:         1000000,
		State:          "pending",
		LocalOrigin:    true,
		PointsReceived: 0,
		Height:         height,
		HeightTime:     time.Now().UTC().Add(-time.Duration(1000-height) * time.Second),
	}
}

// generateDexWithdrawal creates a test DexWithdrawal.
func generateDexWithdrawal(withdrawalID string, height uint64) *indexermodels.DexWithdrawal {
	return &indexermodels.DexWithdrawal{
		OrderID:      withdrawalID,
		Committee:    1,
		Address:      "withdrawer-addr",
		Percent:      50,
		State:        "pending",
		LocalAmount:  0,
		RemoteAmount: 0,
		PointsBurned: 0,
		Height:       height,
		HeightTime:   time.Now().UTC().Add(-time.Duration(1000-height) * time.Second),
	}
}

// generateDexPoolPointsByHolder creates a test DexPoolPointsByHolder.
func generateDexPoolPointsByHolder(holderAddress string, height uint64) *indexermodels.DexPoolPointsByHolder {
	return &indexermodels.DexPoolPointsByHolder{
		Address:             holderAddress,
		PoolID:              indexermodels.LiquidityPoolAddend + 1, // Committee 1
		Committee:           1,
		Points:              1000,
		LiquidityPoolPoints: 500,
		LiquidityPoolID:     indexermodels.LiquidityPoolAddend + 1,
		Height:              height,
		HeightTime:          time.Now().UTC().Add(-time.Duration(1000-height) * time.Second),
	}
}

// generateParams creates a test Params.
func generateParams(height uint64) *indexermodels.Params {
	return &indexermodels.Params{
		Height:                    height,
		HeightTime:                time.Now().UTC().Add(-time.Duration(1000-height) * time.Second),
		BlockSize:                 1000000,
		ProtocolVersion:           "1.0.0",
		RootChainID:               1,
		Retired:                   0,
		UnstakingBlocks:           21600,
		MaxPauseBlocks:            1000,
		DoubleSignSlashPercentage: 5000,
		NonSignSlashPercentage:    100,
		MaxNonSign:                500,
		NonSignWindow:             1000,
		MaxCommittees:             100,
		MaxCommitteeSize:          100,
		EarlyWithdrawalPenalty:    1000,
		DelegateUnstakingBlocks:   21600,
		MinimumOrderSize:          100000,
		StakePercentForSubsidized: 5000,
		MaxSlashPerCommittee:      1000000,
		DelegateRewardPercentage:  5000,
		BuyDeadlineBlocks:         100,
		LockOrderFeeMultiplier:    2,
		SendFee:                   1000,
		StakeFee:                  10000,
		EditStakeFee:              5000,
		UnstakeFee:                5000,
		PauseFee:                  1000,
		UnpauseFee:                1000,
		ChangeParameterFee:        100000,
		DaoTransferFee:            50000,
		CertificateResultsFee:     25000,
		SubsidyFee:                10000,
		CreateOrderFee:            5000,
		EditOrderFee:              2500,
		DeleteOrderFee:            1000,
		DexLimitOrderFee:          1000,
		DexLiquidityDepositFee:    1000,
		DexLiquidityWithdrawFee:   1000,
		DaoRewardPercentage:       1000,
	}
}

// generateValidator creates a test Validator.
func generateValidator(address string, height uint64) *indexermodels.Validator {
	return &indexermodels.Validator{
		Address:         address,
		PublicKey:       fmt.Sprintf("pubkey-%s", address),
		NetAddress:      fmt.Sprintf("http://%s:26656", address),
		StakedAmount:    10000000,
		Output:          fmt.Sprintf("output-%s", address),
		Committees:      []uint64{1, 2, 3},
		MaxPausedHeight: 0,
		UnstakingHeight: 0,
		Delegate:        false,
		Compound:        false,
		Status:          "active",
		Height:          height,
		HeightTime:      time.Now().UTC().Add(-time.Duration(1000-height) * time.Second),
	}
}

// generateValidatorSigningInfo creates a test ValidatorSigningInfo.
func generateValidatorSigningInfo(address string, height uint64) *indexermodels.ValidatorSigningInfo {
	return &indexermodels.ValidatorSigningInfo{
		Address:            address,
		MissedBlocksCount:  5,
		MissedBlocksWindow: 1000,
		LastSignedHeight:   height - 5,
		StartHeight:        1,
		Height:             height,
		HeightTime:         time.Now().UTC().Add(-time.Duration(1000-height) * time.Second),
	}
}

// generateCommittee creates a test Committee.
func generateCommittee(chainID uint64, height uint64) *indexermodels.Committee {
	return &indexermodels.Committee{
		ChainID:                chainID,
		LastRootHeightUpdated:  height - 10,
		LastChainHeightUpdated: 1000,
		NumberOfSamples:        100,
		Subsidized:             true,
		Retired:                false,
		Height:                 height,
		HeightTime:             time.Now().UTC().Add(-time.Duration(1000-height) * time.Second),
	}
}

// generateCommitteeValidator creates a test CommitteeValidator.
func generateCommitteeValidator(committeeID uint64, validatorAddress string, height uint64) *indexermodels.CommitteeValidator {
	return &indexermodels.CommitteeValidator{
		CommitteeID:      committeeID,
		ValidatorAddress: validatorAddress,
		StakedAmount:     10000000,
		Status:           "active",
		Height:           height,
		HeightTime:       time.Now().UTC().Add(-time.Duration(1000-height) * time.Second),
	}
}

// generatePollSnapshot creates a test PollSnapshot.
func generatePollSnapshot(proposalHash, _ string, height uint64) *indexermodels.PollSnapshot {
	return &indexermodels.PollSnapshot{
		ProposalHash:                proposalHash,
		Height:                      height,
		ProposalURL:                 "https://proposal.test",
		AccountsApproveTokens:       1000000,
		AccountsRejectTokens:        500000,
		AccountsTotalVotedTokens:    1500000,
		AccountsTotalTokens:         2000000,
		AccountsApprovePercentage:   50,
		AccountsRejectPercentage:    25,
		AccountsVotedPercentage:     75,
		ValidatorsApproveTokens:     5000000,
		ValidatorsRejectTokens:      2000000,
		ValidatorsTotalVotedTokens:  7000000,
		ValidatorsTotalTokens:       10000000,
		ValidatorsApprovePercentage: 50,
		ValidatorsRejectPercentage:  20,
		ValidatorsVotedPercentage:   70,
		HeightTime:                  time.Now().UTC().Add(-time.Duration(1000-height) * time.Second),
	}
}

// Assertion Helpers

// assertChainEqual compares two Chain structs, ignoring timestamp precision issues.
func assertChainEqual(t *testing.T, expected, actual *adminmodels.Chain) {
	t.Helper()
	require.Equal(t, expected.ChainID, actual.ChainID)
	require.Equal(t, expected.ChainName, actual.ChainName)
	require.Equal(t, expected.RPCEndpoints, actual.RPCEndpoints)
	require.Equal(t, expected.Paused, actual.Paused)
	require.Equal(t, expected.Deleted, actual.Deleted)
	require.Equal(t, expected.Image, actual.Image)
	require.Equal(t, expected.MinReplicas, actual.MinReplicas)
	require.Equal(t, expected.MaxReplicas, actual.MaxReplicas)
	require.Equal(t, expected.Notes, actual.Notes)
	// Health status fields
	require.Equal(t, expected.RPCHealthStatus, actual.RPCHealthStatus)
	require.Equal(t, expected.QueueHealthStatus, actual.QueueHealthStatus)
	require.Equal(t, expected.DeploymentHealthStatus, actual.DeploymentHealthStatus)
	require.Equal(t, expected.OverallHealthStatus, actual.OverallHealthStatus)
}

// assertBlockEqual compares two Block structs.
func assertBlockEqual(t *testing.T, expected, actual *indexermodels.Block) {
	t.Helper()
	require.Equal(t, expected.Height, actual.Height)
	require.Equal(t, expected.Hash, actual.Hash)
	require.WithinDuration(t, expected.Time, actual.Time, time.Second)
	require.Equal(t, expected.LastBlockHash, actual.LastBlockHash)
	require.Equal(t, expected.ProposerAddress, actual.ProposerAddress)
	require.Equal(t, expected.Size, actual.Size)
}

// assertTransactionEqual compares two Transaction structs.
func assertTransactionEqual(t *testing.T, expected, actual *indexermodels.Transaction) {
	t.Helper()
	require.Equal(t, expected.Height, actual.Height)
	require.Equal(t, expected.TxHash, actual.TxHash)
	require.Equal(t, expected.MessageType, actual.MessageType)
	require.Equal(t, expected.Signer, actual.Signer)
	require.Equal(t, expected.Fee, actual.Fee)
}

// assertAccountEqual compares two Account structs.
func assertAccountEqual(t *testing.T, expected, actual *indexermodels.Account) {
	t.Helper()
	require.Equal(t, expected.Address, actual.Address)
	require.Equal(t, expected.Amount, actual.Amount)
	require.Equal(t, expected.Height, actual.Height)
}

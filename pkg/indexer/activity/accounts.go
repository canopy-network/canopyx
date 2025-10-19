package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	indexer "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"go.uber.org/zap"
)

// IndexAccounts indexes account balance changes for a given block using the snapshot-on-change pattern.
//
// Core Algorithm:
// 1. Parallel RPC fetch: Fetch RPC(height H) and RPC(height H-1) simultaneously using goroutines
// 2. Special case: If H=1, fetch RPC(1) and Genesis(0) from DB cache instead of RPC(0)
// 3. Compare: Build map of previous balances, iterate current accounts
// 4. Detect changes: If amount differs, create snapshot with correct created_height
// 5. Insert to staging: Batch insert all changed accounts
//
// Created Height Logic:
// - New account (not in previous state): created_height = current height
// - Existing account (balance changed): query created_height from production table
//
// Performance:
// - Parallel RPC fetching reduces latency by ~50% (2 concurrent requests)
// - In-cluster RPC is fast (~500-800ms total for 200k accounts)
// - Only stores changed accounts (20x storage savings vs full snapshots)
func (c *Context) IndexAccounts(ctx context.Context, input types.IndexAccountsInput) (types.IndexAccountsOutput, error) {
	start := time.Now()

	// Get chain metadata
	ch, err := c.IndexerDB.GetChain(ctx, input.ChainID)
	if err != nil {
		return types.IndexAccountsOutput{}, err
	}

	// Get chain database
	chainDb, err := c.NewChainDb(ctx, input.ChainID)
	if err != nil {
		return types.IndexAccountsOutput{}, err
	}

	// Create RPC client
	cli := c.rpcClient(ch.RPCEndpoints)

	// Parallel RPC fetch using goroutines for performance
	var (
		currentAccounts  []*rpc.RpcAccount
		previousAccounts []*rpc.RpcAccount
		currentErr       error
		previousErr      error
		wg               sync.WaitGroup
	)

	wg.Add(2)

	// Worker 1: Fetch current height accounts from RPC
	go func() {
		defer wg.Done()
		currentAccounts, currentErr = cli.AccountsByHeight(ctx, input.Height)
	}()

	// Worker 2: Fetch previous state
	// - If height 1: read genesis from DB cache
	// - Otherwise: fetch height-1 from RPC
	go func() {
		defer wg.Done()
		if input.Height == 1 {
			// Genesis case: read from cached genesis
			previousAccounts, previousErr = c.getGenesisAccounts(ctx, chainDb)
		} else if input.Height > 1 {
			// Normal case: fetch from RPC
			previousAccounts, previousErr = cli.AccountsByHeight(ctx, input.Height-1)
		}
	}()

	// Wait for both workers to complete
	wg.Wait()

	// Check for errors
	if currentErr != nil {
		return types.IndexAccountsOutput{}, fmt.Errorf("fetch current accounts at height %d: %w", input.Height, currentErr)
	}
	if previousErr != nil {
		return types.IndexAccountsOutput{}, fmt.Errorf("fetch previous accounts at height %d: %w", input.Height-1, previousErr)
	}

	// Build previous state map for O(1) lookups
	prevMap := make(map[string]uint64, len(previousAccounts))
	for _, acc := range previousAccounts {
		prevMap[acc.Address] = acc.Amount
	}

	// Compare and collect changed accounts
	changedAccounts := make([]*indexer.Account, 0)
	for _, curr := range currentAccounts {
		prevAmount, existed := prevMap[curr.Address]

		// Only create snapshot if balance changed
		if curr.Amount != prevAmount {
			var createdHeight uint64

			if !existed {
				// New account - doesn't exist in previous state
				createdHeight = input.Height
			} else {
				// Existing account - query created_height from production table
				createdHeight = c.queryCreatedHeight(ctx, chainDb, curr.Address)
				if createdHeight == 0 {
					// Edge case: not found in DB yet (parallel indexing)
					// Assume it was created at this height
					createdHeight = input.Height
				}
			}

			changedAccounts = append(changedAccounts, &indexer.Account{
				Address:       curr.Address,
				Amount:        curr.Amount,
				Height:        input.Height,
				HeightTime:    input.BlockTime,
				CreatedHeight: createdHeight,
			})
		}
	}

	// Insert to staging table
	if len(changedAccounts) > 0 {
		// Need to cast to *db.ChainDB to access Db field
		dbImpl, ok := chainDb.(*db.ChainDB)
		if !ok {
			return types.IndexAccountsOutput{}, fmt.Errorf("chainDb is not *db.ChainDB")
		}

		err = indexer.InsertAccountsStaging(ctx, dbImpl.Db, "accounts_staging", changedAccounts)
		if err != nil {
			return types.IndexAccountsOutput{}, fmt.Errorf("insert accounts staging: %w", err)
		}
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	c.Logger.Info("Indexed accounts",
		zap.String("chainId", input.ChainID),
		zap.Uint64("height", input.Height),
		zap.Int("totalAccounts", len(currentAccounts)),
		zap.Int("changedAccounts", len(changedAccounts)),
		zap.Float64("durationMs", durationMs))

	return types.IndexAccountsOutput{
		NumAccounts: uint32(len(changedAccounts)),
		DurationMs:  durationMs,
	}, nil
}

// getGenesisAccounts reads accounts from the genesis cache in the database.
// This is used when indexing height 1 to compare RPC(1) vs Genesis(0).
func (c *Context) getGenesisAccounts(ctx context.Context, chainDb db.ChainStore) ([]*rpc.RpcAccount, error) {
	// Need to cast to *db.ChainDB to access ClickHouse() method
	// The ChainStore interface doesn't expose ClickHouse() directly
	dbImpl, ok := chainDb.(*db.ChainDB)
	if !ok {
		return nil, fmt.Errorf("chainDb is not *db.ChainDB")
	}

	// Query genesis table for height 0
	var result struct {
		Data string `ch:"data"`
	}

	err := dbImpl.Db.NewSelect().
		TableExpr("genesis").
		Column("data").
		Where("height = ?", 0).
		Scan(ctx, &result)

	if err != nil {
		return nil, fmt.Errorf("query genesis cache: %w", err)
	}

	// Unmarshal JSON
	var genesis rpc.GenesisState
	if err := json.Unmarshal([]byte(result.Data), &genesis); err != nil {
		return nil, fmt.Errorf("unmarshal genesis JSON: %w", err)
	}

	return genesis.Accounts, nil
}

// queryCreatedHeight retrieves the created_height for an existing account.
// Uses FINAL to ensure we get the most recent version from ReplacingMergeTree.
func (c *Context) queryCreatedHeight(ctx context.Context, chainDb db.ChainStore, address string) uint64 {
	// Need to cast to *db.ChainDB to access Db field
	dbImpl, ok := chainDb.(*db.ChainDB)
	if !ok {
		return 0
	}

	var result struct {
		CreatedHeight uint64 `ch:"created_height"`
	}

	err := dbImpl.Db.NewSelect().
		TableExpr("accounts").
		Column("created_height").
		Where("address = ?", address).
		OrderExpr("height DESC").
		Limit(1).
		Final(). // CRITICAL: Use FINAL to deduplicate ReplacingMergeTree
		Scan(ctx, &result)

	if err != nil {
		// Account not found in DB yet - return 0
		return 0
	}

	return result.CreatedHeight
}

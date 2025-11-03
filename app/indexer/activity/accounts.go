package activity

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	indexer "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"go.uber.org/zap"
)

// IndexAccounts indexes account balance changes for a given block using the snapshot-on-change pattern.
//
// Core Algorithm:
// 1. Parallel RPC fetch: Fetch RPC(height H) and RPC(height H-1) simultaneously using goroutines
// 2. Special case: If H=1, fetch RPC(1) and Genesis(0) from DB cache instead of RPC(0)
// 3. Compare: Build map of previous balances, iterate current accounts
// 4. Detect changes: If amount differs, create snapshot
// 5. Insert to staging: Batch insert all changed accounts
//
// Created Height Tracking:
// Account creation heights are tracked via the account_created_height materialized view,
// which automatically calculates MIN(height) for each address. This approach:
// - Works correctly with parallel/unordered indexing
// - Eliminates the need to query and store created_height on every snapshot
// - Automatically updates as older blocks are indexed
//
// Performance:
// - Parallel RPC fetching reduces latency by ~50% (2 concurrent requests)
// - In-cluster RPC is fast (~500-800ms total for 200k accounts)
// - Only stores changed accounts (20x storage savings vs full snapshots)
func (c *Context) IndexAccounts(ctx context.Context, input types.IndexAccountsInput) (types.IndexAccountsOutput, error) {
	start := time.Now()

	// Get chain metadata
	ch, err := c.AdminDB.GetChain(ctx, input.ChainID)
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
		currentAccounts  []*rpc.Account
		previousAccounts []*rpc.Account
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
			// Genesis case: whatever comes at height 1 is the genesis state, so we should save them always.
			previousAccounts = make([]*rpc.Account, 0)
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
		prevAmount := prevMap[curr.Address]

		// Only create snapshot if balance changed
		if curr.Amount != prevAmount {
			changedAccounts = append(changedAccounts, &indexer.Account{
				Address:    curr.Address,
				Amount:     curr.Amount,
				Height:     input.Height,
				HeightTime: input.BlockTime,
			})
		}
	}

	// Insert to staging table
	if len(changedAccounts) > 0 {
		if err := chainDb.InsertAccountsStaging(ctx, changedAccounts); err != nil {
			return types.IndexAccountsOutput{}, fmt.Errorf("insert accounts staging: %w", err)
		}
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	c.Logger.Info("Indexed accounts",
		zap.Uint64("chainId", input.ChainID),
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
// Currently unused but kept for potential future use.
/*
func (c *Context) getGenesisAccounts(ctx context.Context, chainDb chainstore.Store) ([]*rpc.Account, error) {
	// Query genesis table for height 0
	genesisData, err := chainDb.GetGenesisData(ctx, 0)
	if err != nil {
		return nil, fmt.Errorf("query genesis cache: %w", err)
	}

	// Unmarshal JSON
	var genesis rpc.GenesisState
	if err := json.Unmarshal([]byte(genesisData), &genesis); err != nil {
		return nil, fmt.Errorf("unmarshal genesis JSON: %w", err)
	}

	return genesis.Accounts, nil
}
*/

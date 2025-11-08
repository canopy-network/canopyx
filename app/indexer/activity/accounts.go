package activity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/canopy-network/canopyx/app/indexer/types"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
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
func (ac *Context) IndexAccounts(ctx context.Context, input types.ActivityIndexAtHeight) (types.ActivityIndexAccountsOutput, error) {
	start := time.Now()

	// Create RPC client
	cli, cliErr := ac.rpcClient(ctx)
	if cliErr != nil {
		return types.ActivityIndexAccountsOutput{}, cliErr
	}

	// Get chain database
	chainDb, err := ac.GetChainDb(ctx, ac.ChainID)
	if err != nil {
		return types.ActivityIndexAccountsOutput{}, err
	}

	// Parallel RPC fetch using shared worker pool for performance
	var (
		currentAccounts  []*rpc.Account
		previousAccounts []*rpc.Account
		currentErr       error
		previousErr      error
	)

	// Get a subgroup from the shared worker pool for parallel RPC fetching
	pool := ac.WorkerPool(2)
	group := pool.NewGroupContext(ctx)
	groupCtx := group.Context()

	// Worker 1: Fetch current height accounts from RPC
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		currentAccounts, currentErr = cli.AccountsByHeight(groupCtx, input.Height)
	})

	// Worker 2: Fetch previous state
	// - If height 1: read genesis from DB cache
	// - Otherwise: fetch height-1 from RPC
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		if input.Height == 1 {
			// Genesis case: whatever comes at height 1 is the genesis state, so we should save them always.
			previousAccounts = make([]*rpc.Account, 0)
		} else if input.Height > 1 {
			// Normal case: fetch from RPC
			previousAccounts, previousErr = cli.AccountsByHeight(groupCtx, input.Height-1)
		}
	})

	// Wait for all workers to complete
	if err := group.Wait(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, pond.ErrGroupStopped) {
		ac.Logger.Warn("parallel RPC fetch encountered error",
			zap.Uint64("chainId", ac.ChainID),
			zap.Uint64("height", input.Height),
			zap.Error(err),
		)
	}

	// Check for errors
	if currentErr != nil {
		return types.ActivityIndexAccountsOutput{}, fmt.Errorf("fetch current accounts at height %d: %w", input.Height, currentErr)
	}
	if previousErr != nil {
		return types.ActivityIndexAccountsOutput{}, fmt.Errorf("fetch previous accounts at height %d: %w", input.Height-1, previousErr)
	}

	// Build previous state map for O(1) lookups
	prevMap := make(map[string]uint64, len(previousAccounts))
	for _, acc := range previousAccounts {
		prevMap[acc.Address] = acc.Amount
	}

	// Query account-related events from staging table (event-driven correlation)
	// These events provide context for account balance changes:
	// - EventReward: Validator receives block rewards
	// - EventSlash: Validator slashed for Byzantine behavior
	accountEvents, err := chainDb.GetEventsByTypeAndHeight(ctx, input.Height, true,
		rpc.EventTypeAsStr(rpc.EventTypeReward),
		rpc.EventTypeAsStr(rpc.EventTypeSlash),
	)
	if err != nil {
		return types.ActivityIndexAccountsOutput{}, fmt.Errorf("query account events at height %d: %w", input.Height, err)
	}

	// Build event maps by address for O(1) lookup
	rewardEvents := make(map[string]*indexer.Event)
	slashEvents := make(map[string]*indexer.Event)

	for _, event := range accountEvents {
		// Events have Address field which corresponds to account/validator address
		addr := event.Address

		switch event.EventType {
		case "EventReward":
			rewardEvents[addr] = event
		case "EventSlash":
			slashEvents[addr] = event
		}
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
			return types.ActivityIndexAccountsOutput{}, fmt.Errorf("insert accounts staging: %w", err)
		}
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	ac.Logger.Info("Indexed accounts",
		zap.Uint64("chainId", ac.ChainID),
		zap.Uint64("height", input.Height),
		zap.Int("totalAccounts", len(currentAccounts)),
		zap.Int("changedAccounts", len(changedAccounts)),
		zap.Int("rewardEvents", len(rewardEvents)),
		zap.Int("slashEvents", len(slashEvents)),
		zap.Float64("durationMs", durationMs))

	return types.ActivityIndexAccountsOutput{
		NumAccounts: uint32(len(changedAccounts)),
		DurationMs:  durationMs,
	}, nil
}

package activity

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// IndexDexBatch indexes DEX orders, deposits, and withdrawals for a given block height.
// This activity follows the events-first architecture and RPC(H-1) pattern.
//
// Core Algorithm:
// 1. Parallel RPC fetch: Query DexBatch(H) and NextDexBatch(H) simultaneously
// 2. Query events from staging: Get DEX-related events at height H
// 3. Create order snapshots:
//   - DexBatch orders → state="locked"
//   - NextDexBatch orders → state="future"
//   - Update orders from EventDexSwap events (state="complete")
//
// 4. Create deposit snapshots:
//   - Initial state="pending"
//   - Update from EventDexLiquidityDeposit events (state="complete")
//
// 5. Create withdrawal snapshots:
//   - Initial state="pending"
//   - Update from EventDexLiquidityWithdrawal events (state="complete")
//
// 6. Insert all entities to staging tables
//
// Event Processing Pattern:
// Events at height H describe entities from height H-1. When processing EventDexSwap at H,
// we query DexBatch at H-1 to find the order that was executed. The snapshot we create
// uses height H (when the event occurred), but entity data comes from RPC(H-1).
//
// Performance:
// - Parallel RPC fetching reduces latency by ~50% (2 concurrent requests)
// - Events are queried from staging DB (already indexed by IndexEvents activity)
// - Only changed entities are stored (snapshot-on-change pattern)
func (ac *Context) IndexDexBatch(ctx context.Context, in types.ActivityIndexAtHeight) (types.ActivityIndexDexBatchOutput, error) {
	start := time.Now()

	cli, err := ac.rpcClient(ctx)
	if err != nil {
		return types.ActivityIndexDexBatchOutput{}, err
	}

	// Acquire chain database
	chainDb, err := ac.GetChainDb(ctx, ac.ChainID)
	if err != nil {
		return types.ActivityIndexDexBatchOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", err)
	}

	// Parallel RPC fetch: ALL DexBatches(H) and ALL NextDexBatches(H) in a single call each
	// Using committee=0 returns all committees' batches
	var (
		currentBatchesH1 []*rpc.RpcDexBatch
		currentBatches   []*rpc.RpcDexBatch
		nextBatches      []*rpc.RpcDexBatch
		currentH1Err     error
		currentErr       error
		nextErr          error
		wg               sync.WaitGroup
	)

	wg.Add(3)

	// Worker 1: Fetch ALL current batches (locked orders across all committees)
	go func() {
		defer wg.Done()
		currentBatches, currentErr = cli.AllDexBatchesByHeight(ctx, in.Height)
	}()

	// Worker 2: Fetch ALL next batches (future orders across all committees)
	go func() {
		defer wg.Done()
		nextBatches, nextErr = cli.AllNextDexBatchesByHeight(ctx, in.Height)
	}()

	go func() {
		defer wg.Done()
		if in.Height <= 1 {
			currentBatchesH1 = make([]*rpc.RpcDexBatch, 0)
			return
		}

		currentBatchesH1, currentH1Err = cli.AllDexBatchesByHeight(ctx, in.Height-1)
	}()

	// Wait for both workers
	wg.Wait()

	// Check for errors (empty batches are acceptable - means no batches at this height)
	if currentErr != nil {
		return types.ActivityIndexDexBatchOutput{}, fmt.Errorf("fetch all dex batches at height %d: %w", in.Height, currentErr)
	}
	if nextErr != nil {
		return types.ActivityIndexDexBatchOutput{}, fmt.Errorf("fetch all next dex batches at height %d: %w", in.Height, nextErr)
	}
	if currentH1Err != nil {
		return types.ActivityIndexDexBatchOutput{}, fmt.Errorf("fetch all dex batches at height %d: %w", in.Height-1, currentH1Err)
	}

	// Query events from the staging table
	events, err := chainDb.GetEventsByTypeAndHeight(
		ctx, in.Height, true,
		rpc.EventTypeAsStr(rpc.EventTypeDexSwap),
		rpc.EventTypeAsStr(rpc.EventTypeDexLiquidityDeposit),
		rpc.EventTypeAsStr(rpc.EventTypeDexLiquidityWithdraw),
	)
	if err != nil {
		return types.ActivityIndexDexBatchOutput{}, fmt.Errorf("query events at height %d: %w", in.Height, err)
	}

	// Build event maps for O(1) lookups
	swapEvents := make(map[string]*indexer.Event)
	depositEvents := make(map[string]*indexer.Event)
	withdrawalEvents := make(map[string]*indexer.Event)

	for _, event := range events {
		// Events have nullable OrderID field
		if event.OrderID == nil {
			continue
		}
		orderID := *event.OrderID

		switch event.EventType {
		case rpc.EventTypeAsStr(rpc.EventTypeDexSwap):
			swapEvents[orderID] = event
		case rpc.EventTypeAsStr(rpc.EventTypeDexLiquidityDeposit):
			depositEvents[orderID] = event
		case rpc.EventTypeAsStr(rpc.EventTypeDexLiquidityWithdraw):
			withdrawalEvents[orderID] = event
		}
	}

	// Process orders from ALL committees
	orders := make([]*indexer.DexOrder, 0)
	// Process deposits from ALL committees
	deposits := make([]*indexer.DexDeposit, 0)
	// Process withdrawals from ALL committees
	withdrawals := make([]*indexer.DexWithdrawal, 0)

	// Process Dex event related against H-1 orders, deposits and withdrawals
	for _, currentBatchH1 := range currentBatchesH1 {
		for _, rpcOrder := range currentBatchH1.Orders {
			// Check if this order has a completion event
			if swapEvent, exists := swapEvents[rpcOrder.OrderID]; exists {
				order := &indexer.DexOrder{
					OrderID:         rpcOrder.OrderID,
					Height:          in.Height, // complete at the height of the event
					HeightTime:      in.BlockTime,
					Committee:       currentBatchH1.Committee, // Use committee from batch
					Address:         rpcOrder.Address,
					AmountForSale:   rpcOrder.AmountForSale,
					RequestedAmount: rpcOrder.RequestedAmount,
					State:           indexer.DexCompleteState,
					LockedHeight:    currentBatchH1.LockedHeight,
				}

				// Safely dereference pointer fields from event
				if swapEvent.Success != nil {
					order.Success = *swapEvent.Success
				}
				if swapEvent.SoldAmount != nil {
					order.SoldAmount = *swapEvent.SoldAmount
				}
				if swapEvent.BoughtAmount != nil {
					order.BoughtAmount = *swapEvent.BoughtAmount
				}
				if swapEvent.LocalOrigin != nil {
					order.LocalOrigin = *swapEvent.LocalOrigin
				}

				orders = append(orders, order)
			}
		}

		for _, rpcDeposit := range currentBatchH1.Deposits {
			// Check if this deposit has a completion event
			if depositEvent, exists := depositEvents[rpcDeposit.OrderID]; exists {
				deposit := &indexer.DexDeposit{
					OrderID:    rpcDeposit.OrderID,
					Height:     in.Height,
					HeightTime: in.BlockTime,
					Committee:  currentBatchH1.Committee, // Use committee from batch
					Address:    rpcDeposit.Address,
					Amount:     rpcDeposit.Amount,
					State:      indexer.DexCompleteState,
				}

				// Safely dereference pointer fields from event
				if depositEvent.LocalOrigin != nil {
					deposit.LocalOrigin = *depositEvent.LocalOrigin
				}
				// @TODO: PointsReceived should be calculated from pool state change
				// For now, we leave it as 0 - this will be improved when pool points tracking is added

				deposits = append(deposits, deposit)
			}
		}

		for _, rpcWithdrawal := range currentBatchH1.Withdrawals {
			// Check if this withdrawal has a completion event
			if withdrawalEvent, exists := withdrawalEvents[rpcWithdrawal.OrderID]; exists {
				withdrawal := &indexer.DexWithdrawal{
					OrderID:    rpcWithdrawal.OrderID,
					Height:     in.Height,
					HeightTime: in.BlockTime,
					Committee:  currentBatchH1.Committee, // Use committee from batch
					Address:    rpcWithdrawal.Address,
					Percent:    rpcWithdrawal.Percent,
					State:      indexer.DexCompleteState,
				}

				// Safely dereference pointer fields from event
				if withdrawalEvent.LocalAmount != nil {
					withdrawal.LocalAmount = *withdrawalEvent.LocalAmount
				}
				if withdrawalEvent.RemoteAmount != nil {
					withdrawal.RemoteAmount = *withdrawalEvent.RemoteAmount
				}
				// @TODO: PointsBurned should be calculated from pool state change
				// For now, we leave it as 0 - this will be improved when pool points tracking is added

				withdrawals = append(withdrawals, withdrawal)
			}
		}
	}

	// Process current batch orders, deposits and withdrawals
	for _, currentBatch := range currentBatches {
		for _, rpcOrder := range currentBatch.Orders {
			order := &indexer.DexOrder{
				OrderID:         rpcOrder.OrderID,
				Height:          in.Height,
				HeightTime:      in.BlockTime,
				Committee:       currentBatch.Committee, // Use committee from batch
				Address:         rpcOrder.Address,
				AmountForSale:   rpcOrder.AmountForSale,
				RequestedAmount: rpcOrder.RequestedAmount,
				State:           indexer.DexLockedState,
				LockedHeight:    currentBatch.LockedHeight,
			}

			orders = append(orders, order)
		}

		for _, rpcDeposit := range currentBatch.Deposits {
			deposit := &indexer.DexDeposit{
				OrderID:    rpcDeposit.OrderID,
				Height:     in.Height,
				HeightTime: in.BlockTime,
				Committee:  currentBatch.Committee, // Use committee from batch
				Address:    rpcDeposit.Address,
				Amount:     rpcDeposit.Amount,
				State:      indexer.DexLockedState,
			}

			deposits = append(deposits, deposit)
		}

		for _, rpcWithdrawal := range currentBatch.Withdrawals {
			withdrawal := &indexer.DexWithdrawal{
				OrderID:    rpcWithdrawal.OrderID,
				Height:     in.Height,
				HeightTime: in.BlockTime,
				Committee:  currentBatch.Committee, // Use committee from batch
				Address:    rpcWithdrawal.Address,
				Percent:    rpcWithdrawal.Percent,
				State:      indexer.DexLockedState,
			}

			withdrawals = append(withdrawals, withdrawal)
		}
	}

	// Process next batch orders (state=future)
	for _, nextBatch := range nextBatches {
		for _, rpcOrder := range nextBatch.Orders {
			order := &indexer.DexOrder{
				OrderID:         rpcOrder.OrderID,
				Height:          in.Height,
				HeightTime:      in.BlockTime,
				Committee:       nextBatch.Committee, // Use committee from batch
				Address:         rpcOrder.Address,
				AmountForSale:   rpcOrder.AmountForSale,
				RequestedAmount: rpcOrder.RequestedAmount,
				State:           indexer.DexPendingState,
				LockedHeight:    0, // Not locked yet
			}
			orders = append(orders, order)
		}

		for _, rpcDeposit := range nextBatch.Deposits {
			deposit := &indexer.DexDeposit{
				OrderID:    rpcDeposit.OrderID,
				Height:     in.Height,
				HeightTime: in.BlockTime,
				Committee:  nextBatch.Committee, // Use committee from batch
				Address:    rpcDeposit.Address,
				Amount:     rpcDeposit.Amount,
				State:      indexer.DexPendingState,
			}
			deposits = append(deposits, deposit)
		}

		for _, rpcWithdrawal := range nextBatch.Withdrawals {
			withdrawal := &indexer.DexWithdrawal{
				OrderID:    rpcWithdrawal.OrderID,
				Height:     in.Height,
				HeightTime: in.BlockTime,
				Committee:  nextBatch.Committee, // Use committee from batch
				Address:    rpcWithdrawal.Address,
				Percent:    rpcWithdrawal.Percent,
				State:      indexer.DexPendingState,
			}
			withdrawals = append(withdrawals, withdrawal)
		}
	}

	// Insert to staging tables
	if len(orders) > 0 {
		if err := chainDb.InsertDexOrdersStaging(ctx, orders); err != nil {
			return types.ActivityIndexDexBatchOutput{}, fmt.Errorf("insert dex orders staging: %w", err)
		}
	}

	if len(deposits) > 0 {
		if err := chainDb.InsertDexDepositsStaging(ctx, deposits); err != nil {
			return types.ActivityIndexDexBatchOutput{}, fmt.Errorf("insert dex deposits staging: %w", err)
		}
	}

	if len(withdrawals) > 0 {
		if err := chainDb.InsertDexWithdrawalsStaging(ctx, withdrawals); err != nil {
			return types.ActivityIndexDexBatchOutput{}, fmt.Errorf("insert dex withdrawals staging: %w", err)
		}
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	ac.Logger.Info("Indexed DEX batches (all committees)",
		zap.Uint64("chainId", ac.ChainID),
		zap.Uint64("height", in.Height),
		zap.Int("committees", len(currentBatches)+len(nextBatches)),
		zap.Int("orders", len(orders)),
		zap.Int("deposits", len(deposits)),
		zap.Int("withdrawals", len(withdrawals)),
		zap.Float64("durationMs", durationMs))

	return types.ActivityIndexDexBatchOutput{
		NumOrders:      uint32(len(orders)),
		NumDeposits:    uint32(len(deposits)),
		NumWithdrawals: uint32(len(withdrawals)),
		// @TODO: Separate them into success/fail future/lock/completed - but leave the "total" for the block visibility
		DurationMs: durationMs,
	}, nil
}

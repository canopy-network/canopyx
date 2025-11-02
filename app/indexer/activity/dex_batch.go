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
// uses height H (when the event occurred) but entity data comes from RPC(H-1).
//
// Performance:
// - Parallel RPC fetching reduces latency by ~50% (2 concurrent requests)
// - Events are queried from staging DB (already indexed by IndexEvents activity)
// - Only changed entities are stored (snapshot-on-change pattern)
func (c *Context) IndexDexBatch(ctx context.Context, in types.IndexDexBatchInput) (types.IndexDexBatchOutput, error) {
	start := time.Now()

	// Get chain configuration
	ch, err := c.AdminDB.GetChain(ctx, in.ChainID)
	if err != nil {
		return types.IndexDexBatchOutput{}, err
	}

	// Acquire chain database
	chainDb, err := c.NewChainDb(ctx, in.ChainID)
	if err != nil {
		return types.IndexDexBatchOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", err)
	}

	// Create RPC client
	cli := c.rpcClient(ch.RPCEndpoints)

	// Parallel RPC fetch: DexBatch(H) and NextDexBatch(H)
	var (
		currentBatch *rpc.RpcDexBatch
		nextBatch    *rpc.RpcDexBatch
		currentErr   error
		nextErr      error
		wg           sync.WaitGroup
	)

	wg.Add(2)

	// Worker 1: Fetch current batch (locked orders)
	go func() {
		defer wg.Done()
		currentBatch, currentErr = cli.DexBatchByHeight(ctx, in.Height, in.Committee)
	}()

	// Worker 2: Fetch next batch (future orders)
	go func() {
		defer wg.Done()
		nextBatch, nextErr = cli.NextDexBatchByHeight(ctx, in.Height, in.Committee)
	}()

	// Wait for both workers
	wg.Wait()

	// Check for errors (nil batch is acceptable - means no batch at this height)
	if currentErr != nil {
		return types.IndexDexBatchOutput{}, fmt.Errorf("fetch dex batch at height %d: %w", in.Height, currentErr)
	}
	if nextErr != nil {
		return types.IndexDexBatchOutput{}, fmt.Errorf("fetch next dex batch at height %d: %w", in.Height, nextErr)
	}

	// Query events from staging table
	events, err := chainDb.GetEventsByTypeAndHeight(ctx, in.Height,
		"EventDexSwap",
		"EventDexLiquidityDeposit",
		"EventDexLiquidityWithdrawal",
	)
	if err != nil {
		return types.IndexDexBatchOutput{}, fmt.Errorf("query events at height %d: %w", in.Height, err)
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
		case "EventDexSwap":
			swapEvents[orderID] = event
		case "EventDexLiquidityDeposit":
			depositEvents[orderID] = event
		case "EventDexLiquidityWithdrawal":
			withdrawalEvents[orderID] = event
		}
	}

	// Process orders
	orders := make([]*indexer.DexOrder, 0)

	// Process current batch orders (state=locked or state=complete if event exists)
	if currentBatch != nil {
		for _, rpcOrder := range currentBatch.Orders {
			order := &indexer.DexOrder{
				OrderID:         rpcOrder.OrderID,
				Height:          in.Height,
				HeightTime:      in.BlockTime,
				Committee:       in.Committee,
				Address:         rpcOrder.Address,
				AmountForSale:   rpcOrder.AmountForSale,
				RequestedAmount: rpcOrder.RequestedAmount,
				State:           "locked",
				LockedHeight:    currentBatch.LockedHeight,
			}

			// Check if this order has a completion event
			if swapEvent, exists := swapEvents[rpcOrder.OrderID]; exists {
				order.State = "complete"
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
			}

			orders = append(orders, order)
		}
	}

	// Process next batch orders (state=future)
	if nextBatch != nil {
		for _, rpcOrder := range nextBatch.Orders {
			order := &indexer.DexOrder{
				OrderID:         rpcOrder.OrderID,
				Height:          in.Height,
				HeightTime:      in.BlockTime,
				Committee:       in.Committee,
				Address:         rpcOrder.Address,
				AmountForSale:   rpcOrder.AmountForSale,
				RequestedAmount: rpcOrder.RequestedAmount,
				State:           "future",
				LockedHeight:    0, // Not locked yet
			}
			orders = append(orders, order)
		}
	}

	// Process deposits
	deposits := make([]*indexer.DexDeposit, 0)

	// Process current batch deposits
	if currentBatch != nil {
		for _, rpcDeposit := range currentBatch.Deposits {
			deposit := &indexer.DexDeposit{
				OrderID:    rpcDeposit.OrderID,
				Height:     in.Height,
				HeightTime: in.BlockTime,
				Committee:  in.Committee,
				Address:    rpcDeposit.Address,
				Amount:     rpcDeposit.Amount,
				State:      "pending",
			}

			// Check if this deposit has a completion event
			if depositEvent, exists := depositEvents[rpcDeposit.OrderID]; exists {
				deposit.State = "complete"
				// Safely dereference pointer fields from event
				if depositEvent.LocalOrigin != nil {
					deposit.LocalOrigin = *depositEvent.LocalOrigin
				}
				// Note: PointsReceived should be calculated from pool state change
				// For now, we leave it as 0 - this will be improved when pool points tracking is added
			}

			deposits = append(deposits, deposit)
		}
	}

	// Process next batch deposits
	if nextBatch != nil {
		for _, rpcDeposit := range nextBatch.Deposits {
			deposit := &indexer.DexDeposit{
				OrderID:    rpcDeposit.OrderID,
				Height:     in.Height,
				HeightTime: in.BlockTime,
				Committee:  in.Committee,
				Address:    rpcDeposit.Address,
				Amount:     rpcDeposit.Amount,
				State:      "pending",
			}
			deposits = append(deposits, deposit)
		}
	}

	// Process withdrawals
	withdrawals := make([]*indexer.DexWithdrawal, 0)

	// Process current batch withdrawals
	if currentBatch != nil {
		for _, rpcWithdrawal := range currentBatch.Withdrawals {
			withdrawal := &indexer.DexWithdrawal{
				OrderID:    rpcWithdrawal.OrderID,
				Height:     in.Height,
				HeightTime: in.BlockTime,
				Committee:  in.Committee,
				Address:    rpcWithdrawal.Address,
				Percent:    rpcWithdrawal.Percent,
				State:      "pending",
			}

			// Check if this withdrawal has a completion event
			if withdrawalEvent, exists := withdrawalEvents[rpcWithdrawal.OrderID]; exists {
				withdrawal.State = "complete"
				// Safely dereference pointer fields from event
				if withdrawalEvent.LocalAmount != nil {
					withdrawal.LocalAmount = *withdrawalEvent.LocalAmount
				}
				if withdrawalEvent.RemoteAmount != nil {
					withdrawal.RemoteAmount = *withdrawalEvent.RemoteAmount
				}
				// Note: PointsBurned should be calculated from pool state change
				// For now, we leave it as 0 - this will be improved when pool points tracking is added
			}

			withdrawals = append(withdrawals, withdrawal)
		}
	}

	// Process next batch withdrawals
	if nextBatch != nil {
		for _, rpcWithdrawal := range nextBatch.Withdrawals {
			withdrawal := &indexer.DexWithdrawal{
				OrderID:    rpcWithdrawal.OrderID,
				Height:     in.Height,
				HeightTime: in.BlockTime,
				Committee:  in.Committee,
				Address:    rpcWithdrawal.Address,
				Percent:    rpcWithdrawal.Percent,
				State:      "pending",
			}
			withdrawals = append(withdrawals, withdrawal)
		}
	}

	// Insert to staging tables
	if len(orders) > 0 {
		if err := chainDb.InsertDexOrdersStaging(ctx, orders); err != nil {
			return types.IndexDexBatchOutput{}, fmt.Errorf("insert dex orders staging: %w", err)
		}
	}

	if len(deposits) > 0 {
		if err := chainDb.InsertDexDepositsStaging(ctx, deposits); err != nil {
			return types.IndexDexBatchOutput{}, fmt.Errorf("insert dex deposits staging: %w", err)
		}
	}

	if len(withdrawals) > 0 {
		if err := chainDb.InsertDexWithdrawalsStaging(ctx, withdrawals); err != nil {
			return types.IndexDexBatchOutput{}, fmt.Errorf("insert dex withdrawals staging: %w", err)
		}
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	c.Logger.Info("Indexed DEX batch",
		zap.Uint64("chainId", in.ChainID),
		zap.Uint64("height", in.Height),
		zap.Uint64("committee", in.Committee),
		zap.Int("orders", len(orders)),
		zap.Int("deposits", len(deposits)),
		zap.Int("withdrawals", len(withdrawals)),
		zap.Float64("durationMs", durationMs))

	return types.IndexDexBatchOutput{
		NumOrders:      uint32(len(orders)),
		NumDeposits:    uint32(len(deposits)),
		NumWithdrawals: uint32(len(withdrawals)),
		DurationMs:     durationMs,
	}, nil
}

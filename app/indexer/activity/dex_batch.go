package activity

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/canopy-network/canopy/lib"
	"github.com/canopy-network/canopyx/app/indexer/types"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
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

	// Get RPC client with height-aware endpoint selection
	cli, err := ac.rpcClientForHeight(ctx, in.Height)
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
		currentBatchesH1 []*lib.DexBatch
		currentBatches   []*lib.DexBatch
		nextBatches      []*lib.DexBatch
		currentH1Err     error
		currentErr       error
		nextErr          error
	)

	// Get a subgroup from the shared worker pool for parallel RPC fetching
	pool := ac.WorkerPool(3)
	group := pool.NewGroupContext(ctx)
	groupCtx := group.Context()

	// Worker 1: Fetch ALL current batches (locked orders across all committees)
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		currentBatches, currentErr = cli.AllDexBatchesByHeight(groupCtx, in.Height)
	})

	// Worker 2: Fetch ALL next batches (future orders across all committees)
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		nextBatches, nextErr = cli.AllNextDexBatchesByHeight(groupCtx, in.Height)
	})

	// Worker 3: Fetch current batches at H-1 for event processing
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		if in.Height <= 1 {
			currentBatchesH1 = make([]*lib.DexBatch, 0)
			return
		}
		currentBatchesH1, currentH1Err = cli.AllDexBatchesByHeight(groupCtx, in.Height-1)
	})

	// Wait for all workers to complete
	if err := group.Wait(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, pond.ErrGroupStopped) {
		ac.Logger.Warn("parallel RPC fetch encountered error",
			zap.Uint64("chainId", ac.ChainID),
			zap.Uint64("height", in.Height),
			zap.Error(err),
		)
	}

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
		string(lib.EventTypeDexSwap),
		string(lib.EventTypeDexLiquidityDeposit),
		string(lib.EventTypeDexLiquidityWithdraw),
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
		case string(lib.EventTypeDexSwap):
			swapEvents[orderID] = event
		case string(lib.EventTypeDexLiquidityDeposit):
			depositEvents[orderID] = event
		case string(lib.EventTypeDexLiquidityWithdraw):
			withdrawalEvents[orderID] = event
		}
	}

	// Diagnostic: Log withdrawal event details for debugging OrderID matching
	if len(withdrawalEvents) > 0 {
		ac.Logger.Info("Loaded DEX withdrawal events",
			zap.Uint64("height", in.Height),
			zap.Int("count", len(withdrawalEvents)))
		for orderID := range withdrawalEvents {
			ac.Logger.Debug("Event OrderID",
				zap.String("orderId", orderID),
				zap.Int("length", len(orderID)))
		}
	}

	// Process orders from ALL committees
	orders := make([]*indexer.DexOrder, 0)
	// Process deposits from ALL committees
	deposits := make([]*indexer.DexDeposit, 0)
	// Process withdrawals from ALL committees
	withdrawals := make([]*indexer.DexWithdrawal, 0)

	// Initialize state counters for orders
	var numOrdersFuture, numOrdersLocked, numOrdersComplete uint32
	var numOrdersSuccess, numOrdersFailed uint32
	// Initialize state counters for deposits
	var numDepositsPending, numDepositsLocked, numDepositsComplete uint32
	// Initialize state counters for withdrawals
	var numWithdrawalsPending, numWithdrawalsLocked, numWithdrawalsComplete uint32

	// Process Dex event related against H-1 orders, deposits and withdrawals
	for _, currentBatchH1 := range currentBatchesH1 {
		for _, rpcOrder := range currentBatchH1.Orders {
			// Convert protobuf []byte fields to hex strings for database storage
			orderID := hex.EncodeToString(rpcOrder.OrderId)
			address := hex.EncodeToString(rpcOrder.Address)

			// Check if this order has a completion event
			if swapEvent, exists := swapEvents[orderID]; exists {
				order := &indexer.DexOrder{
					OrderID:         orderID,
					Height:          in.Height, // complete at the height of the event
					HeightTime:      in.BlockTime,
					Committee:       currentBatchH1.Committee, // Use committee from batch
					Address:         address,
					AmountForSale:   rpcOrder.AmountForSale,
					RequestedAmount: rpcOrder.RequestedAmount,
					State:           indexer.DexCompleteState,
					LockedHeight:    currentBatchH1.LockedHeight,
				}

				// Safely dereference pointer fields from event
				if swapEvent.Success != nil {
					order.Success = *swapEvent.Success
					// Count success/failed orders
					if order.Success {
						numOrdersSuccess++
					} else {
						numOrdersFailed++
					}
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

				numOrdersComplete++
				orders = append(orders, order)
			}
		}

		for _, rpcDeposit := range currentBatchH1.Deposits {
			// Convert protobuf []byte fields to hex strings for database storage
			orderID := hex.EncodeToString(rpcDeposit.OrderId)
			address := hex.EncodeToString(rpcDeposit.Address)

			// Check if this deposit has a completion event
			if depositEvent, exists := depositEvents[orderID]; exists {
				deposit := &indexer.DexDeposit{
					OrderID:    orderID,
					Height:     in.Height,
					HeightTime: in.BlockTime,
					Committee:  currentBatchH1.Committee, // Use committee from batch
					Address:    address,
					Amount:     rpcDeposit.Amount,
					State:      indexer.DexCompleteState,
				}

				// Safely dereference pointer fields from event
				if depositEvent.LocalOrigin != nil {
					deposit.LocalOrigin = *depositEvent.LocalOrigin
				}
				if depositEvent.PointsReceived != nil {
					deposit.PointsReceived = *depositEvent.PointsReceived
				}

				numDepositsComplete++
				deposits = append(deposits, deposit)
			}
		}

		for _, rpcWithdrawal := range currentBatchH1.Withdrawals {
			// Convert protobuf []byte fields to hex strings for database storage
			orderID := hex.EncodeToString(rpcWithdrawal.OrderId)
			address := hex.EncodeToString(rpcWithdrawal.Address)

			// Diagnostic: Log withdrawal matching attempt
			ac.Logger.Debug("Checking H-1 batch withdrawal for completion",
				zap.String("orderID_batch", orderID),
				zap.Int("length", len(orderID)),
				zap.Int("available_events", len(withdrawalEvents)))

			// Check if this withdrawal has a completion event
			if withdrawalEvent, exists := withdrawalEvents[orderID]; exists {
				ac.Logger.Info("MATCHED: Withdrawal completed",
					zap.String("orderID", orderID))
				withdrawal := &indexer.DexWithdrawal{
					OrderID:    orderID,
					Height:     in.Height,
					HeightTime: in.BlockTime,
					Committee:  currentBatchH1.Committee, // Use committee from batch
					Address:    address,
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
				if withdrawalEvent.PointsBurned != nil {
					withdrawal.PointsBurned = *withdrawalEvent.PointsBurned
				}

				numWithdrawalsComplete++
				withdrawals = append(withdrawals, withdrawal)
			} else {
				// Diagnostic: Log when withdrawal event not found
				ac.Logger.Warn("NO MATCH: Withdrawal event not found",
					zap.String("orderID_batch", orderID),
					zap.Int("available_events", len(withdrawalEvents)))
			}
		}
	}

	// Process current batch orders, deposits and withdrawals
	for _, currentBatch := range currentBatches {
		for _, rpcOrder := range currentBatch.Orders {
			// Convert protobuf []byte fields to hex strings for database storage
			orderID := hex.EncodeToString(rpcOrder.OrderId)
			address := hex.EncodeToString(rpcOrder.Address)

			order := &indexer.DexOrder{
				OrderID:         orderID,
				Height:          in.Height,
				HeightTime:      in.BlockTime,
				Committee:       currentBatch.Committee, // Use committee from batch
				Address:         address,
				AmountForSale:   rpcOrder.AmountForSale,
				RequestedAmount: rpcOrder.RequestedAmount,
				State:           indexer.DexLockedState,
				LockedHeight:    currentBatch.LockedHeight,
			}

			numOrdersLocked++
			orders = append(orders, order)
		}

		for _, rpcDeposit := range currentBatch.Deposits {
			// Convert protobuf []byte fields to hex strings for database storage
			orderID := hex.EncodeToString(rpcDeposit.OrderId)
			address := hex.EncodeToString(rpcDeposit.Address)

			deposit := &indexer.DexDeposit{
				OrderID:    orderID,
				Height:     in.Height,
				HeightTime: in.BlockTime,
				Committee:  currentBatch.Committee, // Use committee from batch
				Address:    address,
				Amount:     rpcDeposit.Amount,
				State:      indexer.DexLockedState,
			}

			numDepositsLocked++
			deposits = append(deposits, deposit)
		}

		for _, rpcWithdrawal := range currentBatch.Withdrawals {
			// Convert protobuf []byte fields to hex strings for database storage
			orderID := hex.EncodeToString(rpcWithdrawal.OrderId)
			address := hex.EncodeToString(rpcWithdrawal.Address)

			withdrawal := &indexer.DexWithdrawal{
				OrderID:    orderID,
				Height:     in.Height,
				HeightTime: in.BlockTime,
				Committee:  currentBatch.Committee, // Use committee from batch
				Address:    address,
				Percent:    rpcWithdrawal.Percent,
				State:      indexer.DexLockedState,
			}

			numWithdrawalsLocked++
			withdrawals = append(withdrawals, withdrawal)
		}
	}

	// Process next batch orders (state=future)
	for _, nextBatch := range nextBatches {
		for _, rpcOrder := range nextBatch.Orders {
			// Convert protobuf []byte fields to hex strings for database storage
			orderID := hex.EncodeToString(rpcOrder.OrderId)
			address := hex.EncodeToString(rpcOrder.Address)

			order := &indexer.DexOrder{
				OrderID:         orderID,
				Height:          in.Height,
				HeightTime:      in.BlockTime,
				Committee:       nextBatch.Committee, // Use committee from batch
				Address:         address,
				AmountForSale:   rpcOrder.AmountForSale,
				RequestedAmount: rpcOrder.RequestedAmount,
				State:           indexer.DexPendingState,
				LockedHeight:    0, // Not locked yet
			}
			numOrdersFuture++
			orders = append(orders, order)
		}

		for _, rpcDeposit := range nextBatch.Deposits {
			// Convert protobuf []byte fields to hex strings for database storage
			orderID := hex.EncodeToString(rpcDeposit.OrderId)
			address := hex.EncodeToString(rpcDeposit.Address)

			deposit := &indexer.DexDeposit{
				OrderID:    orderID,
				Height:     in.Height,
				HeightTime: in.BlockTime,
				Committee:  nextBatch.Committee, // Use committee from batch
				Address:    address,
				Amount:     rpcDeposit.Amount,
				State:      indexer.DexPendingState,
			}
			numDepositsPending++
			deposits = append(deposits, deposit)
		}

		for _, rpcWithdrawal := range nextBatch.Withdrawals {
			// Convert protobuf []byte fields to hex strings for database storage
			orderID := hex.EncodeToString(rpcWithdrawal.OrderId)
			address := hex.EncodeToString(rpcWithdrawal.Address)

			withdrawal := &indexer.DexWithdrawal{
				OrderID:    orderID,
				Height:     in.Height,
				HeightTime: in.BlockTime,
				Committee:  nextBatch.Committee, // Use committee from batch
				Address:    address,
				Percent:    rpcWithdrawal.Percent,
				State:      indexer.DexPendingState,
			}
			numWithdrawalsPending++
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
		NumOrders:              uint32(len(orders)),
		NumOrdersFuture:        numOrdersFuture,
		NumOrdersLocked:        numOrdersLocked,
		NumOrdersComplete:      numOrdersComplete,
		NumOrdersSuccess:       numOrdersSuccess,
		NumOrdersFailed:        numOrdersFailed,
		NumDeposits:            uint32(len(deposits)),
		NumDepositsPending:     numDepositsPending,
		NumDepositsLocked:      numDepositsLocked,
		NumDepositsComplete:    numDepositsComplete,
		NumWithdrawals:         uint32(len(withdrawals)),
		NumWithdrawalsPending:  numWithdrawalsPending,
		NumWithdrawalsLocked:   numWithdrawalsLocked,
		NumWithdrawalsComplete: numWithdrawalsComplete,
		DurationMs:             durationMs,
	}, nil
}

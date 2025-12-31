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
	chainstore "github.com/canopy-network/canopyx/pkg/db/chain"
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
// - Parallel RPC fetching reduces latency by ~50% (4 concurrent requests)
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

	// Fetch batch data from RPC (H and H-1) in parallel
	currentBatchesH1, nextBatchesH1, currentBatches, nextBatches, err := ac.fetchBatchData(ctx, cli, in.Height)
	if err != nil {
		return types.ActivityIndexDexBatchOutput{}, err
	}

	// Query and build event maps
	swapEvents, depositEvents, withdrawalEvents, err := ac.buildEventMaps(ctx, chainDb, in.Height)
	if err != nil {
		return types.ActivityIndexDexBatchOutput{}, err
	}

	// Build H-1 comparison maps for change detection
	h1Maps := ac.buildH1ComparisonMaps(currentBatchesH1, nextBatchesH1)

	// Initialize collections and counters
	orders := make([]*indexer.DexOrder, 0)
	deposits := make([]*indexer.DexDeposit, 0)
	withdrawals := make([]*indexer.DexWithdrawal, 0)
	counters := &dexBatchCounters{}

	// Process complete items (from H-1 batch with completion events)
	ac.processCompleteItems(
		currentBatchesH1, swapEvents, depositEvents, withdrawalEvents,
		in.Height, in.BlockTime, &orders, &deposits, &withdrawals, counters,
	)

	// Process locked items (from current batch with change detection)
	ac.processLockedItems(
		currentBatches, h1Maps, in.Height, in.BlockTime,
		&orders, &deposits, &withdrawals, counters,
	)

	// Process pending items (from next batch with change detection)
	ac.processPendingItems(
		nextBatches, h1Maps, in.Height, in.BlockTime,
		&orders, &deposits, &withdrawals, counters,
	)

	// Insert to staging tables
	if err := ac.insertDexBatchData(ctx, chainDb, orders, deposits, withdrawals); err != nil {
		return types.ActivityIndexDexBatchOutput{}, err
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
		NumOrdersFuture:        counters.NumOrdersFuture,
		NumOrdersLocked:        counters.NumOrdersLocked,
		NumOrdersComplete:      counters.NumOrdersComplete,
		NumOrdersSuccess:       counters.NumOrdersSuccess,
		NumOrdersFailed:        counters.NumOrdersFailed,
		NumDeposits:            uint32(len(deposits)),
		NumDepositsPending:     counters.NumDepositsPending,
		NumDepositsLocked:      counters.NumDepositsLocked,
		NumDepositsComplete:    counters.NumDepositsComplete,
		NumWithdrawals:         uint32(len(withdrawals)),
		NumWithdrawalsPending:  counters.NumWithdrawalsPending,
		NumWithdrawalsLocked:   counters.NumWithdrawalsLocked,
		NumWithdrawalsComplete: counters.NumWithdrawalsComplete,
		DurationMs:             durationMs,
	}, nil
}

// dexBatchCounters holds counters for different states and outcomes of DEX batch items
type dexBatchCounters struct {
	NumOrdersFuture             uint32
	NumOrdersLocked             uint32
	NumOrdersComplete           uint32
	NumOrdersSuccess            uint32
	NumOrdersFailed             uint32
	NumDepositsPending          uint32
	NumDepositsLocked           uint32
	NumDepositsComplete         uint32
	NumWithdrawalsPending       uint32
	NumWithdrawalsLocked        uint32
	NumWithdrawalsComplete      uint32
	OrdersLockedChanged         int
	OrdersLockedUnchanged       int
	OrdersPendingChanged        int
	OrdersPendingUnchanged      int
	DepositsLockedChanged       int
	DepositsLockedUnchanged     int
	DepositsPendingChanged      int
	DepositsPendingUnchanged    int
	WithdrawalsLockedChanged    int
	WithdrawalsLockedUnchanged  int
	WithdrawalsPendingChanged   int
	WithdrawalsPendingUnchanged int
}

// h1ComparisonMaps holds H-1 data organized by orderID for change detection
type h1ComparisonMaps struct {
	OrdersLocked       map[string]*lib.DexLimitOrder
	OrdersPending      map[string]*lib.DexLimitOrder
	DepositsLocked     map[string]*lib.DexLiquidityDeposit
	DepositsPending    map[string]*lib.DexLiquidityDeposit
	WithdrawalsLocked  map[string]*lib.DexLiquidityWithdraw
	WithdrawalsPending map[string]*lib.DexLiquidityWithdraw
}

// fetchBatchData fetches all batch data from RPC in parallel (H and H-1)
func (ac *Context) fetchBatchData(ctx context.Context, cli rpc.Client, height uint64) (
	currentBatchesH1, nextBatchesH1, currentBatches, nextBatches []*lib.DexBatch, err error,
) {
	var (
		currentH1Err error
		nextH1Err    error
		currentErr   error
		nextErr      error
	)

	// Get a subgroup from the shared worker pool for parallel RPC fetching
	pool := ac.WorkerPool(4)
	group := pool.NewGroupContext(ctx)
	groupCtx := group.Context()

	// Worker 1: Fetch ALL current batches (locked orders across all committees)
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		currentBatches, currentErr = cli.AllDexBatchesByHeight(groupCtx, height)
	})

	// Worker 2: Fetch ALL next batches (future orders across all committees)
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		nextBatches, nextErr = cli.AllNextDexBatchesByHeight(groupCtx, height)
	})

	// Worker 3: Fetch current batches at H-1 for event processing and change detection
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		if height <= 1 {
			currentBatchesH1 = make([]*lib.DexBatch, 0)
			return
		}
		currentBatchesH1, currentH1Err = cli.AllDexBatchesByHeight(groupCtx, height-1)
	})

	// Worker 4: Fetch next batches at H-1 for change detection
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		if height <= 1 {
			nextBatchesH1 = make([]*lib.DexBatch, 0)
			return
		}
		nextBatchesH1, nextH1Err = cli.AllNextDexBatchesByHeight(groupCtx, height-1)
	})

	// Wait for all workers to complete
	if waitErr := group.Wait(); waitErr != nil && !errors.Is(waitErr, context.Canceled) && !errors.Is(waitErr, pond.ErrGroupStopped) {
		ac.Logger.Warn("parallel RPC fetch encountered error",
			zap.Uint64("chainId", ac.ChainID),
			zap.Uint64("height", height),
			zap.Error(waitErr),
		)
	}

	// Check for errors (empty batches are acceptable - means no batches at this height)
	if currentErr != nil {
		return nil, nil, nil, nil, fmt.Errorf("fetch all dex batches at height %d: %w", height, currentErr)
	}
	if nextErr != nil {
		return nil, nil, nil, nil, fmt.Errorf("fetch all next dex batches at height %d: %w", height, nextErr)
	}
	if currentH1Err != nil {
		return nil, nil, nil, nil, fmt.Errorf("fetch all dex batches at height %d: %w", height-1, currentH1Err)
	}
	if nextH1Err != nil {
		return nil, nil, nil, nil, fmt.Errorf("fetch all next dex batches at height %d: %w", height-1, nextH1Err)
	}

	return currentBatchesH1, nextBatchesH1, currentBatches, nextBatches, nil
}

// buildEventMaps queries events and builds lookup maps by orderID
func (ac *Context) buildEventMaps(ctx context.Context, chainDb chainstore.Store, height uint64) (
	swapEvents, depositEvents, withdrawalEvents map[string]*indexer.Event, err error,
) {
	// Query events from the staging table
	events, err := chainDb.GetEventsByTypeAndHeight(
		ctx, height, true,
		string(lib.EventTypeDexSwap),
		string(lib.EventTypeDexLiquidityDeposit),
		string(lib.EventTypeDexLiquidityWithdraw),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("query events at height %d: %w", height, err)
	}

	// Build event maps for O(1) lookups
	swapEvents = make(map[string]*indexer.Event)
	depositEvents = make(map[string]*indexer.Event)
	withdrawalEvents = make(map[string]*indexer.Event)

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

	// Diagnostic logging for withdrawal events
	if len(withdrawalEvents) > 0 {
		ac.Logger.Info("Loaded DEX withdrawal events",
			zap.Uint64("height", height),
			zap.Int("count", len(withdrawalEvents)))
	}

	return swapEvents, depositEvents, withdrawalEvents, nil
}

// buildH1ComparisonMaps builds H-1 comparison maps for change detection
func (ac *Context) buildH1ComparisonMaps(currentBatchesH1, nextBatchesH1 []*lib.DexBatch) *h1ComparisonMaps {
	maps := &h1ComparisonMaps{
		OrdersLocked:       make(map[string]*lib.DexLimitOrder),
		OrdersPending:      make(map[string]*lib.DexLimitOrder),
		DepositsLocked:     make(map[string]*lib.DexLiquidityDeposit),
		DepositsPending:    make(map[string]*lib.DexLiquidityDeposit),
		WithdrawalsLocked:  make(map[string]*lib.DexLiquidityWithdraw),
		WithdrawalsPending: make(map[string]*lib.DexLiquidityWithdraw),
	}

	// Build maps from currentBatchesH1 (locked items at H-1)
	for _, batch := range currentBatchesH1 {
		for _, order := range batch.Orders {
			orderID := hex.EncodeToString(order.OrderId)
			maps.OrdersLocked[orderID] = order
		}
		for _, deposit := range batch.Deposits {
			orderID := hex.EncodeToString(deposit.OrderId)
			maps.DepositsLocked[orderID] = deposit
		}
		for _, withdrawal := range batch.Withdrawals {
			orderID := hex.EncodeToString(withdrawal.OrderId)
			maps.WithdrawalsLocked[orderID] = withdrawal
		}
	}

	// Build maps from nextBatchesH1 (pending items at H-1)
	for _, batch := range nextBatchesH1 {
		for _, order := range batch.Orders {
			orderID := hex.EncodeToString(order.OrderId)
			maps.OrdersPending[orderID] = order
		}
		for _, deposit := range batch.Deposits {
			orderID := hex.EncodeToString(deposit.OrderId)
			maps.DepositsPending[orderID] = deposit
		}
		for _, withdrawal := range batch.Withdrawals {
			orderID := hex.EncodeToString(withdrawal.OrderId)
			maps.WithdrawalsPending[orderID] = withdrawal
		}
	}

	return maps
}

// processCompleteItems processes items from H-1 batch that have completion events
func (ac *Context) processCompleteItems(
	currentBatchesH1 []*lib.DexBatch,
	swapEvents, depositEvents, withdrawalEvents map[string]*indexer.Event,
	height uint64, blockTime time.Time,
	orders *[]*indexer.DexOrder,
	deposits *[]*indexer.DexDeposit,
	withdrawals *[]*indexer.DexWithdrawal,
	counters *dexBatchCounters,
) {
	for _, currentBatchH1 := range currentBatchesH1 {
		// Process complete orders
		for _, rpcOrder := range currentBatchH1.Orders {
			orderID := hex.EncodeToString(rpcOrder.OrderId)
			address := hex.EncodeToString(rpcOrder.Address)

			if swapEvent, exists := swapEvents[orderID]; exists {
				order := &indexer.DexOrder{
					OrderID:         orderID,
					Height:          height,
					HeightTime:      blockTime,
					Committee:       currentBatchH1.Committee,
					Address:         address,
					AmountForSale:   rpcOrder.AmountForSale,
					RequestedAmount: rpcOrder.RequestedAmount,
					State:           indexer.DexCompleteState,
					LockedHeight:    currentBatchH1.LockedHeight,
				}

				if swapEvent.Success != nil {
					order.Success = *swapEvent.Success
					if order.Success {
						counters.NumOrdersSuccess++
					} else {
						counters.NumOrdersFailed++
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

				counters.NumOrdersComplete++
				*orders = append(*orders, order)
			}
		}

		// Process complete deposits
		for _, rpcDeposit := range currentBatchH1.Deposits {
			orderID := hex.EncodeToString(rpcDeposit.OrderId)
			address := hex.EncodeToString(rpcDeposit.Address)

			if depositEvent, exists := depositEvents[orderID]; exists {
				deposit := &indexer.DexDeposit{
					OrderID:    orderID,
					Height:     height,
					HeightTime: blockTime,
					Committee:  currentBatchH1.Committee,
					Address:    address,
					Amount:     rpcDeposit.Amount,
					State:      indexer.DexCompleteState,
				}

				if depositEvent.LocalOrigin != nil {
					deposit.LocalOrigin = *depositEvent.LocalOrigin
				}
				if depositEvent.PointsReceived != nil {
					deposit.PointsReceived = *depositEvent.PointsReceived
				}

				counters.NumDepositsComplete++
				*deposits = append(*deposits, deposit)
			}
		}

		// Process complete withdrawals
		for _, rpcWithdrawal := range currentBatchH1.Withdrawals {
			orderID := hex.EncodeToString(rpcWithdrawal.OrderId)
			address := hex.EncodeToString(rpcWithdrawal.Address)

			ac.Logger.Debug("Checking H-1 batch withdrawal for completion",
				zap.String("orderID_batch", orderID),
				zap.Int("length", len(orderID)),
				zap.Int("available_events", len(withdrawalEvents)))

			if withdrawalEvent, exists := withdrawalEvents[orderID]; exists {
				ac.Logger.Info("MATCHED: Withdrawal completed",
					zap.String("orderID", orderID))
				withdrawal := &indexer.DexWithdrawal{
					OrderID:    orderID,
					Height:     height,
					HeightTime: blockTime,
					Committee:  currentBatchH1.Committee,
					Address:    address,
					Percent:    rpcWithdrawal.Percent,
					State:      indexer.DexCompleteState,
				}

				if withdrawalEvent.LocalAmount != nil {
					withdrawal.LocalAmount = *withdrawalEvent.LocalAmount
				}
				if withdrawalEvent.RemoteAmount != nil {
					withdrawal.RemoteAmount = *withdrawalEvent.RemoteAmount
				}
				if withdrawalEvent.PointsBurned != nil {
					withdrawal.PointsBurned = *withdrawalEvent.PointsBurned
				}

				counters.NumWithdrawalsComplete++
				*withdrawals = append(*withdrawals, withdrawal)
			} else {
				ac.Logger.Warn("NO MATCH: Withdrawal event not found",
					zap.String("orderID_batch", orderID),
					zap.Int("available_events", len(withdrawalEvents)))
			}
		}
	}
}

// processLockedItems processes locked items from current batch with change detection
func (ac *Context) processLockedItems(
	currentBatches []*lib.DexBatch,
	h1Maps *h1ComparisonMaps,
	height uint64, blockTime time.Time,
	orders *[]*indexer.DexOrder,
	deposits *[]*indexer.DexDeposit,
	withdrawals *[]*indexer.DexWithdrawal,
	counters *dexBatchCounters,
) {
	for _, currentBatch := range currentBatches {
		// Process locked orders
		for _, rpcOrder := range currentBatch.Orders {
			orderID := hex.EncodeToString(rpcOrder.OrderId)
			address := hex.EncodeToString(rpcOrder.Address)

			orderChanged := true
			if orderH1, existsLocked := h1Maps.OrdersLocked[orderID]; existsLocked {
				if orderH1.AmountForSale == rpcOrder.AmountForSale &&
					orderH1.RequestedAmount == rpcOrder.RequestedAmount &&
					hex.EncodeToString(orderH1.Address) == address {
					orderChanged = false
					counters.OrdersLockedUnchanged++
				}
			} else if _, existsPending := h1Maps.OrdersPending[orderID]; existsPending {
				orderChanged = true // State transition pending → locked
			}

			if orderChanged {
				order := &indexer.DexOrder{
					OrderID:         orderID,
					Height:          height,
					HeightTime:      blockTime,
					Committee:       currentBatch.Committee,
					Address:         address,
					AmountForSale:   rpcOrder.AmountForSale,
					RequestedAmount: rpcOrder.RequestedAmount,
					State:           indexer.DexLockedState,
					LockedHeight:    currentBatch.LockedHeight,
				}
				counters.NumOrdersLocked++
				counters.OrdersLockedChanged++
				*orders = append(*orders, order)
			}
		}

		// Process locked deposits
		for _, rpcDeposit := range currentBatch.Deposits {
			orderID := hex.EncodeToString(rpcDeposit.OrderId)
			address := hex.EncodeToString(rpcDeposit.Address)

			depositChanged := true
			if depositH1, existsLocked := h1Maps.DepositsLocked[orderID]; existsLocked {
				if depositH1.Amount == rpcDeposit.Amount &&
					hex.EncodeToString(depositH1.Address) == address {
					depositChanged = false
					counters.DepositsLockedUnchanged++
				}
			} else if _, existsPending := h1Maps.DepositsPending[orderID]; existsPending {
				depositChanged = true // State transition pending → locked
			}

			if depositChanged {
				deposit := &indexer.DexDeposit{
					OrderID:    orderID,
					Height:     height,
					HeightTime: blockTime,
					Committee:  currentBatch.Committee,
					Address:    address,
					Amount:     rpcDeposit.Amount,
					State:      indexer.DexLockedState,
				}
				counters.NumDepositsLocked++
				counters.DepositsLockedChanged++
				*deposits = append(*deposits, deposit)
			}
		}

		// Process locked withdrawals
		for _, rpcWithdrawal := range currentBatch.Withdrawals {
			orderID := hex.EncodeToString(rpcWithdrawal.OrderId)
			address := hex.EncodeToString(rpcWithdrawal.Address)

			withdrawalChanged := true
			if withdrawalH1, existsLocked := h1Maps.WithdrawalsLocked[orderID]; existsLocked {
				if withdrawalH1.Percent == rpcWithdrawal.Percent &&
					hex.EncodeToString(withdrawalH1.Address) == address {
					withdrawalChanged = false
					counters.WithdrawalsLockedUnchanged++
				}
			} else if _, existsPending := h1Maps.WithdrawalsPending[orderID]; existsPending {
				withdrawalChanged = true // State transition pending → locked
			}

			if withdrawalChanged {
				withdrawal := &indexer.DexWithdrawal{
					OrderID:    orderID,
					Height:     height,
					HeightTime: blockTime,
					Committee:  currentBatch.Committee,
					Address:    address,
					Percent:    rpcWithdrawal.Percent,
					State:      indexer.DexLockedState,
				}
				counters.NumWithdrawalsLocked++
				counters.WithdrawalsLockedChanged++
				*withdrawals = append(*withdrawals, withdrawal)
			}
		}
	}
}

// processPendingItems processes pending items from next batch with change detection
func (ac *Context) processPendingItems(
	nextBatches []*lib.DexBatch,
	h1Maps *h1ComparisonMaps,
	height uint64, blockTime time.Time,
	orders *[]*indexer.DexOrder,
	deposits *[]*indexer.DexDeposit,
	withdrawals *[]*indexer.DexWithdrawal,
	counters *dexBatchCounters,
) {
	for _, nextBatch := range nextBatches {
		// Process pending orders
		for _, rpcOrder := range nextBatch.Orders {
			orderID := hex.EncodeToString(rpcOrder.OrderId)
			address := hex.EncodeToString(rpcOrder.Address)

			orderChanged := true
			if orderH1, exists := h1Maps.OrdersPending[orderID]; exists {
				if orderH1.AmountForSale == rpcOrder.AmountForSale &&
					orderH1.RequestedAmount == rpcOrder.RequestedAmount &&
					hex.EncodeToString(orderH1.Address) == address {
					orderChanged = false
					counters.OrdersPendingUnchanged++
				}
			}

			if orderChanged {
				order := &indexer.DexOrder{
					OrderID:         orderID,
					Height:          height,
					HeightTime:      blockTime,
					Committee:       nextBatch.Committee,
					Address:         address,
					AmountForSale:   rpcOrder.AmountForSale,
					RequestedAmount: rpcOrder.RequestedAmount,
					State:           indexer.DexPendingState,
					LockedHeight:    0,
				}
				counters.NumOrdersFuture++
				counters.OrdersPendingChanged++
				*orders = append(*orders, order)
			}
		}

		// Process pending deposits
		for _, rpcDeposit := range nextBatch.Deposits {
			orderID := hex.EncodeToString(rpcDeposit.OrderId)
			address := hex.EncodeToString(rpcDeposit.Address)

			depositChanged := true
			if depositH1, exists := h1Maps.DepositsPending[orderID]; exists {
				if depositH1.Amount == rpcDeposit.Amount &&
					hex.EncodeToString(depositH1.Address) == address {
					depositChanged = false
					counters.DepositsPendingUnchanged++
				}
			}

			if depositChanged {
				deposit := &indexer.DexDeposit{
					OrderID:    orderID,
					Height:     height,
					HeightTime: blockTime,
					Committee:  nextBatch.Committee,
					Address:    address,
					Amount:     rpcDeposit.Amount,
					State:      indexer.DexPendingState,
				}
				counters.NumDepositsPending++
				counters.DepositsPendingChanged++
				*deposits = append(*deposits, deposit)
			}
		}

		// Process pending withdrawals
		for _, rpcWithdrawal := range nextBatch.Withdrawals {
			orderID := hex.EncodeToString(rpcWithdrawal.OrderId)
			address := hex.EncodeToString(rpcWithdrawal.Address)

			withdrawalChanged := true
			if withdrawalH1, exists := h1Maps.WithdrawalsPending[orderID]; exists {
				if withdrawalH1.Percent == rpcWithdrawal.Percent &&
					hex.EncodeToString(withdrawalH1.Address) == address {
					withdrawalChanged = false
					counters.WithdrawalsPendingUnchanged++
				}
			}

			if withdrawalChanged {
				withdrawal := &indexer.DexWithdrawal{
					OrderID:    orderID,
					Height:     height,
					HeightTime: blockTime,
					Committee:  nextBatch.Committee,
					Address:    address,
					Percent:    rpcWithdrawal.Percent,
					State:      indexer.DexPendingState,
				}
				counters.NumWithdrawalsPending++
				counters.WithdrawalsPendingChanged++
				*withdrawals = append(*withdrawals, withdrawal)
			}
		}
	}

	// Log change detection summary
	ac.Logger.Debug("DEX batch change detection complete",
		zap.Uint64("height", height),
		zap.Int("orders_locked_changed", counters.OrdersLockedChanged),
		zap.Int("orders_locked_unchanged", counters.OrdersLockedUnchanged),
		zap.Int("orders_pending_changed", counters.OrdersPendingChanged),
		zap.Int("orders_pending_unchanged", counters.OrdersPendingUnchanged),
		zap.Int("deposits_locked_changed", counters.DepositsLockedChanged),
		zap.Int("deposits_locked_unchanged", counters.DepositsLockedUnchanged),
		zap.Int("deposits_pending_changed", counters.DepositsPendingChanged),
		zap.Int("deposits_pending_unchanged", counters.DepositsPendingUnchanged),
		zap.Int("withdrawals_locked_changed", counters.WithdrawalsLockedChanged),
		zap.Int("withdrawals_locked_unchanged", counters.WithdrawalsLockedUnchanged),
		zap.Int("withdrawals_pending_changed", counters.WithdrawalsPendingChanged),
		zap.Int("withdrawals_pending_unchanged", counters.WithdrawalsPendingUnchanged))
}

// insertDexBatchData inserts all DEX batch data to staging tables
func (ac *Context) insertDexBatchData(
	ctx context.Context,
	chainDb chainstore.Store,
	orders []*indexer.DexOrder,
	deposits []*indexer.DexDeposit,
	withdrawals []*indexer.DexWithdrawal,
) error {
	if len(orders) > 0 {
		if err := chainDb.InsertDexOrdersStaging(ctx, orders); err != nil {
			return fmt.Errorf("insert dex orders staging: %w", err)
		}
	}

	if len(deposits) > 0 {
		if err := chainDb.InsertDexDepositsStaging(ctx, deposits); err != nil {
			return fmt.Errorf("insert dex deposits staging: %w", err)
		}
	}

	if len(withdrawals) > 0 {
		if err := chainDb.InsertDexWithdrawalsStaging(ctx, withdrawals); err != nil {
			return fmt.Errorf("insert dex withdrawals staging: %w", err)
		}
	}

	return nil
}

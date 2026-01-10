package activity

import (
	"context"
	"encoding/hex"
	"time"

	"github.com/canopy-network/canopy/lib"
	"github.com/canopy-network/canopyx/app/indexer/types"
	globalstore "github.com/canopy-network/canopyx/pkg/db/global"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"go.uber.org/zap"
)

// indexDexBatchFromBlob indexes DEX orders, deposits, and withdrawals for a given block height.
func (ac *Context) indexDexBatchFromBlob(ctx context.Context, chainDb globalstore.Store, height uint64, heightTime time.Time, currentData *blobData, previousData *blobData, events *blobEventMaps) (types.ActivityIndexDexBatchOutput, float64, error) {
	start := time.Now()
	h1Maps := ac.buildH1ComparisonMaps(previousData.dexBatches, previousData.nextDexBatches)
	orders := make([]*indexermodels.DexOrder, 0)
	deposits := make([]*indexermodels.DexDeposit, 0)
	withdrawals := make([]*indexermodels.DexWithdrawal, 0)
	counters := &dexBatchCounters{}

	ac.processCompleteItems(previousData.dexBatches, events.dexSwap, events.dexDeposit, events.dexWithdrawal, height, heightTime, &orders, &deposits, &withdrawals, counters)
	ac.processLockedItems(currentData.dexBatches, h1Maps, height, heightTime, &orders, &deposits, &withdrawals, counters)
	ac.processPendingItems(currentData.nextDexBatches, h1Maps, height, heightTime, &orders, &deposits, &withdrawals, counters)

	if len(orders) > 0 {
		if err := chainDb.InsertDexOrders(ctx, orders); err != nil {
			return types.ActivityIndexDexBatchOutput{}, 0, err
		}
	}
	if len(deposits) > 0 {
		if err := chainDb.InsertDexDeposits(ctx, deposits); err != nil {
			return types.ActivityIndexDexBatchOutput{}, 0, err
		}
	}
	if len(withdrawals) > 0 {
		if err := chainDb.InsertDexWithdrawals(ctx, withdrawals); err != nil {
			return types.ActivityIndexDexBatchOutput{}, 0, err
		}
	}

	out := types.ActivityIndexDexBatchOutput{
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
	}
	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return out, durationMs, nil
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
	swapEvents, depositEvents, withdrawalEvents map[string]*globalstore.EventDexBatch,
	height uint64, blockTime time.Time,
	orders *[]*indexermodels.DexOrder,
	deposits *[]*indexermodels.DexDeposit,
	withdrawals *[]*indexermodels.DexWithdrawal,
	counters *dexBatchCounters,
) {
	for _, currentBatchH1 := range currentBatchesH1 {
		// Process complete orders
		for _, rpcOrder := range currentBatchH1.Orders {
			orderID := hex.EncodeToString(rpcOrder.OrderId)
			address := hex.EncodeToString(rpcOrder.Address)

			if swapEvent, exists := swapEvents[orderID]; exists {
				order := &indexermodels.DexOrder{
					OrderID:         orderID,
					Height:          height,
					HeightTime:      blockTime,
					Committee:       uint16(currentBatchH1.Committee),
					Address:         address,
					AmountForSale:   rpcOrder.AmountForSale,
					RequestedAmount: rpcOrder.RequestedAmount,
					State:           indexermodels.DexCompleteState,
					LockedHeight:    currentBatchH1.LockedHeight,
				}

				order.Success = swapEvent.Success
				if order.Success {
					counters.NumOrdersSuccess++
				} else {
					counters.NumOrdersFailed++
				}
				order.SoldAmount = swapEvent.SoldAmount
				order.BoughtAmount = swapEvent.BoughtAmount
				order.LocalOrigin = swapEvent.LocalOrigin

				counters.NumOrdersComplete++
				*orders = append(*orders, order)
			}
		}

		// Process complete deposits
		for _, rpcDeposit := range currentBatchH1.Deposits {
			orderID := hex.EncodeToString(rpcDeposit.OrderId)
			address := hex.EncodeToString(rpcDeposit.Address)

			if depositEvent, exists := depositEvents[orderID]; exists {
				deposit := &indexermodels.DexDeposit{
					OrderID:        orderID,
					Height:         height,
					HeightTime:     blockTime,
					Committee:      uint16(currentBatchH1.Committee),
					Address:        address,
					Amount:         rpcDeposit.Amount,
					State:          indexermodels.DexCompleteState,
					LocalOrigin:    depositEvent.LocalOrigin,
					PointsReceived: depositEvent.PointsReceived,
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
				withdrawal := &indexermodels.DexWithdrawal{
					OrderID:      orderID,
					Height:       height,
					HeightTime:   blockTime,
					Committee:    uint16(currentBatchH1.Committee),
					Address:      address,
					Percent:      rpcWithdrawal.Percent,
					State:        indexermodels.DexCompleteState,
					LocalAmount:  withdrawalEvent.LocalAmount,
					RemoteAmount: withdrawalEvent.RemoteAmount,
					PointsBurned: withdrawalEvent.PointsBurned,
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
	orders *[]*indexermodels.DexOrder,
	deposits *[]*indexermodels.DexDeposit,
	withdrawals *[]*indexermodels.DexWithdrawal,
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
				order := &indexermodels.DexOrder{
					OrderID:         orderID,
					Height:          height,
					HeightTime:      blockTime,
					Committee:       uint16(currentBatch.Committee),
					Address:         address,
					AmountForSale:   rpcOrder.AmountForSale,
					RequestedAmount: rpcOrder.RequestedAmount,
					State:           indexermodels.DexLockedState,
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
				deposit := &indexermodels.DexDeposit{
					OrderID:    orderID,
					Height:     height,
					HeightTime: blockTime,
					Committee:  uint16(currentBatch.Committee),
					Address:    address,
					Amount:     rpcDeposit.Amount,
					State:      indexermodels.DexLockedState,
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
				withdrawal := &indexermodels.DexWithdrawal{
					OrderID:    orderID,
					Height:     height,
					HeightTime: blockTime,
					Committee:  uint16(currentBatch.Committee),
					Address:    address,
					Percent:    rpcWithdrawal.Percent,
					State:      indexermodels.DexLockedState,
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
	orders *[]*indexermodels.DexOrder,
	deposits *[]*indexermodels.DexDeposit,
	withdrawals *[]*indexermodels.DexWithdrawal,
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
				order := &indexermodels.DexOrder{
					OrderID:         orderID,
					Height:          height,
					HeightTime:      blockTime,
					Committee:       uint16(nextBatch.Committee),
					Address:         address,
					AmountForSale:   rpcOrder.AmountForSale,
					RequestedAmount: rpcOrder.RequestedAmount,
					State:           indexermodels.DexPendingState,
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
				deposit := &indexermodels.DexDeposit{
					OrderID:    orderID,
					Height:     height,
					HeightTime: blockTime,
					Committee:  uint16(nextBatch.Committee),
					Address:    address,
					Amount:     rpcDeposit.Amount,
					State:      indexermodels.DexPendingState,
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
				withdrawal := &indexermodels.DexWithdrawal{
					OrderID:    orderID,
					Height:     height,
					HeightTime: blockTime,
					Committee:  uint16(nextBatch.Committee),
					Address:    address,
					Percent:    rpcWithdrawal.Percent,
					State:      indexermodels.DexPendingState,
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

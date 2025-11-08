package activity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/canopy-network/canopyx/app/indexer/types"
	indexer "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"go.uber.org/zap"
)

// IndexOrders indexes order state changes for a given block using the snapshot-on-change pattern.
//
// Core Algorithm:
// 1. Parallel RPC fetch: Fetch RPC(height H) and RPC(height H-1) simultaneously using goroutines
// 2. Compare: Build map of previous states, iterate current orders
// 3. Detect changes: If order state differs (amount, status, buyer, etc.), create snapshot
// 4. Insert to staging: Batch insert all changed orders
//
// Created Height Tracking:
// Order creation heights are tracked via the order_created_height materialized view,
// which automatically calculates MIN(height) for each order_id. This approach:
// - Works correctly with parallel/unordered indexing
// - Eliminates the need to query and store created_height on every snapshot
// - Automatically updates as older blocks are indexed
//
// Performance:
// - Parallel RPC fetching reduces latency by ~50% (2 concurrent requests)
// - Only stores changed orders (significant storage savings vs full snapshots)
//
// Note: This implementation assumes the chainID parameter needed for RPC calls can be derived
// from the chain configuration. If a numeric chain ID is needed, it should be added to the
// admin.Chain model or passed as a parameter.
func (ac *Context) IndexOrders(ctx context.Context, input types.ActivityIndexAtHeight) (types.ActivityIndexOrdersOutput, error) {
	start := time.Now()

	cli, err := ac.rpcClient(ctx)
	if err != nil {
		return types.ActivityIndexOrdersOutput{}, err
	}

	// Get chain database
	chainDb, err := ac.GetChainDb(ctx, ac.ChainID)
	if err != nil {
		return types.ActivityIndexOrdersOutput{}, err
	}

	// Parallel RPC fetch using shared worker pool for performance
	var (
		currentOrders  []*rpc.RpcOrder
		previousOrders []*rpc.RpcOrder
		currentErr     error
		previousErr    error
	)

	// Get a subgroup from the shared worker pool for parallel RPC fetching
	pool := ac.WorkerPool(2)
	group := pool.NewGroupContext(ctx)
	groupCtx := group.Context()

	// Worker 1: Fetch current height orders from RPC
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		currentOrders, currentErr = cli.OrdersByHeight(groupCtx, input.Height, ac.ChainID)
	})

	// Worker 2: Fetch previous height orders from RPC
	// Note: Unlike accounts, orders don't have genesis state, so we just fetch from RPC
	// At height 1 (genesis), there are no previous orders to fetch
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		if input.Height > 1 {
			previousOrders, previousErr = cli.OrdersByHeight(groupCtx, input.Height-1, ac.ChainID)
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
		return types.ActivityIndexOrdersOutput{}, fmt.Errorf("fetch current orders at height %d: %w", input.Height, currentErr)
	}
	if previousErr != nil && input.Height > 1 {
		return types.ActivityIndexOrdersOutput{}, fmt.Errorf("fetch previous orders at height %d: %w", input.Height-1, previousErr)
	}

	// Build previous state map for O(1) lookups
	// Map key is orderID, value is the full order state
	prevMap := make(map[string]*rpc.RpcOrder, len(previousOrders))
	for _, order := range previousOrders {
		prevMap[order.ID] = order
	}

	// Query order lifecycle events from staging table (event-driven state tracking)
	// EventOrderBookSwap events indicate when an order transitions to "filled" state
	orderEvents, err := chainDb.GetEventsByTypeAndHeight(
		ctx, input.Height, true,
		rpc.EventTypeAsStr(rpc.EventTypeOrderBookSwap),
	)
	if err != nil {
		return types.ActivityIndexOrdersOutput{}, fmt.Errorf("query order events at height %d: %w", input.Height, err)
	}

	// Build event map by order ID for O(1) lookup
	swapEvents := make(map[string]*indexer.Event)
	for _, event := range orderEvents {
		// Events have nullable OrderID field
		if event.OrderID == nil {
			continue
		}
		orderID := *event.OrderID
		swapEvents[orderID] = event
	}

	// Compare and collect changed orders
	changedOrders := make([]*indexer.Order, 0)
	for _, curr := range currentOrders {
		prev, existed := prevMap[curr.ID]

		// Check if the order has a swap event (state transition to "complete")
		_, hasSwapEvent := swapEvents[curr.ID]

		// Determine if order state changed
		// We check all significant fields: amount, buyer details, deadline,
		// OR if there's a swap event indicating state transition
		hasChanged := !existed || hasSwapEvent || orderStateChanged(prev, curr)

		if hasChanged {
			// Derive status from events and state:
			// - "complete" if there's a swap event
			// - "open" otherwise (order exists in current state)
			status := indexer.OrderStatusOpen
			if hasSwapEvent {
				status = indexer.OrderStatusComplete
			}

			changedOrders = append(changedOrders, &indexer.Order{
				OrderID:              curr.ID,
				Height:               input.Height,
				HeightTime:           input.BlockTime,
				Committee:            curr.Committee,
				Data:                 curr.Data,
				AmountForSale:        curr.AmountForSale,
				RequestedAmount:      curr.RequestedAmount,
				SellerReceiveAddress: curr.SellerReceiveAddress,
				BuyerSendAddress:     curr.BuyerSendAddress,
				BuyerReceiveAddress:  curr.BuyerReceiveAddress,
				BuyerChainDeadline:   curr.BuyerChainDeadline,
				SellersSendAddress:   curr.SellersSendAddress,
				Status:               status,
			})
		}
	}

	// Check for canceled orders (existed at H-1 but not at H, and no swap event)
	for prevID, prevOrder := range prevMap {
		// Check if order exists at current height
		_, existsNow := func() (bool, bool) {
			for _, curr := range currentOrders {
				if curr.ID == prevID {
					return true, true
				}
			}
			return false, false
		}()

		// If order disappeared and there's no swap event, it was canceled
		if !existsNow {
			_, hasSwapEvent := swapEvents[prevID]
			if !hasSwapEvent {
				// Order was canceled - create a final snapshot with "canceled" status
				changedOrders = append(changedOrders, &indexer.Order{
					OrderID:              prevOrder.ID,
					Height:               input.Height,
					HeightTime:           input.BlockTime,
					Committee:            prevOrder.Committee,
					Data:                 prevOrder.Data,
					AmountForSale:        prevOrder.AmountForSale,
					RequestedAmount:      prevOrder.RequestedAmount,
					SellerReceiveAddress: prevOrder.SellerReceiveAddress,
					BuyerSendAddress:     prevOrder.BuyerSendAddress,
					BuyerReceiveAddress:  prevOrder.BuyerReceiveAddress,
					BuyerChainDeadline:   prevOrder.BuyerChainDeadline,
					SellersSendAddress:   prevOrder.SellersSendAddress,
					Status:               indexer.OrderStatusCanceled,
				})
			}
		}
	}

	// Insert to staging table
	if len(changedOrders) > 0 {
		if err := chainDb.InsertOrdersStaging(ctx, changedOrders); err != nil {
			return types.ActivityIndexOrdersOutput{}, fmt.Errorf("insert orders staging: %w", err)
		}
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	ac.Logger.Info("Indexed orders",
		zap.Uint64("chainId", ac.ChainID),
		zap.Uint64("height", input.Height),
		zap.Int("totalOrders", len(currentOrders)),
		zap.Int("changedOrders", len(changedOrders)),
		zap.Int("swapEvents", len(swapEvents)),
		zap.Float64("durationMs", durationMs))

	return types.ActivityIndexOrdersOutput{
		NumOrders:  uint32(len(changedOrders)),
		DurationMs: durationMs,
	}, nil
}

// orderStateChanged compares two order states to detect changes.
// Returns true if any significant field has changed.
func orderStateChanged(prev, curr *rpc.RpcOrder) bool {
	// Check all significant fields
	if prev.Data != curr.Data {
		return true
	}
	if prev.AmountForSale != curr.AmountForSale {
		return true
	}
	if prev.RequestedAmount != curr.RequestedAmount {
		return true
	}
	if prev.SellerReceiveAddress != curr.SellerReceiveAddress {
		return true
	}
	if prev.BuyerSendAddress != curr.BuyerSendAddress {
		return true
	}
	if prev.BuyerReceiveAddress != curr.BuyerReceiveAddress {
		return true
	}
	if prev.BuyerChainDeadline != curr.BuyerChainDeadline {
		return true
	}
	if prev.SellersSendAddress != curr.SellersSendAddress {
		return true
	}

	return false
}

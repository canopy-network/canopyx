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
func (c *Context) IndexOrders(ctx context.Context, input types.IndexOrdersInput) (types.IndexOrdersOutput, error) {
	start := time.Now()

	// Get chain metadata
	ch, err := c.AdminDB.GetChain(ctx, input.ChainID)
	if err != nil {
		return types.IndexOrdersOutput{}, err
	}

	// Get chain database
	chainDb, err := c.NewChainDb(ctx, input.ChainID)
	if err != nil {
		return types.IndexOrdersOutput{}, err
	}

	// Create RPC client
	cli := c.rpcClient(ch.RPCEndpoints)

	// IMPORTANT: The RPC OrdersByHeight method requires a numeric chainID parameter.
	// This should be extracted from chain configuration or passed as input.
	// For now, we use a placeholder value of 1. This should be updated based on
	// the actual chain configuration structure.
	//
	// TODO: Add numeric chain_id field to admin.Chain model or pass it in IndexOrdersInput
	var numericChainID uint64 = 1 // PLACEHOLDER - should come from chain config

	// Parallel RPC fetch using goroutines for performance
	var (
		currentOrders  []*rpc.RpcOrder
		previousOrders []*rpc.RpcOrder
		currentErr     error
		previousErr    error
		wg             sync.WaitGroup
	)

	wg.Add(2)

	// Worker 1: Fetch current height orders from RPC
	go func() {
		defer wg.Done()
		currentOrders, currentErr = cli.OrdersByHeight(ctx, input.Height, numericChainID)
	}()

	// Worker 2: Fetch previous height orders from RPC
	// Note: Unlike accounts, orders don't have genesis state, so we just fetch from RPC
	// At height 1 (genesis), there are no previous orders to fetch
	go func() {
		defer wg.Done()
		if input.Height > 1 {
			previousOrders, previousErr = cli.OrdersByHeight(ctx, input.Height-1, numericChainID)
		}
	}()

	// Wait for both workers to complete
	wg.Wait()

	// Check for errors
	if currentErr != nil {
		return types.IndexOrdersOutput{}, fmt.Errorf("fetch current orders at height %d: %w", input.Height, currentErr)
	}
	if previousErr != nil && input.Height > 1 {
		return types.IndexOrdersOutput{}, fmt.Errorf("fetch previous orders at height %d: %w", input.Height-1, previousErr)
	}

	// Build previous state map for O(1) lookups
	// Map key is orderID, value is the full order state
	prevMap := make(map[string]*rpc.RpcOrder, len(previousOrders))
	for _, order := range previousOrders {
		prevMap[order.OrderID] = order
	}

	// Query order lifecycle events from staging table (event-driven state tracking)
	// EventOrderBookSwap events indicate when an order transitions to "filled" state
	orderEvents, err := chainDb.GetEventsByTypeAndHeight(ctx, input.Height, "EventOrderBookSwap")
	if err != nil {
		return types.IndexOrdersOutput{}, fmt.Errorf("query order events at height %d: %w", input.Height, err)
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
		prev, existed := prevMap[curr.OrderID]

		// Check if order has a swap event (state transition to "filled")
		_, hasSwapEvent := swapEvents[curr.OrderID]

		// Determine if order state changed
		// We check all significant fields: amount, status, buyer, deadline
		// OR if there's a swap event indicating state transition
		hasChanged := !existed || hasSwapEvent || orderStateChanged(prev, curr)

		if hasChanged {
			changedOrders = append(changedOrders, &indexer.Order{
				OrderID:         curr.OrderID,
				Height:          input.Height,
				HeightTime:      input.BlockTime,
				Committee:       curr.Committee,
				AmountForSale:   curr.AmountForSale,
				RequestedAmount: curr.RequestedAmount,
				SellerAddress:   curr.SellerAddress,
				BuyerAddress:    curr.BuyerAddress,
				Deadline:        curr.Deadline,
				Status:          curr.Status,
			})
		}
	}

	// Insert to staging table
	if len(changedOrders) > 0 {
		if err := chainDb.InsertOrdersStaging(ctx, changedOrders); err != nil {
			return types.IndexOrdersOutput{}, fmt.Errorf("insert orders staging: %w", err)
		}
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	c.Logger.Info("Indexed orders",
		zap.Uint64("chainId", input.ChainID),
		zap.Uint64("height", input.Height),
		zap.Int("totalOrders", len(currentOrders)),
		zap.Int("changedOrders", len(changedOrders)),
		zap.Int("swapEvents", len(swapEvents)),
		zap.Float64("durationMs", durationMs))

	return types.IndexOrdersOutput{
		NumOrders:  uint32(len(changedOrders)),
		DurationMs: durationMs,
	}, nil
}

// orderStateChanged compares two order states to detect changes.
// Returns true if any significant field has changed.
func orderStateChanged(prev, curr *rpc.RpcOrder) bool {
	// Check all significant fields
	if prev.AmountForSale != curr.AmountForSale {
		return true
	}
	if prev.RequestedAmount != curr.RequestedAmount {
		return true
	}
	if prev.Status != curr.Status {
		return true
	}
	if prev.SellerAddress != curr.SellerAddress {
		return true
	}

	// Check nullable fields (buyer address, deadline)
	if !stringPtrEqual(prev.BuyerAddress, curr.BuyerAddress) {
		return true
	}
	if !uint64PtrEqual(prev.Deadline, curr.Deadline) {
		return true
	}

	return false
}

// stringPtrEqual compares two nullable string pointers.
// Returns true if both are nil or both point to the same value.
func stringPtrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// uint64PtrEqual compares two nullable uint64 pointers.
// Returns true if both are nil or both point to the same value.
func uint64PtrEqual(a, b *uint64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

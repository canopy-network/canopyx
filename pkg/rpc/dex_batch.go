package rpc

import (
	"context"
	"fmt"
	"net/http"
)

// AllDexBatchesByHeight fetches ALL DEX batches at a specific height across all committees.
// Uses the special id: 0 parameter to return batches for all committees in a single call.
//
// This is more efficient than querying committees and fetching each batch individually.
// The RPC returns all active batches at the specified height.
func (c *HTTPClient) AllDexBatchesByHeight(ctx context.Context, height uint64) ([]*RpcDexBatch, error) {
	req := DexBatchRequest{
		Height:    height,
		Committee: 0, // 0 means "all committees"
	}

	// Try to unmarshal as array first (multiple batches)
	var batches []*RpcDexBatch
	if err := c.doJSON(ctx, http.MethodPost, dexBatchByHeightPath, req, &batches); err != nil {
		// If that fails, try as single object
		var singleBatch RpcDexBatch
		if err := c.doJSON(ctx, http.MethodPost, dexBatchByHeightPath, req, &singleBatch); err != nil {
			return nil, fmt.Errorf("fetch all dex batches at height %d: %w", height, err)
		}
		batches = []*RpcDexBatch{&singleBatch}
	}

	return batches, nil
}

// AllNextDexBatchesByHeight fetches ALL next DEX batches at a specific height across all committees.
// Uses the special id: 0 parameter to return next batches for all committees in a single call.
//
// This is more efficient than querying committees and fetching each batch individually.
// The RPC returns all active next batches at the specified height.
func (c *HTTPClient) AllNextDexBatchesByHeight(ctx context.Context, height uint64) ([]*RpcDexBatch, error) {
	req := DexBatchRequest{
		Height:    height,
		Committee: 0, // 0 means "all committees"
	}

	// Try to unmarshal as array first (multiple batches)
	var batches []*RpcDexBatch
	if err := c.doJSON(ctx, http.MethodPost, nextDexBatchByHeightPath, req, &batches); err != nil {
		// If that fails, try as single object
		var singleBatch RpcDexBatch
		if err := c.doJSON(ctx, http.MethodPost, nextDexBatchByHeightPath, req, &singleBatch); err != nil {
			return nil, fmt.Errorf("fetch all next dex batches at height %d: %w", height, err)
		}
		batches = []*RpcDexBatch{&singleBatch}
	}

	return batches, nil
}

// RpcDexLimitOrder represents a DEX limit order from the RPC.
// This matches the DexLimitOrder proto structure returned by dex-batch endpoints.
type RpcDexLimitOrder struct {
	OrderID         string `json:"orderId"`         // Hex-encoded order ID
	AmountForSale   uint64 `json:"amountForSale"`   // Amount of asset being sold
	RequestedAmount uint64 `json:"requestedAmount"` // Minimum amount of counter-asset to receive
	Address         string `json:"address"`         // Hex-encoded address
}

// RpcDexLiquidityDeposit represents a liquidity deposit from the RPC.
// This matches the DexLiquidityDeposit proto structure.
type RpcDexLiquidityDeposit struct {
	OrderID string `json:"orderId"` // Hex-encoded order ID
	Address string `json:"address"` // Hex-encoded address
	Amount  uint64 `json:"amount"`  // Amount being deposited
}

// RpcDexLiquidityWithdraw represents a liquidity withdrawal from the RPC.
// This matches the DexLiquidityWithdraw proto structure.
type RpcDexLiquidityWithdraw struct {
	OrderID string `json:"orderId"` // Hex-encoded order ID
	Address string `json:"address"` // Hex-encoded address
	Percent uint64 `json:"percent"` // Percentage of pool points being withdrawn (0-100)
}

// RpcPoolPoints represents an address's pool points allocation.
// This matches the PoolPoints proto structure.
type RpcPoolPoints struct {
	Address string `json:"address"` // Hex-encoded address
	Points  uint64 `json:"points"`  // Number of pool points
}

// RpcDexBatch represents a DEX batch from the RPC.
// This matches the DexBatch proto structure returned by /v1/query/dex-batch and /v1/query/next-dex-batch.
type RpcDexBatch struct {
	Committee       uint64                     `json:"committee"`       // Committee ID (counter-asset chain ID)
	ReceiptHash     string                     `json:"receiptHash"`     // Hash of counter chain batch receipts
	Orders          []*RpcDexLimitOrder        `json:"orders"`          // Limit orders in this batch
	Deposits        []*RpcDexLiquidityDeposit  `json:"deposits"`        // Liquidity deposits in this batch
	Withdrawals     []*RpcDexLiquidityWithdraw `json:"withdraws"`       // Liquidity withdrawals in this batch
	PoolSize        uint64                     `json:"poolSize"`        // Current liquidity pool balance
	CounterPoolSize uint64                     `json:"counterPoolSize"` // Last known counter chain pool size
	PoolPoints      []*RpcPoolPoints           `json:"poolPoints"`      // Pool points by holder
	TotalPoolPoints uint64                     `json:"totalPoolPoints"` // Total pool points
	Receipts        []uint64                   `json:"receipts"`        // Execution receipts (amounts distributed)
	LockedHeight    uint64                     `json:"locked_height"`   // Height when batch was locked
}

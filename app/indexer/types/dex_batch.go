package types

import "time"

// IndexDexBatchInput contains the parameters for indexing DEX batches (orders, deposits, withdrawals).
// Uses bulk fetch to retrieve batches for all committees in a single RPC call.
type IndexDexBatchInput struct {
	ChainID   uint64    `json:"chainId"`
	Height    uint64    `json:"height"`
	BlockTime time.Time `json:"blockTime"` // Block timestamp for populating height_time
}

// IndexDexBatchOutput contains the number of indexed DEX entities along with execution duration.
type IndexDexBatchOutput struct {
	NumOrders      uint32  `json:"numOrders"`      // Number of orders indexed
	NumDeposits    uint32  `json:"numDeposits"`    // Number of deposits indexed
	NumWithdrawals uint32  `json:"numWithdrawals"` // Number of withdrawals indexed
	DurationMs     float64 `json:"durationMs"`     // Execution time in milliseconds
}

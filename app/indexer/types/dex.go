package types

// ActivityIndexDexBatchOutput contains the number of indexed DEX entities along with execution duration.
type ActivityIndexDexBatchOutput struct {
	NumOrders      uint32  `json:"numOrders"`      // Number of orders indexed
	NumDeposits    uint32  `json:"numDeposits"`    // Number of deposits indexed
	NumWithdrawals uint32  `json:"numWithdrawals"` // Number of withdrawals indexed
	DurationMs     float64 `json:"durationMs"`     // Execution time in milliseconds
}

// ActivityIndexDexPricesOutput contains the number of indexed DEX price records along with execution duration.
type ActivityIndexDexPricesOutput struct {
	NumPrices  uint32  `json:"numPrices"`  // Number of DEX price records indexed
	DurationMs float64 `json:"durationMs"` // Execution time in milliseconds
}

// ActivityIndexOrdersOutput contains the number of changed orders (snapshots created) along with execution duration.
type ActivityIndexOrdersOutput struct {
	NumOrders  uint32  `json:"numOrders"`  // Number of changed orders (snapshots created)
	DurationMs float64 `json:"durationMs"` // Execution time in milliseconds
}

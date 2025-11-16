package types

// ActivityIndexDexBatchOutput contains the number of indexed DEX entities along with execution duration.
type ActivityIndexDexBatchOutput struct {
	NumOrders              uint32  `json:"numOrders"`              // Number of orders indexed
	NumOrdersFuture        uint32  `json:"numOrdersFuture"`        // Number of future orders (nextBatch)
	NumOrdersLocked        uint32  `json:"numOrdersLocked"`        // Number of locked orders (currentBatch)
	NumOrdersComplete      uint32  `json:"numOrdersComplete"`      // Number of complete orders (from H-1 with events)
	NumOrdersSuccess       uint32  `json:"numOrdersSuccess"`       // Number of successful complete orders
	NumOrdersFailed        uint32  `json:"numOrdersFailed"`        // Number of failed complete orders
	NumDeposits            uint32  `json:"numDeposits"`            // Number of deposits indexed
	NumDepositsPending     uint32  `json:"numDepositsPending"`     // Number of pending deposits (nextBatch)
	NumDepositsLocked      uint32  `json:"numDepositsLocked"`      // Number of locked deposits (currentBatch)
	NumDepositsComplete    uint32  `json:"numDepositsComplete"`    // Number of complete deposits (from H-1 with events)
	NumWithdrawals         uint32  `json:"numWithdrawals"`         // Number of withdrawals indexed
	NumWithdrawalsPending  uint32  `json:"numWithdrawalsPending"`  // Number of pending withdrawals (nextBatch)
	NumWithdrawalsLocked   uint32  `json:"numWithdrawalsLocked"`   // Number of locked withdrawals (currentBatch)
	NumWithdrawalsComplete uint32  `json:"numWithdrawalsComplete"` // Number of complete withdrawals (from H-1 with events)
	DurationMs             float64 `json:"durationMs"`             // Execution time in milliseconds
}

// ActivityIndexDexPricesOutput contains the number of indexed DEX price records along with execution duration.
type ActivityIndexDexPricesOutput struct {
	NumPrices  uint32  `json:"numPrices"`  // Number of DEX price records indexed
	DurationMs float64 `json:"durationMs"` // Execution time in milliseconds
}

// ActivityIndexOrdersOutput contains the number of changed orders (snapshots created) along with execution duration.
type ActivityIndexOrdersOutput struct {
	NumOrders          uint32  `json:"numOrders"`          // Number of changed orders (snapshots created)
	NumOrdersNew       uint32  `json:"numOrdersNew"`       // Number of new orders (first seen at this height)
	NumOrdersOpen      uint32  `json:"numOrdersOpen"`      // Number of open orders (all current, not just changed)
	NumOrdersFilled    uint32  `json:"numOrdersFilled"`    // Number of filled/complete orders (all current, not just changed)
	NumOrdersCancelled uint32  `json:"numOrdersCancelled"` // Number of cancelled orders (detected this height)
	DurationMs         float64 `json:"durationMs"`         // Execution time in milliseconds
}

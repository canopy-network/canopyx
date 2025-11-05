package types

// ActivityIndexTransactionsOutput contains the number of indexed transactions along with execution duration.
type ActivityIndexTransactionsOutput struct {
	NumTxs         uint32            `json:"numTxs"`         // Number of transactions indexed
	TxCountsByType map[string]uint32 `json:"txCountsByType"` // Count per type: {"send": 5, "delegate": 2}
	DurationMs     float64           `json:"durationMs"`     // Execution time in milliseconds
}

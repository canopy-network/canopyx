package types

// ActivityIndexPoolsOutput contains the number of indexed pools along with execution duration.
type ActivityIndexPoolsOutput struct {
	NumPools       uint32  `json:"numPools"`       // Number of pools indexed
	NumPoolHolders uint32  `json:"numPoolHolders"` // Number of pool point holders indexed (snapshot-on-change)
	DurationMs     float64 `json:"durationMs"`     // Execution time in milliseconds
}

package types

// ActivityIndexPoolsOutput contains the number of indexed pools along with execution duration.
type ActivityIndexPoolsOutput struct {
	NumPools   uint32  `json:"numPools"`   // Number of pools indexed
	DurationMs float64 `json:"durationMs"` // Execution time in milliseconds
}

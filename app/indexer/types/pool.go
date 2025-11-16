package types

// ActivityIndexPoolsOutput contains the number of indexed pools along with execution duration.
type ActivityIndexPoolsOutput struct {
	NumPools          uint32  `json:"numPools"`          // Number of pools indexed
	NumPoolsNew       uint32  `json:"numPoolsNew"`       // Number of new pools (first seen at this height)
	NumPoolHolders    uint32  `json:"numPoolHolders"`    // Number of pool point holders indexed (snapshot-on-change)
	NumPoolHoldersNew uint32  `json:"numPoolHoldersNew"` // Number of new pool holders (first seen at this height)
	DurationMs        float64 `json:"durationMs"`        // Execution time in milliseconds
}

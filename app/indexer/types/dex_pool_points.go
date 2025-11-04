package types

import "time"

// IndexDexPoolPointsInput contains the parameters for indexing DEX pool points by holder.
type IndexDexPoolPointsInput struct {
	ChainID   uint64    `json:"chainId"`
	Height    uint64    `json:"height"`
	BlockTime time.Time `json:"blockTime"` // Block timestamp for populating height_time
}

// IndexDexPoolPointsOutput contains the number of indexed pool point holders along with execution duration.
type IndexDexPoolPointsOutput struct {
	NumHolders uint32  `json:"numHolders"` // Number of pool point holders indexed
	DurationMs float64 `json:"durationMs"` // Execution time in milliseconds
}

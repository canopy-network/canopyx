package types

import (
	"time"
)

// IndexValidatorsInput contains the parameters for indexing validators and their signing info.
type IndexValidatorsInput struct {
	ChainID   uint64    `json:"chainId"`
	Height    uint64    `json:"height"`
	BlockTime time.Time `json:"blockTime"` // Block timestamp for populating height_time
}

// IndexValidatorsOutput contains the results of indexing validators along with execution duration.
type IndexValidatorsOutput struct {
	NumValidators   uint32  `json:"numValidators"`   // Number of validators indexed
	NumSigningInfos uint32  `json:"numSigningInfos"` // Number of signing info records indexed
	DurationMs      float64 `json:"durationMs"`      // Execution time in milliseconds
}

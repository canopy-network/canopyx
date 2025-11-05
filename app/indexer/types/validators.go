package types

// ActivityIndexValidatorsOutput contains the results of indexing validators along with execution duration.
type ActivityIndexValidatorsOutput struct {
	NumValidators         uint32  `json:"numValidators"`         // Number of validators indexed
	NumSigningInfos       uint32  `json:"numSigningInfos"`       // Number of signing info records indexed
	NumDoubleSigningInfos uint32  `json:"numDoubleSigningInfos"` // Number of double signing info records indexed
	DurationMs            float64 `json:"durationMs"`            // Execution time in milliseconds
}

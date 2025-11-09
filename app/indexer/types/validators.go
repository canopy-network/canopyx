package types

// ActivityIndexValidatorsOutput contains the results of indexing validators along with execution duration.
type ActivityIndexValidatorsOutput struct {
	NumValidators         uint32  `json:"numValidators"`         // Number of validators indexed
	NumValidatorsNew      uint32  `json:"numValidatorsNew"`      // Number of new validators (first seen at this height)
	NumValidatorsActive   uint32  `json:"numValidatorsActive"`   // Number of active validators (all, not just changed)
	NumValidatorsPaused   uint32  `json:"numValidatorsPaused"`   // Number of paused validators (all, not just changed)
	NumValidatorsUnstaking uint32 `json:"numValidatorsUnstaking"` // Number of unstaking validators (all, not just changed)
	NumSigningInfos       uint32  `json:"numSigningInfos"`       // Number of signing info records indexed
	NumSigningInfosNew    uint32  `json:"numSigningInfosNew"`    // Number of new signing info records (first seen at this height)
	NumDoubleSigningInfos uint32  `json:"numDoubleSigningInfos"` // Number of double signing info records indexed
	NumCommitteeValidators uint32 `json:"numCommitteeValidators"` // Number of committee-validator junction records
	DurationMs            float64 `json:"durationMs"`            // Execution time in milliseconds
}

package types

// ActivityIndexParamsOutput contains the result of indexing params along with execution duration.
type ActivityIndexParamsOutput struct {
	ParamsChanged bool    `json:"paramsChanged"` // True if params changed from previous height
	DurationMs    float64 `json:"durationMs"`    // Execution time in milliseconds
}

// ActivityIndexPollOutput contains the result of indexing poll data along with execution duration.
type ActivityIndexPollOutput struct {
	NumProposals uint32  `json:"numProposals"` // Number of proposals in the poll
	DurationMs   float64 `json:"durationMs"`   // Execution time in milliseconds
}

// ActivityIndexCommitteesOutput contains the result of indexing committees along with execution duration.
type ActivityIndexCommitteesOutput struct {
	NumCommittees           uint32  `json:"numCommittees"`           // Number of committees that changed
	NumCommitteesNew        uint32  `json:"numCommitteesNew"`        // Number of new committees (first seen at this height)
	NumCommitteesSubsidized uint32  `json:"numCommitteesSubsidized"` // Number of subsidized committees (all, not just changed)
	NumCommitteesRetired    uint32  `json:"numCommitteesRetired"`    // Number of retired committees (all, not just changed)
	DurationMs              float64 `json:"durationMs"`              // Execution time in milliseconds
}

// ActivityIndexSupplyOutput contains the result of indexing supply metrics along with execution duration.
type ActivityIndexSupplyOutput struct {
	Changed    bool    `json:"changed"`    // True if supply changed from previous height
	DurationMs float64 `json:"durationMs"` // Execution time in milliseconds
}

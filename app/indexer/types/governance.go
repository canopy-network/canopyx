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
	NumCommittees uint32  `json:"numCommittees"` // Number of committees that changed
	DurationMs    float64 `json:"durationMs"`    // Execution time in milliseconds
}

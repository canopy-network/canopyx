package types

// ActivityIndexAccountsOutput contains the number of changed accounts (snapshots created) along with execution duration.
type ActivityIndexAccountsOutput struct {
	NumAccounts    uint32  `json:"numAccounts"`    // Number of changed accounts (snapshots created)
	NumAccountsNew uint32  `json:"numAccountsNew"` // Number of new accounts (first seen at this height)
	DurationMs     float64 `json:"durationMs"`     // Execution time in milliseconds
}

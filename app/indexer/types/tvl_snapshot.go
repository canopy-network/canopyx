package types

import "time"

// ActivityUpdateTVLSnapshotInput defines the input for the UpdateTVLSnapshot activity.
// Called after each block is indexed to update the current hour's TVL snapshot.
type ActivityUpdateTVLSnapshotInput struct {
	Height    uint64    `json:"height"`    // Block height that was just indexed
	BlockTime time.Time `json:"blockTime"` // Block timestamp
}

// ActivityUpdateTVLSnapshotOutput contains the result of updating the TVL snapshot.
type ActivityUpdateTVLSnapshotOutput struct {
	SnapshotHour time.Time `json:"snapshotHour"` // The hour bucket that was updated
	TotalTVL     uint64    `json:"totalTvl"`     // Total TVL computed
	DurationMs   float64   `json:"durationMs"`   // Execution time in milliseconds
}

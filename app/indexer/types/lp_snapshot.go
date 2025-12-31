package types

import "time"

// ActivityComputeLPSnapshotsInput defines the input for the ComputeLPSnapshots activity.
// The activity computes LP position snapshots for a specific target date.
type ActivityComputeLPSnapshotsInput struct {
	TargetDate time.Time `json:"targetDate"` // The calendar date (UTC) to compute snapshots for
}

// ActivityComputeLPSnapshotsOutput contains the result of computing LP snapshots along with execution duration.
type ActivityComputeLPSnapshotsOutput struct {
	TargetDate     time.Time `json:"targetDate"`     // The calendar date (UTC) that was processed
	TotalSnapshots int       `json:"totalSnapshots"` // Number of position snapshots computed
	DurationMs     float64   `json:"durationMs"`     // Execution time in milliseconds
}

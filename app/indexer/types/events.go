package types

// ActivityIndexEventsOutput contains the number of indexed events along with execution duration.
type ActivityIndexEventsOutput struct {
	NumEvents         uint32            `json:"numEvents"`         // Number of events indexed
	EventCountsByType map[string]uint32 `json:"eventCountsByType"` // Count per type: {"reward": 100, "dex-swap": 5}
	DurationMs        float64           `json:"durationMs"`        // Execution time in milliseconds
}

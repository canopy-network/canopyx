package types

// ProgressResponse represents indexing progress at a point in time
type ProgressResponse struct {
	Timestamp string  `json:"timestamp"`
	Height    uint64  `json:"height"`
	Rate      float64 `json:"rate"`
}

type ResponsePoint struct {
	Time              string  `json:"time"`
	Height            uint64  `json:"height"`
	AvgLatency        float64 `json:"avg_latency"`         // seconds
	AvgProcessingTime float64 `json:"avg_processing_time"` // milliseconds
	BlocksIndexed     uint64  `json:"blocks_indexed"`
	Velocity          float64 `json:"velocity"` // blocks per minute
}

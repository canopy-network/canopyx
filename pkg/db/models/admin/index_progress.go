package admin

import (
	"time"
)

// IndexProgress Raw indexing progress (one row per indexed height).
type IndexProgress struct {
	ChainID      uint64    `ch:"chain_id"`
	Height       uint64    `ch:"height"`
	IndexedAt    time.Time `ch:"indexed_at"`
	IndexingTime float64   `ch:"indexing_time"` // Time in seconds from block creation to indexing completion

	// Workflow execution timing fields
	IndexingTimeMs float64 `ch:"indexing_time_ms"` // Total indexing time in milliseconds (actual processing time)
	IndexingDetail string  `ch:"indexing_detail"`  // JSON string with breakdown of individual activity timings
}

type ProgressPoint struct {
	TimeBucket        time.Time `ch:"time_bucket"`
	MaxHeight         uint64    `ch:"max_height"`
	AvgLatency        float64   `ch:"avg_latency"`
	AvgProcessingTime float64   `ch:"avg_processing_time"`
	BlocksIndexed     uint64    `ch:"blocks_indexed"`
}

package types

import (
	"time"

	"github.com/puzpuzpuz/xsync/v4"
)

type CachedQueueStats struct {
	OpsQueue        QueueStatus
	LiveQueue       QueueStatus // NEW: Live queue stats (blocks within a threshold)
	HistoricalQueue QueueStatus // NEW: Historical queue stats (blocks beyond a threshold)
	Fetched         time.Time
}

// NewQueueStatsCache creates a new queue stats cache
func NewQueueStatsCache() *xsync.Map[string, CachedQueueStats] {
	return xsync.NewMap[string, CachedQueueStats]()
}

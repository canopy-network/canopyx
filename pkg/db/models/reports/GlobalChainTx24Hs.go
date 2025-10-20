package reports

import (
	"time"
)

type ChainTx24h struct {
	ChainID string    `ch:"chain_id"`
	AsOf    time.Time `ch:"asof"`
	Count   uint64    `ch:"count"`
	// ReplacingMergeTree(version) needs the version column present:
	Version uint64 `ch:"version"`
}

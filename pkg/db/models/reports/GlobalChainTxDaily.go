package reports

import (
	"time"
)

type ChainTxDaily struct {
	ChainID string    `ch:"chain_id"`
	Day     time.Time `ch:"day"` // Note: Date (not DateTime)
	Count   uint64    `ch:"count"`
	Version uint64    `ch:"version"`
}

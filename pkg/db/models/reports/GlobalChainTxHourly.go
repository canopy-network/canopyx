package reports

import (
	"time"
)

type ChainTxHourly struct {
	ChainID string    `ch:"chain_id"`
	Hour    time.Time `ch:"hour"`
	Count   uint64    `ch:"count"`
	Version uint64    `ch:"version"`
}

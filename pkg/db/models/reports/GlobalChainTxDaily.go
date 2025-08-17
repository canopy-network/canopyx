package reports

import (
	"time"

	"github.com/uptrace/go-clickhouse/ch"
)

type ChainTxDaily struct {
	ch.CHModel `ch:"table:chain_tx_daily"`

	ChainID string    `ch:"chain_id,type:String"`
	Day     time.Time `ch:"day,type:Date"` // Note: Date (not DateTime)
	Count   uint64    `ch:"count,type:UInt64"`
	Version uint64    `ch:"version,type:UInt64,default:toUInt64(now64())"`
}

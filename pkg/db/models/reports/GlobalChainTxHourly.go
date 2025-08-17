package reports

import (
	"time"

	"github.com/uptrace/go-clickhouse/ch"
)

type ChainTxHourly struct {
	ch.CHModel `ch:"table:chain_tx_hourly"`

	ChainID string    `ch:"chain_id,type:String"`
	Hour    time.Time `ch:"hour,type:DateTime"`
	Count   uint64    `ch:"count,type:UInt64"`
	Version uint64    `ch:"version,type:UInt64,default:toUInt64(now64())"`
}

package reports

import (
	"time"

	"github.com/uptrace/go-clickhouse/ch"
)

type ChainTx24h struct {
	ch.CHModel `ch:"table:chain_tx_24h"`

	ChainID string    `ch:"chain_id,type:String"`
	AsOf    time.Time `ch:"asof,type:DateTime"`
	Count   uint64    `ch:"count,type:UInt64"`
	// ReplacingMergeTree(version) needs the version column present:
	Version uint64 `ch:"version,type:UInt64,default:toUInt64(now64())"`
}

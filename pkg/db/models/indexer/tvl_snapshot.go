package indexer

import "time"

const TVLSnapshotsProductionTableName = "tvl_snapshots"
const TVLSnapshotsUpdatedAtColumnName = "updated_at"

// TVLSnapshotColumns defines the schema for hourly TVL snapshots.
// Stores pre-computed TVL per chain at hourly intervals for fast historical queries.
//
// Compression strategy:
// - Delta,ZSTD(1) for chain IDs (low cardinality, sequential)
// - DoubleDelta,ZSTD(1) for timestamps and heights (monotonic)
// - Delta,ZSTD(3) for TVL values (gradual changes, large numbers)
var TVLSnapshotColumns = []ColumnDef{
	// Identity
	{Name: "chain_id", Type: "UInt64", Codec: "Delta, ZSTD(1)"},
	{Name: "snapshot_hour", Type: "DateTime", Codec: "DoubleDelta, ZSTD(1)"}, // Truncated to hour

	// Snapshot State
	{Name: "snapshot_height", Type: "UInt64", Codec: "DoubleDelta, ZSTD(1)"},

	// TVL Components (allows drill-down queries)
	{Name: "accounts_tvl", Type: "UInt64", Codec: "Delta, ZSTD(3)"},
	{Name: "pools_tvl", Type: "UInt64", Codec: "Delta, ZSTD(3)"},
	{Name: "validators_tvl", Type: "UInt64", Codec: "Delta, ZSTD(3)"},
	{Name: "orders_tvl", Type: "UInt64", Codec: "Delta, ZSTD(3)"},
	{Name: "dex_orders_tvl", Type: "UInt64", Codec: "Delta, ZSTD(3)"},
	{Name: "total_tvl", Type: "UInt64", Codec: "Delta, ZSTD(3)"}, // Sum of above

	// Metadata
	{Name: "computed_at", Type: "DateTime64(6)", Codec: "DoubleDelta, ZSTD(1)"},
	{Name: TVLSnapshotsUpdatedAtColumnName, Type: "DateTime64(6)", Codec: "DoubleDelta, ZSTD(1)"},
}

// TVLSnapshot stores hourly TVL snapshots per chain.
// Each row represents the TVL state for one chain at one hour boundary.
//
// ARCHITECTURAL NOTES:
// - Hourly granularity: Snapshots at toStartOfHour(height_time)
// - Cross-chain storage: canopyx_cross_chain.tvl_snapshots_global
// - Data source: Per-chain tables (accounts, pools, validators, orders, dex_orders)
// - ReplacingMergeTree: Deduplicates by updated_at (allows recomputing same hour)
// - Scheduled workflow: Runs hourly, computes snapshot for completed hour
// - Backfill support: Can recompute any historical hour
//
// Query patterns:
//   - TVL history: SELECT snapshot_hour, SUM(total_tvl) FROM tvl_snapshots_global WHERE snapshot_hour BETWEEN ? AND ? GROUP BY snapshot_hour
//   - Per-chain history: SELECT * FROM tvl_snapshots_global WHERE chain_id = ? ORDER BY snapshot_hour DESC
//   - Component breakdown: SELECT snapshot_hour, accounts_tvl, pools_tvl, ... WHERE chain_id = ?
//   - Latest snapshot: SELECT * FROM tvl_snapshots_global FINAL WHERE chain_id = ? ORDER BY snapshot_hour DESC LIMIT 1
type TVLSnapshot struct {
	ChainID      uint64    `ch:"chain_id" json:"chain_id"`
	SnapshotHour time.Time `ch:"snapshot_hour" json:"snapshot_hour"`

	// Snapshot State
	SnapshotHeight uint64 `ch:"snapshot_height" json:"snapshot_height"`

	// TVL Components
	AccountsTVL   uint64 `ch:"accounts_tvl" json:"accounts_tvl"`
	PoolsTVL      uint64 `ch:"pools_tvl" json:"pools_tvl"`
	ValidatorsTVL uint64 `ch:"validators_tvl" json:"validators_tvl"`
	OrdersTVL     uint64 `ch:"orders_tvl" json:"orders_tvl"`
	DexOrdersTVL  uint64 `ch:"dex_orders_tvl" json:"dex_orders_tvl"`
	TotalTVL      uint64 `ch:"total_tvl" json:"total_tvl"`

	// Metadata
	ComputedAt time.Time `ch:"computed_at" json:"computed_at"`
	UpdatedAt  time.Time `ch:"updated_at" json:"updated_at"`
}

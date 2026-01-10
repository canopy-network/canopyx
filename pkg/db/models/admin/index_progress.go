package admin

import (
	"time"
)

const IndexProgressTableName = "index_progress"
const IndexProgressAggTableName = "index_progress_agg"
const IndexProgressMvTableName = "index_progress_mv"

// IndexProgressColumns defines the schema for the index_progress table.
// Uses individual columns for timing metrics instead of JSON for ClickHouse column efficiency.
var IndexProgressColumns = []ColumnDef{
	{Name: "chain_id", Type: "UInt64"},
	{Name: "height", Type: "UInt64"},
	{Name: "indexed_at", Type: "DateTime64(6)"},
	{Name: "indexing_time", Type: "Float64"},
	{Name: "indexing_time_ms", Type: "Float64"},
	// Individual timing columns (milliseconds) - replacing JSON indexing_detail
	{Name: "timing_fetch_block_ms", Type: "Float64 DEFAULT 0"},
	{Name: "timing_prepare_index_ms", Type: "Float64 DEFAULT 0"},
	{Name: "timing_index_accounts_ms", Type: "Float64 DEFAULT 0"},
	{Name: "timing_index_committees_ms", Type: "Float64 DEFAULT 0"},
	{Name: "timing_index_dex_batch_ms", Type: "Float64 DEFAULT 0"},
	{Name: "timing_index_dex_prices_ms", Type: "Float64 DEFAULT 0"},
	{Name: "timing_index_events_ms", Type: "Float64 DEFAULT 0"},
	{Name: "timing_index_orders_ms", Type: "Float64 DEFAULT 0"},
	{Name: "timing_index_params_ms", Type: "Float64 DEFAULT 0"},
	{Name: "timing_index_pools_ms", Type: "Float64 DEFAULT 0"},
	{Name: "timing_index_supply_ms", Type: "Float64 DEFAULT 0"},
	{Name: "timing_index_transactions_ms", Type: "Float64 DEFAULT 0"},
	{Name: "timing_index_validators_ms", Type: "Float64 DEFAULT 0"},
	{Name: "timing_save_block_ms", Type: "Float64 DEFAULT 0"},
	{Name: "timing_save_block_summary_ms", Type: "Float64 DEFAULT 0"},
}

// IndexProgress Raw indexing progress (one row per indexed height).
type IndexProgress struct {
	ChainID      uint64    `ch:"chain_id"`
	Height       uint64    `ch:"height"`
	IndexedAt    time.Time `ch:"indexed_at"`
	IndexingTime float64   `ch:"indexing_time"` // Time in seconds from block creation to indexing completion

	// Workflow execution timing fields
	IndexingTimeMs float64 `ch:"indexing_time_ms"` // Total indexing time in milliseconds (actual processing time)

	// Individual timing metrics (milliseconds) - columnar for ClickHouse efficiency
	TimingFetchBlockMs        float64 `ch:"timing_fetch_block_ms"`
	TimingPrepareIndexMs      float64 `ch:"timing_prepare_index_ms"`
	TimingIndexAccountsMs     float64 `ch:"timing_index_accounts_ms"`
	TimingIndexCommitteesMs   float64 `ch:"timing_index_committees_ms"`
	TimingIndexDexBatchMs     float64 `ch:"timing_index_dex_batch_ms"`
	TimingIndexDexPricesMs    float64 `ch:"timing_index_dex_prices_ms"`
	TimingIndexEventsMs       float64 `ch:"timing_index_events_ms"`
	TimingIndexOrdersMs       float64 `ch:"timing_index_orders_ms"`
	TimingIndexParamsMs       float64 `ch:"timing_index_params_ms"`
	TimingIndexPoolsMs        float64 `ch:"timing_index_pools_ms"`
	TimingIndexSupplyMs       float64 `ch:"timing_index_supply_ms"`
	TimingIndexTransactionsMs float64 `ch:"timing_index_transactions_ms"`
	TimingIndexValidatorsMs   float64 `ch:"timing_index_validators_ms"`
	TimingSaveBlockMs         float64 `ch:"timing_save_block_ms"`
	TimingSaveBlockSummaryMs  float64 `ch:"timing_save_block_summary_ms"`
}

type ProgressPoint struct {
	TimeBucket     time.Time `ch:"time_bucket"`
	MaxHeight      uint64    `ch:"max_height"`
	AvgTotalTimeMs float64   `ch:"avg_total_time_ms"` // Average total workflow time in milliseconds
	BlocksIndexed  uint64    `ch:"blocks_indexed"`

	// Individual timing averages (milliseconds)
	AvgFetchBlockMs        float64 `ch:"avg_fetch_block_ms"`
	AvgPrepareIndexMs      float64 `ch:"avg_prepare_index_ms"`
	AvgIndexAccountsMs     float64 `ch:"avg_index_accounts_ms"`
	AvgIndexCommitteesMs   float64 `ch:"avg_index_committees_ms"`
	AvgIndexDexBatchMs     float64 `ch:"avg_index_dex_batch_ms"`
	AvgIndexDexPricesMs    float64 `ch:"avg_index_dex_prices_ms"`
	AvgIndexEventsMs       float64 `ch:"avg_index_events_ms"`
	AvgIndexOrdersMs       float64 `ch:"avg_index_orders_ms"`
	AvgIndexParamsMs       float64 `ch:"avg_index_params_ms"`
	AvgIndexPoolsMs        float64 `ch:"avg_index_pools_ms"`
	AvgIndexSupplyMs       float64 `ch:"avg_index_supply_ms"`
	AvgIndexTransactionsMs float64 `ch:"avg_index_transactions_ms"`
	AvgIndexValidatorsMs   float64 `ch:"avg_index_validators_ms"`
	AvgSaveBlockMs         float64 `ch:"avg_save_block_ms"`
	AvgSaveBlockSummaryMs  float64 `ch:"avg_save_block_summary_ms"`
}

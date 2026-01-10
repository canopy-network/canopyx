package types

// ProgressResponse represents indexing progress at a point in time
type ProgressResponse struct {
	Timestamp string  `json:"timestamp"`
	Height    uint64  `json:"height"`
	Rate      float64 `json:"rate"`
}

type ResponsePoint struct {
	Time           string  `json:"time"`
	Height         uint64  `json:"height"`
	AvgTotalTimeMs float64 `json:"avg_total_time_ms"` // Total workflow time in milliseconds
	BlocksIndexed  uint64  `json:"blocks_indexed"`
	Velocity       float64 `json:"velocity"` // blocks per minute

	// Individual timing averages (milliseconds)
	AvgFetchBlockMs        float64 `json:"avg_fetch_block_ms"`
	AvgPrepareIndexMs      float64 `json:"avg_prepare_index_ms"`
	AvgIndexAccountsMs     float64 `json:"avg_index_accounts_ms"`
	AvgIndexCommitteesMs   float64 `json:"avg_index_committees_ms"`
	AvgIndexDexBatchMs     float64 `json:"avg_index_dex_batch_ms"`
	AvgIndexDexPricesMs    float64 `json:"avg_index_dex_prices_ms"`
	AvgIndexEventsMs       float64 `json:"avg_index_events_ms"`
	AvgIndexOrdersMs       float64 `json:"avg_index_orders_ms"`
	AvgIndexParamsMs       float64 `json:"avg_index_params_ms"`
	AvgIndexPoolsMs        float64 `json:"avg_index_pools_ms"`
	AvgIndexSupplyMs       float64 `json:"avg_index_supply_ms"`
	AvgIndexTransactionsMs float64 `json:"avg_index_transactions_ms"`
	AvgIndexValidatorsMs   float64 `json:"avg_index_validators_ms"`
	AvgSaveBlockMs         float64 `json:"avg_save_block_ms"`
	AvgSaveBlockSummaryMs  float64 `json:"avg_save_block_summary_ms"`
}

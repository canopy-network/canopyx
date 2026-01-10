package admin

import (
	"context"
	"fmt"

	adminmodels "github.com/canopy-network/canopyx/pkg/db/models/admin"
	"go.uber.org/zap"
)

// IndexProgressHistory returns index progress metrics grouped by time intervals.
//
// PREWHERE Optimization:
// Uses PREWHERE for indexed_at time filter because:
// - indexed_at is DateTime64 (8 bytes) vs many columns including String (indexing_detail)
// - Not in ORDER BY (chain_id, height), so primary index doesn't help for time filtering
// - Highly selective: filters to last X hours from potentially millions of rows
// See: https://clickhouse.com/resources/engineering/clickhouse-query-optimisation-definitive-guide#optimisation-6-filter-earlier-with-prewhere
func (db *DB) IndexProgressHistory(ctx context.Context, chainID uint64, hours, intervalSeconds int) ([]adminmodels.ProgressPoint, error) {
	query := fmt.Sprintf(`
        SELECT
            toStartOfInterval(indexed_at, INTERVAL %d SECOND) AS time_bucket,
            max(height) AS max_height,
            avg(indexing_time_ms) AS avg_total_time_ms,
            countDistinct(height) AS blocks_indexed,
            avg(timing_fetch_block_ms) AS avg_fetch_block_ms,
            avg(timing_prepare_index_ms) AS avg_prepare_index_ms,
            avg(timing_index_accounts_ms) AS avg_index_accounts_ms,
            avg(timing_index_committees_ms) AS avg_index_committees_ms,
            avg(timing_index_dex_batch_ms) AS avg_index_dex_batch_ms,
            avg(timing_index_dex_prices_ms) AS avg_index_dex_prices_ms,
            avg(timing_index_events_ms) AS avg_index_events_ms,
            avg(timing_index_orders_ms) AS avg_index_orders_ms,
            avg(timing_index_params_ms) AS avg_index_params_ms,
            avg(timing_index_pools_ms) AS avg_index_pools_ms,
            avg(timing_index_supply_ms) AS avg_index_supply_ms,
            avg(timing_index_transactions_ms) AS avg_index_transactions_ms,
            avg(timing_index_validators_ms) AS avg_index_validators_ms,
            avg(timing_save_block_ms) AS avg_save_block_ms,
            avg(timing_save_block_summary_ms) AS avg_save_block_summary_ms
        FROM "%s"."%s"
        PREWHERE indexed_at >= now() - INTERVAL %d HOUR
        WHERE chain_id = ?
        GROUP BY time_bucket
        ORDER BY time_bucket ASC
    `, intervalSeconds, db.Name, adminmodels.IndexProgressTableName, hours)

	var points []adminmodels.ProgressPoint
	if err := db.Select(ctx, &points, query, chainID); err != nil {
		db.Logger.Error("query index progress history failed", zap.Uint64("chain_id", chainID), zap.Error(err))
		return nil, err
	}

	return points, nil
}

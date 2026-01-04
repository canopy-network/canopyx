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
func (db *DB) IndexProgressHistory(ctx context.Context, chainID uint64, hours, intervalMinutes int) ([]adminmodels.ProgressPoint, error) {
	query := fmt.Sprintf(`
        SELECT
            toStartOfInterval(indexed_at, INTERVAL %d MINUTE) AS time_bucket,
            max(height) AS max_height,
            avg(indexing_time) AS avg_latency,
            avg(indexing_time_ms) AS avg_processing_time,
            count() AS blocks_indexed
        FROM "%s"."%s"
        PREWHERE indexed_at >= now() - INTERVAL %d HOUR
        WHERE chain_id = ?
        GROUP BY time_bucket
        ORDER BY time_bucket ASC
    `, intervalMinutes, db.Name, adminmodels.IndexProgressTableName, hours)

	var points []adminmodels.ProgressPoint
	if err := db.Select(ctx, &points, query, chainID); err != nil {
		db.Logger.Error("query index progress history failed", zap.Uint64("chain_id", chainID), zap.Error(err))
		return nil, err
	}

	return points, nil
}

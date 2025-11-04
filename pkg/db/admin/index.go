package admin

import (
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

func (db *DB) IndexProgressHistory(ctx context.Context, chainID uint64, hours, intervalMinutes int) ([]admin.ProgressPoint, error) {
	query := fmt.Sprintf(`
        SELECT
            toStartOfInterval(indexed_at, INTERVAL %d MINUTE) AS time_bucket,
            max(height) AS max_height,
            avg(indexing_time) AS avg_latency,
            avg(indexing_time_ms) AS avg_processing_time,
            count() AS blocks_indexed
        FROM "%s"."index_progress"
        WHERE chain_id = ?
          AND indexed_at >= now() - INTERVAL %d HOUR
        GROUP BY time_bucket
        ORDER BY time_bucket ASC
    `, intervalMinutes, db.Name, hours)

	var points []admin.ProgressPoint
	if err := db.Select(ctx, &points, query, chainID); err != nil {
		db.Logger.Error("query index progress history failed", zap.Uint64("chain_id", chainID), zap.Error(err))
		return nil, err
	}

	return points, nil
}

package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// IndexProgress Raw indexing progress (one row per indexed height).
type IndexProgress struct {
	ChainID      string    `ch:"chain_id"`
	Height       uint64    `ch:"height"`
	IndexedAt    time.Time `ch:"indexed_at"`
	IndexingTime float64   `ch:"indexing_time"` // Time in seconds from block creation to indexing completion

	// Workflow execution timing fields
	IndexingTimeMs float64 `ch:"indexing_time_ms"` // Total indexing time in milliseconds (actual processing time)
	IndexingDetail string  `ch:"indexing_detail"`  // JSON string with breakdown of individual activity timings
}

// InitIndexProgress initializes the index_progress table with its aggregation infrastructure.
// Creates:
// 1. Base table (MergeTree) - stores raw indexing progress
// 2. Aggregate table (AggregatingMergeTree) - stores aggregate state for max height per chain
// 3. Materialized view - automatically updates aggregate on inserts
func InitIndexProgress(ctx context.Context, db driver.Conn, dbName string) error {
	// 1) Base table: index_progress
	ddlBase := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."index_progress" (
			chain_id String,
			height UInt64,
			indexed_at DateTime64(6),
			indexing_time Float64,
			indexing_time_ms Float64,
			indexing_detail String
		) ENGINE = MergeTree()
		ORDER BY (chain_id, height)
	`, dbName)
	if err := db.Exec(ctx, ddlBase); err != nil {
		return fmt.Errorf("create index_progress table: %w", err)
	}

	// 2) Aggregate table (stores aggregate STATE), requires ORDER BY
	ddlAgg := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."index_progress_agg" (
			chain_id String,
			max_height AggregateFunction(max, UInt64)
		) ENGINE = AggregatingMergeTree()
		ORDER BY (chain_id)
	`, dbName)
	if err := db.Exec(ctx, ddlAgg); err != nil {
		return fmt.Errorf("create index_progress_agg table: %w", err)
	}

	// 3) Materialized view that updates the aggregate on every insert into base
	ddlMV := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."index_progress_mv"
		TO "%s"."index_progress_agg" AS
		SELECT
			chain_id,
			maxState(height) AS max_height
		FROM "%s"."index_progress"
		GROUP BY chain_id
	`, dbName, dbName, dbName)
	if err := db.Exec(ctx, ddlMV); err != nil {
		return fmt.Errorf("create index_progress_mv: %w", err)
	}

	return nil
}

// InsertIndexProgress inserts a new index progress record.
func InsertIndexProgress(ctx context.Context, db driver.Conn, ip *IndexProgress) error {
	query := `
		INSERT INTO index_progress (chain_id, height, indexed_at, indexing_time, indexing_time_ms, indexing_detail)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	return db.Exec(ctx, query,
		ip.ChainID,
		ip.Height,
		ip.IndexedAt,
		ip.IndexingTime,
		ip.IndexingTimeMs,
		ip.IndexingDetail,
	)
}

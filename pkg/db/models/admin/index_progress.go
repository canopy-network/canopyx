package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/go-clickhouse/ch"
)

// IndexProgress Raw indexing progress (one row per indexed height).
type IndexProgress struct {
	ch.CHModel `ch:"table:index_progress,engine:MergeTree(),order_by:(chain_id,height)"`

	ChainID      string    `ch:"chain_id"`
	Height       uint64    `ch:"height"`
	IndexedAt    time.Time `ch:"indexed_at,type:DateTime64(6)"`
	IndexingTime float64   `ch:"indexing_time,type:Float64"` // Time in seconds from block creation to indexing completion

	// Workflow execution timing fields
	IndexingTimeMs float64 `ch:"indexing_time_ms,type:Float64"` // Total indexing time in milliseconds (actual processing time)
	IndexingDetail string  `ch:"indexing_detail,type:String"`   // JSON string with breakdown of individual activity timings
}

// InitIndexProgress initializes the index_progress table.
func InitIndexProgress(ctx context.Context, db *ch.DB, dbName string) error {
	// 1) Base table via model/builder (includes ORDER BY)
	if _, err := db.NewCreateTable().
		Model((*IndexProgress)(nil)).
		IfNotExists().
		Order("chain_id,height").
		Exec(ctx); err != nil {
		return err
	}

	// 2) Aggregate table (stores aggregate STATE), requires ORDER BY
	ddlAgg := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS "%[1]s"."index_progress_agg" (
  chain_id   String,
  max_height AggregateFunction(max, UInt64)
) ENGINE = AggregatingMergeTree
ORDER BY (chain_id)
`, dbName)
	if _, err := db.ExecContext(ctx, ddlAgg); err != nil {
		return err
	}

	// 3) Materialized view that updates the aggregate on every insert into base
	ddlMV := fmt.Sprintf(`
CREATE MATERIALIZED VIEW IF NOT EXISTS "%[1]s"."index_progress_mv"
TO "%[1]s"."index_progress_agg" AS
SELECT
  chain_id,
  maxState(height) AS max_height
FROM "%[1]s"."index_progress"
GROUP BY chain_id
`, dbName)
	if _, err := db.ExecContext(ctx, ddlMV); err != nil {
		return err
	}

	return nil
}

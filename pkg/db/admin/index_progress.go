package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/admin"
)

// initIndexProgress initializes the index_progress table with its aggregation infrastructure.
// Creates:
// 1. Base table (MergeTree) - stores raw indexing progress
// 2. Aggregate table (AggregatingMergeTree) - stores aggregate state for max height per chain
// 3. Materialized view - automatically updates aggregate on inserts
func (db *DB) initIndexProgress(ctx context.Context) error {
	// 1) Base table: index_progress
	ddlBase := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."index_progress" (
			chain_id UInt64,
			height UInt64,
			indexed_at DateTime64(6),
			indexing_time Float64,
			indexing_time_ms Float64,
			indexing_detail String
		) ENGINE = MergeTree()
		ORDER BY (chain_id, height)
	`, db.Name)
	if err := db.Db.Exec(ctx, ddlBase); err != nil {
		return fmt.Errorf("create index_progress table: %w", err)
	}

	// 2) Aggregate table (stores aggregate STATE), requires ORDER BY
	ddlAgg := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."index_progress_agg" (
			chain_id UInt64,
			max_height AggregateFunction(max, UInt64)
		) ENGINE = AggregatingMergeTree()
		ORDER BY (chain_id)
	`, db.Name)
	if err := db.Db.Exec(ctx, ddlAgg); err != nil {
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
	`, db.Name, db.Name, db.Name)
	if err := db.Db.Exec(ctx, ddlMV); err != nil {
		return fmt.Errorf("create index_progress_mv: %w", err)
	}

	return nil
}

// RecordIndexed records the height of the last indexed block for the provided chain along with timing metrics.
// blockTime is the timestamp when the block was created.
// indexingTimeMs is the total activity execution time in milliseconds (actual processing time).
// indexingDetail is a JSON string with the breakdown of individual activity timings.
func (db *DB) RecordIndexed(ctx context.Context, chainID uint64, height uint64, blockTime time.Time, indexingTimeMs float64, indexingDetail string) error {
	now := time.Now().UTC()

	// Calculate end-to-end indexing latency (time from block creation to indexing completion)
	var indexingTime float64
	if !blockTime.IsZero() {
		indexingTime = now.Sub(blockTime).Seconds()
		// Handle edge case: if system clock is behind or block time is in future, set to 0
		if indexingTime < 0 {
			indexingTime = 0
		}
	}

	ip := &admin.IndexProgress{
		ChainID:        chainID,
		Height:         height,
		IndexedAt:      now,
		IndexingTime:   indexingTime,   // Time from block creation to indexing completion (seconds)
		IndexingTimeMs: indexingTimeMs, // Total activity execution time (milliseconds)
		IndexingDetail: indexingDetail, // JSON breakdown of individual activity timings
	}

	return db.insertIndexProgress(ctx, ip)
}

// insertIndexProgress inserts a new index progress record.
func (db *DB) insertIndexProgress(ctx context.Context, ip *admin.IndexProgress) error {
	query := `
		INSERT INTO index_progress (chain_id, height, indexed_at, indexing_time, indexing_time_ms, indexing_detail)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	return db.Db.Exec(ctx, query,
		ip.ChainID,
		ip.Height,
		ip.IndexedAt,
		ip.IndexingTime,
		ip.IndexingTimeMs,
		ip.IndexingDetail,
	)
}

// LastIndexed returns the latest indexed height for a chain.
// 1) Prefer the summarized ReplacingMergeTree table (index_progress_agg).
// 2) Fallback to max(height) from the raw index_progress if the summary is empty.
func (db *DB) LastIndexed(ctx context.Context, chainID uint64) (uint64, error) {
	// Try the aggregate first:
	var h uint64
	query := fmt.Sprintf(`SELECT maxMerge(max_height) FROM "%s"."index_progress_agg" WHERE chain_id = ?`, db.Name)
	err := db.Db.QueryRow(ctx, query, chainID).Scan(&h)

	if err == nil && h != 0 {
		return h, nil
	}

	// Fallback to the base table if agg is empty (e.g., very first rows)
	var fallback uint64
	fallbackQuery := fmt.Sprintf(`SELECT max(height) FROM "%s"."index_progress" WHERE chain_id = ?`, db.Name)
	if err := db.Db.QueryRow(ctx, fallbackQuery, chainID).Scan(&fallback); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	return fallback, nil
}

// FindGaps returns missing [From, To] heights strictly inside observed heights,
// and does NOT include the trailing gap to 'up to'. The caller should add a tail gap separately.
func (db *DB) FindGaps(ctx context.Context, chainID uint64) ([]admin.Gap, error) {
	query := fmt.Sprintf(`
		SELECT CAST(assumeNotNull(prev_h) + 1 AS UInt64) AS from_h, CAST(h - 1 AS UInt64) AS to_h
		FROM (
		  SELECT
		    height AS h,
		    lagInFrame(toNullable(height)) OVER (
		      PARTITION BY chain_id
		      ORDER BY height
		      ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
		    ) AS prev_h
		  FROM "%s"."index_progress"
		  WHERE chain_id = ?
		  ORDER BY height
		)
		WHERE prev_h IS NOT NULL AND h > prev_h + 1
		ORDER BY from_h
	`, db.Name)

	var rows []admin.Gap
	if err := db.Select(ctx, &rows, query, chainID); err != nil {
		return nil, err
	}

	return rows, nil
}

// GetAllChainIndexProgress retrieves the last indexed height for all chains.
// Uses aggregate table with fallback to the base table.
func (db *DB) GetAllChainIndexProgress(ctx context.Context) (map[string]uint64, error) {
	progressMap := make(map[string]uint64)

	// Try the aggregate table first
	query := fmt.Sprintf(`
		SELECT chain_id, maxMerge(max_height) AS last_idx
		FROM "%s"."index_progress_agg"
		GROUP BY chain_id
	`, db.Name)

	rows, err := db.Db.Query(ctx, query)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var chainID uint64
			var lastIdx uint64
			if err := rows.Scan(&chainID, &lastIdx); err != nil {
				return nil, err
			}
			progressMap[fmt.Sprintf("%d", chainID)] = lastIdx
		}
		if len(progressMap) > 0 {
			return progressMap, nil
		}
	}

	// Fallback to base table
	fallbackQuery := fmt.Sprintf(`
		SELECT chain_id, max(height) AS last_idx
		FROM "%s"."index_progress"
		GROUP BY chain_id
	`, db.Name)

	rows, err = db.Db.Query(ctx, fallbackQuery)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var chainID uint64
		var lastIdx uint64
		if err := rows.Scan(&chainID, &lastIdx); err != nil {
			return nil, err
		}
		progressMap[fmt.Sprintf("%d", chainID)] = lastIdx
	}

	return progressMap, nil
}

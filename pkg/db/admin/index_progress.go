package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	adminmodels "github.com/canopy-network/canopyx/pkg/db/models/admin"
)

// initIndexProgress initializes the index_progress table with its aggregation infrastructure.
// Creates:
// 1. Base table (ReplicatedMergeTree) - stores raw indexing progress
// 2. Aggregate table (ReplicatedAggregatingMergeTree) - stores aggregate state for max height per chain
// 3. Materialized view - automatically updates aggregate on inserts
// index_progress (MergeTree) -- All raw events
//
//	↓ (Materialized View)
//	index_progress_agg (AggregatingMergeTree) -- Max height per chain
func (db *DB) initIndexProgress(ctx context.Context) error {
	schemaSQL := adminmodels.ColumnsToSchemaSQL(adminmodels.IndexProgressColumns)
	baseEngine := clickhouse.ReplicatedEngine(clickhouse.MergeTree, "")
	aggEngine := clickhouse.ReplicatedEngine(clickhouse.AggregatingMergeTree, "")

	// Base table: index_progress
	ddlBase := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, height)
	`, db.Name, adminmodels.IndexProgressTableName, schemaSQL, baseEngine)
	if err := db.Db.Exec(ctx, ddlBase); err != nil {
		return fmt.Errorf("create index_progress table: %w", err)
	}

	// Aggregate table (stores aggregate STATE), requires ORDER BY
	ddlAgg := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			chain_id UInt64,
			max_height AggregateFunction(max, UInt64)
		) ENGINE = %s
		ORDER BY (chain_id)
	`, db.Name, adminmodels.IndexProgressAggTableName, aggEngine)
	if err := db.Db.Exec(ctx, ddlAgg); err != nil {
		return fmt.Errorf("create index_progress_agg table: %w", err)
	}

	// Materialized view that updates the aggregate on every insert into base
	ddlMV := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."%s"
		TO "%s"."%s" AS
		SELECT
			chain_id,
			maxState(height) AS max_height
		FROM "%s"."%s"
		GROUP BY chain_id
	`,
		db.Name, adminmodels.IndexProgressMvTableName,
		db.Name, adminmodels.IndexProgressAggTableName,
		db.Name, adminmodels.IndexProgressTableName,
	)
	if err := db.Db.Exec(ctx, ddlMV); err != nil {
		return fmt.Errorf("create index_progress_mv: %w", err)
	}

	return nil
}

// RecordIndexed records the height of the last indexed block for the provided chain along with timing metrics.
// indexingTimeMs is the total workflow execution time in milliseconds (from workflow start to completion).
// indexingDetail is a JSON string with the breakdown of individual activity timings.
func (db *DB) RecordIndexed(ctx context.Context, chainID uint64, height uint64, indexingTimeMs float64, indexingDetail string) error {
	ip := &adminmodels.IndexProgress{
		ChainID:        chainID,
		Height:         height,
		IndexedAt:      time.Now().UTC(),
		IndexingTime:   indexingTimeMs / 1000.0, // Convert ms to seconds for backwards compatibility
		IndexingTimeMs: indexingTimeMs,          // Total workflow execution time (milliseconds)
		IndexingDetail: indexingDetail,          // JSON breakdown of individual activity timings
	}

	return db.insertIndexProgress(ctx, ip)
}

// insertIndexProgress inserts a new index progress record.
func (db *DB) insertIndexProgress(ctx context.Context, ip *adminmodels.IndexProgress) error {
	query := fmt.Sprintf(`
		INSERT INTO "%s"."%s" (chain_id, height, indexed_at, indexing_time, indexing_time_ms, indexing_detail)
		VALUES (?, ?, ?, ?, ?, ?)
	`, db.Name, adminmodels.IndexProgressTableName)

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
	query := fmt.Sprintf(
		`SELECT maxMerge(max_height) FROM "%s"."%s" WHERE chain_id = ?`,
		db.Name,
		adminmodels.IndexProgressAggTableName,
	)
	err := db.Db.QueryRow(ctx, query, chainID).Scan(&h)

	if err == nil && h != 0 {
		return h, nil
	}

	// Fallback to the base table if agg is empty (e.g., very first rows)
	var fallback uint64
	fallbackQuery := fmt.Sprintf(
		`SELECT max(height) FROM "%s"."%s" WHERE chain_id = ?`,
		db.Name,
		adminmodels.IndexProgressTableName,
	)
	if err := db.Db.QueryRow(ctx, fallbackQuery, chainID).Scan(&fallback); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	return fallback, nil
}

// FindGaps returns missing [From, To] heights strictly inside observed heights,
// and does NOT include the trailing gap to 'up to'. The caller should add a tail gap separately.
func (db *DB) FindGaps(ctx context.Context, chainID uint64) ([]adminmodels.Gap, error) {
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
		  FROM "%s"."%s"
		  WHERE chain_id = ?
		  ORDER BY height
		)
		WHERE prev_h IS NOT NULL AND h > prev_h + 1
		ORDER BY from_h
	`, db.Name, adminmodels.IndexProgressTableName)

	var rows []adminmodels.Gap
	if err := db.Select(ctx, &rows, query, chainID); err != nil {
		return nil, err
	}

	return rows, nil
}

// chainProgress is a helper struct for scanning chain index progress results.
type chainProgress struct {
	ChainID uint64 `ch:"chain_id"`
	LastIdx uint64 `ch:"last_idx"`
}

// GetAllChainIndexProgress retrieves the last indexed height for all chains.
// Uses aggregate table with fallback to the base table.
func (db *DB) GetAllChainIndexProgress(ctx context.Context) (map[string]uint64, error) {
	progressMap := make(map[string]uint64)

	// Try the aggregate table first
	query := fmt.Sprintf(`
		SELECT chain_id, maxMerge(max_height) AS last_idx
		FROM "%s"."%s"
		GROUP BY chain_id
	`, db.Name, adminmodels.IndexProgressAggTableName)

	var results []chainProgress
	if err := db.Db.Select(ctx, &results, query); err == nil && len(results) > 0 {
		for _, r := range results {
			progressMap[fmt.Sprintf("%d", r.ChainID)] = r.LastIdx
		}
		return progressMap, nil
	}

	// Fallback to base table
	fallbackQuery := fmt.Sprintf(`
		SELECT chain_id, max(height) AS last_idx
		FROM "%s"."%s"
		GROUP BY chain_id
	`, db.Name, adminmodels.IndexProgressTableName)

	var fallbackResults []chainProgress
	if err := db.Db.Select(ctx, &fallbackResults, fallbackQuery); err != nil {
		return nil, err
	}

	for _, r := range fallbackResults {
		progressMap[fmt.Sprintf("%d", r.ChainID)] = r.LastIdx
	}

	return progressMap, nil
}

// heightRow is a helper struct for scanning height values from ClickHouse.
// clickhouse-go v2 requires struct destinations for Select.
type heightRow struct {
	Height uint64 `ch:"height"`
}

// GetCleanableHeights returns heights that have been indexed and are safe to clean from staging.
// Returns heights indexed within the last lookbackHours - these have been promoted to production.
//
// Example: With lookbackHours=2, at 4pm returns heights indexed between 2pm and 4pm.
// These heights are in index_progress = successfully indexed = staging can be cleaned.
func (db *DB) GetCleanableHeights(ctx context.Context, chainID uint64, lookbackHours int) ([]uint64, error) {
	query := fmt.Sprintf(`
		SELECT DISTINCT height
		FROM "%s"."%s"
		WHERE chain_id = ?
		  AND indexed_at >= now() - INTERVAL ? HOUR
		ORDER BY height
	`, db.Name, adminmodels.IndexProgressTableName)

	var rows []heightRow
	if err := db.Db.Select(ctx, &rows, query, chainID, lookbackHours); err != nil {
		return nil, fmt.Errorf("get cleanable heights for chain %d: %w", chainID, err)
	}

	// Extract heights from rows
	heights := make([]uint64, len(rows))
	for i, r := range rows {
		heights[i] = r.Height
	}

	return heights, nil
}

// DeleteIndexProgressForChain removes all index progress records for a chain.
func (db *DB) DeleteIndexProgressForChain(ctx context.Context, chainID uint64) error {
	baseQuery := fmt.Sprintf(
		`DELETE FROM "%s"."%s" WHERE chain_id = ?`,
		db.Name,
		adminmodels.IndexProgressTableName,
	)
	if err := db.Db.Exec(ctx, baseQuery, chainID); err != nil {
		return fmt.Errorf("delete index_progress for chain %d: %w", chainID, err)
	}

	aggQuery := fmt.Sprintf(
		`DELETE FROM "%s"."%s" WHERE chain_id = ?`,
		db.Name,
		adminmodels.IndexProgressAggTableName,
	)
	if err := db.Db.Exec(ctx, aggQuery, chainID); err != nil {
		return fmt.Errorf("delete index_progress_agg for chain %d: %w", chainID, err)
	}

	return nil
}

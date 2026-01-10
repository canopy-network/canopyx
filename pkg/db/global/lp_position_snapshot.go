package global

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initLPPositionSnapshots creates the lp_position_snapshots table with ReplacingMergeTree engine.
// Uses updated_at as the deduplication version key (allows recomputing "today" multiple times).
// The table stores daily snapshots of liquidity provider positions across all chains.
//
// ARCHITECTURAL NOTE: Unlike other tables, LP snapshots are NOT synced via
// materialized views. They are computed independently by a scheduled workflow that:
// 1. Queries pool_points_by_holder table at snapshot height
// 2. Computes snapshots with lifecycle tracking
// 3. Writes directly to this table
func (db *DB) initLPPositionSnapshots(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.LPPositionSnapshotColumns)
	tableName := indexermodels.LPPositionSnapshotsProductionTableName

	// Create table with ReplacingMergeTree(updated_at) for idempotent updates
	// ORDER BY: pool_id before address (lower cardinality first)
	// Bloom filter on address for fast lookups by LP holder
	query := fmt.Sprintf(
		`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s,
			INDEX address_bloom address TYPE bloom_filter GRANULARITY 4
		) ENGINE = %s
		ORDER BY (source_chain_id, pool_id, address, snapshot_date)
	`,
		db.Name,
		tableName,
		schemaSQL,
		clickhouse.ReplicatedEngine(
			clickhouse.ReplacingMergeTree,
			indexermodels.LPPositionSnapshotsUpdateAtColumnName,
		),
	)

	if err := db.Exec(ctx, query); err != nil {
		return fmt.Errorf("create %s: %w", tableName, err)
	}

	return nil
}

// InsertLPPositionSnapshots inserts LP position snapshots directly into the production table.
// Uses ReplacingMergeTree deduplication to allow recomputing "today" multiple times.
func (db *DB) InsertLPPositionSnapshots(ctx context.Context, snapshots []*indexermodels.LPPositionSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}

	tableName := indexermodels.LPPositionSnapshotsProductionTableName
	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		source_chain_id, address, pool_id,
		snapshot_date, snapshot_height, snapshot_balance, pool_share_percentage,
		position_created_date, position_closed_date, is_position_active,
		computed_at, updated_at
	) VALUES`, db.Name, tableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return fmt.Errorf("prepare batch for %s: %w", tableName, err)
	}
	// Ensure the batch is closed, especially if not all data is sent immediately
	defer func() { _ = batch.Close() }()

	for _, snapshot := range snapshots {
		err = batch.Append(
			snapshot.SourceChainID,
			snapshot.Address,
			snapshot.PoolID,
			snapshot.SnapshotDate,
			snapshot.SnapshotHeight,
			snapshot.SnapshotBalance,
			snapshot.PoolSharePercentage,
			snapshot.PositionCreatedDate,
			snapshot.PositionClosedDate,
			snapshot.IsPositionActive,
			snapshot.ComputedAt,
			snapshot.UpdatedAt,
		)
		if err != nil {
			_ = batch.Abort()
			return fmt.Errorf("append snapshot to batch: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("send batch for %s: %w", tableName, err)
	}

	return nil
}

// LPPositionSnapshotQueryParams defines filtering criteria for querying LP position snapshots.
type LPPositionSnapshotQueryParams struct {
	SourceChainID *uint64    // Filter by source chain (optional)
	Address       *string    // Filter by LP holder address (optional)
	PoolID        *uint64    // Filter by pool ID (optional)
	StartDate     *time.Time // Filter snapshots >= start date (optional)
	EndDate       *time.Time // Filter snapshots <= end date (optional)
	ActiveOnly    bool       // Only return active positions if true
	Limit         int        // Max number of results (0 = unlimited)
	Offset        int        // Offset for pagination
}

// QueryLPPositionSnapshots retrieves LP position snapshots based on filter criteria.
// Returns snapshots ordered by (source_chain_id, pool_id, address, snapshot_date DESC).
// Uses FINAL to ensure ReplacingMergeTree deduplication is applied.
func (db *DB) QueryLPPositionSnapshots(ctx context.Context, params LPPositionSnapshotQueryParams) ([]*indexermodels.LPPositionSnapshot, error) {
	tableName := indexermodels.LPPositionSnapshotsProductionTableName
	baseQuery := fmt.Sprintf(`
		SELECT
			source_chain_id, address, pool_id,
			snapshot_date, snapshot_height, snapshot_balance, pool_share_percentage,
			position_created_date, position_closed_date, is_position_active,
			computed_at, updated_at
		FROM "%s"."%s" FINAL
	`, db.Name, tableName)

	// Build PREWHERE clause for is_position_active (highly selective, small column)
	var prewhereClause string
	if params.ActiveOnly {
		prewhereClause = "PREWHERE is_position_active = 1"
	}

	// Build WHERE clause dynamically for other filters
	var whereClauses []string
	var args []interface{}

	if params.SourceChainID != nil {
		whereClauses = append(whereClauses, "source_chain_id = ?")
		args = append(args, *params.SourceChainID)
	}

	if params.Address != nil {
		whereClauses = append(whereClauses, "address = ?")
		args = append(args, *params.Address)
	}

	if params.PoolID != nil {
		whereClauses = append(whereClauses, "pool_id = ?")
		args = append(args, *params.PoolID)
	}

	if params.StartDate != nil {
		whereClauses = append(whereClauses, "snapshot_date >= ?")
		args = append(args, *params.StartDate)
	}

	if params.EndDate != nil {
		whereClauses = append(whereClauses, "snapshot_date <= ?")
		args = append(args, *params.EndDate)
	}

	// Add PREWHERE clause first (if present)
	if prewhereClause != "" {
		baseQuery += " " + prewhereClause
	}

	// Add WHERE clause if we have other filters
	if len(whereClauses) > 0 {
		baseQuery += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Add ORDER BY - matches table ORDER BY for optimal query performance
	baseQuery += " ORDER BY source_chain_id, pool_id, address, snapshot_date DESC"

	// Add LIMIT/OFFSET if specified
	if params.Limit > 0 {
		baseQuery += fmt.Sprintf(" LIMIT %d", params.Limit)
		if params.Offset > 0 {
			baseQuery += fmt.Sprintf(" OFFSET %d", params.Offset)
		}
	}

	rows, err := db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", tableName, err)
	}
	defer func() { _ = rows.Close() }()

	var snapshots []*indexermodels.LPPositionSnapshot
	for rows.Next() {
		snapshot := &indexermodels.LPPositionSnapshot{}
		err = rows.Scan(
			&snapshot.SourceChainID,
			&snapshot.Address,
			&snapshot.PoolID,
			&snapshot.SnapshotDate,
			&snapshot.SnapshotHeight,
			&snapshot.SnapshotBalance,
			&snapshot.PoolSharePercentage,
			&snapshot.PositionCreatedDate,
			&snapshot.PositionClosedDate,
			&snapshot.IsPositionActive,
			&snapshot.ComputedAt,
			&snapshot.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		snapshots = append(snapshots, snapshot)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return snapshots, nil
}

// GetLatestLPPositionSnapshot retrieves the most recent snapshot for a specific position.
// Returns nil if no snapshot exists for the given (source_chain_id, address, pool_id).
func (db *DB) GetLatestLPPositionSnapshot(ctx context.Context, sourceChainID uint64, address string, poolID uint64) (*indexermodels.LPPositionSnapshot, error) {
	tableName := indexermodels.LPPositionSnapshotsProductionTableName
	query := fmt.Sprintf(`
		SELECT
			source_chain_id, address, pool_id,
			snapshot_date, snapshot_height, snapshot_balance, pool_share_percentage,
			position_created_date, position_closed_date, is_position_active,
			computed_at, updated_at
		FROM "%s"."%s" FINAL
		WHERE source_chain_id = ? AND address = ? AND pool_id = ?
		ORDER BY snapshot_date DESC
		LIMIT 1
	`, db.Name, tableName)

	snapshot := &indexermodels.LPPositionSnapshot{}
	err := db.QueryRow(ctx, query, sourceChainID, address, poolID).Scan(
		&snapshot.SourceChainID,
		&snapshot.Address,
		&snapshot.PoolID,
		&snapshot.SnapshotDate,
		&snapshot.SnapshotHeight,
		&snapshot.SnapshotBalance,
		&snapshot.PoolSharePercentage,
		&snapshot.PositionCreatedDate,
		&snapshot.PositionClosedDate,
		&snapshot.IsPositionActive,
		&snapshot.ComputedAt,
		&snapshot.UpdatedAt,
	)

	if err != nil {
		// Return nil if no snapshot exists (not an error)
		if clickhouse.IsNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("query latest snapshot: %w", err)
	}

	return snapshot, nil
}

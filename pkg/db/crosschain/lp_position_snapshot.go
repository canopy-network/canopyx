package crosschain

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"go.uber.org/zap"
)

// initLPPositionSnapshots creates the lp_position_snapshots_global table with ReplacingMergeTree engine.
// Uses updated_at as the deduplication version key (allows recomputing "today" multiple times).
// The table stores daily snapshots of liquidity provider positions across all chains.
//
// Compression strategy:
// - Delta + ZSTD(1) for chain IDs and pool IDs (gradual changes)
// - ZSTD(1) for addresses
// - DoubleDelta + ZSTD(1) for monotonic heights
// - Delta + ZSTD(3) for balances and percentages (higher compression for larger values)
// - DoubleDelta + ZSTD(1) for timestamps
//
// ARCHITECTURAL NOTE: Unlike other cross-chain tables, LP snapshots are NOT synced via
// materialized views. They are computed independently by a scheduled workflow that:
// 1. Queries per-chain pool_points_by_holder tables at snapshot height
// 2. Computes snapshots with lifecycle tracking
// 3. Writes directly to this global table
//
// This prevents race conditions when indexing new chains (snapshots only created after indexing completes).
// Manual schedule control via Admin API ensures snapshots are only computed when data is ready.
func (db *DB) initLPPositionSnapshots(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.LPPositionSnapshotColumns)
	tableName := db.getGlobalTableName(indexermodels.LPPositionSnapshotsProductionTableName)

	// Create a global table with ReplacingMergeTree(updated_at) for idempotent updates
	// Order by: (source_chain_id, address, pool_id, snapshot_date) for efficient queries
	// Bloom filter on address for fast lookups by LP holder
	query := fmt.Sprintf(
		`
		CREATE TABLE IF NOT EXISTS "%s"."%s" %s (
			%s,
			INDEX address_bloom address TYPE bloom_filter GRANULARITY 4
		) ENGINE = %s
		ORDER BY (source_chain_id, address, pool_id, snapshot_date)
	`,
		db.Name,
		tableName,
		db.OnCluster(),
		schemaSQL,
		clickhouse.ReplicatedEngine(
			clickhouse.ReplacingMergeTree,
			indexermodels.LPPositionSnapshotsUpdateAtColumnName,
		),
	)

	if err := db.Exec(ctx, query); err != nil {
		return fmt.Errorf("create %s: %w", tableName, err)
	}

	db.Logger.Debug("LP position snapshots table initialized",
		zap.String("table", tableName),
		zap.String("database", db.Name))

	return nil
}

// InsertLPPositionSnapshots inserts LP position snapshots directly into the production table.
// Uses ReplacingMergeTree deduplication to allow recomputing "today" multiple times.
// The newest updated_at timestamp wins during deduplication.
//
// ARCHITECTURAL NOTE: LP snapshots are computed by a scheduled workflow (hourly per chain)
// that queries per-chain pool_points_by_holder tables at the snapshot height. This avoids
// race conditions when starting to index new chains.
func (db *DB) InsertLPPositionSnapshots(ctx context.Context, snapshots []*indexermodels.LPPositionSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}

	tableName := db.getGlobalTableName(indexermodels.LPPositionSnapshotsProductionTableName)
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
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

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
// Returns snapshots ordered by (source_chain_id, address, pool_id, snapshot_date DESC).
// Uses FINAL to ensure ReplacingMergeTree deduplication is applied.
func (db *DB) QueryLPPositionSnapshots(ctx context.Context, params LPPositionSnapshotQueryParams) ([]*indexermodels.LPPositionSnapshot, error) {
	tableName := db.getGlobalTableName(indexermodels.LPPositionSnapshotsProductionTableName)
	baseQuery := fmt.Sprintf(`
		SELECT
			source_chain_id, address, pool_id,
			snapshot_date, snapshot_height, snapshot_balance, pool_share_percentage,
			position_created_date, position_closed_date, is_position_active,
			computed_at, updated_at
		FROM "%s"."%s" FINAL
	`, db.Name, tableName)

	// Build WHERE clause dynamically
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

	if params.ActiveOnly {
		whereClauses = append(whereClauses, "is_position_active = 1")
	}

	// Add WHERE clause if we have filters
	if len(whereClauses) > 0 {
		baseQuery += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Add ORDER BY
	baseQuery += " ORDER BY source_chain_id, address, pool_id, snapshot_date DESC"

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
	tableName := db.getGlobalTableName(indexermodels.LPPositionSnapshotsProductionTableName)
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

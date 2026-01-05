package crosschain

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"go.uber.org/zap"
)

// initTVLSnapshots creates the tvl_snapshots_global table.
// Uses ReplacingMergeTree(updated_at) for idempotent hourly snapshot updates.
//
// ORDER BY (chain_id, snapshot_hour) optimizes for:
// - Per-chain history queries: WHERE chain_id = ? ORDER BY snapshot_hour
// - Cross-chain aggregation: GROUP BY snapshot_hour (scans all chains efficiently)
//
// This table stores pre-computed hourly TVL snapshots to avoid expensive
// real-time aggregation queries that suffer from the "missing entity" problem
// when entities have no activity in certain time buckets.
func (db *DB) initTVLSnapshots(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.TVLSnapshotColumns)
	tableName := db.getGlobalTableName(indexermodels.TVLSnapshotsProductionTableName)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" %s (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, snapshot_hour)
	`,
		db.Name,
		tableName,
		db.OnCluster(),
		schemaSQL,
		clickhouse.ReplicatedEngine(
			clickhouse.ReplacingMergeTree,
			indexermodels.TVLSnapshotsUpdatedAtColumnName,
		),
	)

	if err := db.Exec(ctx, query); err != nil {
		return fmt.Errorf("create %s: %w", tableName, err)
	}

	db.Logger.Debug("TVL snapshots table initialized",
		zap.String("table", tableName),
		zap.String("database", db.Name))

	return nil
}

// InsertTVLSnapshots inserts TVL snapshots in batch.
// ReplacingMergeTree deduplication allows recomputing the same hour multiple times;
// the row with the latest updated_at wins during merge.
func (db *DB) InsertTVLSnapshots(ctx context.Context, snapshots []*indexermodels.TVLSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}

	tableName := db.getGlobalTableName(indexermodels.TVLSnapshotsProductionTableName)
	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, snapshot_hour, snapshot_height,
		accounts_tvl, pools_tvl, validators_tvl, orders_tvl, dex_orders_tvl, total_tvl,
		computed_at, updated_at
	) VALUES`, db.Name, tableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return fmt.Errorf("prepare batch for %s: %w", tableName, err)
	}
	defer func(batch driver.Batch) { _ = batch.Abort() }(batch)

	for _, s := range snapshots {
		if err := batch.Append(
			s.ChainID,
			s.SnapshotHour,
			s.SnapshotHeight,
			s.AccountsTVL,
			s.PoolsTVL,
			s.ValidatorsTVL,
			s.OrdersTVL,
			s.DexOrdersTVL,
			s.TotalTVL,
			s.ComputedAt,
			s.UpdatedAt,
		); err != nil {
			return fmt.Errorf("append snapshot to batch: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("send batch for %s: %w", tableName, err)
	}

	return nil
}

// TVLSnapshotQueryParams defines filtering criteria for querying TVL snapshots.
type TVLSnapshotQueryParams struct {
	ChainIDs  []uint64   // Filter by chain IDs (optional, empty = all chains)
	StartTime *time.Time // Filter snapshots >= start time (optional)
	EndTime   *time.Time // Filter snapshots <= end time (optional)
	Limit     int        // Max number of results (0 = unlimited)
}

// QueryTVLSnapshots retrieves TVL snapshots based on filter criteria.
// Returns snapshots ordered by (chain_id, snapshot_hour ASC).
// Uses FINAL to ensure ReplacingMergeTree deduplication is applied.
func (db *DB) QueryTVLSnapshots(ctx context.Context, params TVLSnapshotQueryParams) ([]*indexermodels.TVLSnapshot, error) {
	tableName := db.getGlobalTableName(indexermodels.TVLSnapshotsProductionTableName)

	query := fmt.Sprintf(`
		SELECT
			chain_id, snapshot_hour, snapshot_height,
			accounts_tvl, pools_tvl, validators_tvl, orders_tvl, dex_orders_tvl, total_tvl,
			computed_at, updated_at
		FROM "%s"."%s" FINAL
	`, db.Name, tableName)

	var whereClauses []string
	var args []any

	if len(params.ChainIDs) > 0 {
		whereClauses = append(whereClauses, "chain_id IN (?)")
		args = append(args, params.ChainIDs)
	}

	if params.StartTime != nil {
		whereClauses = append(whereClauses, "snapshot_hour >= ?")
		args = append(args, *params.StartTime)
	}

	if params.EndTime != nil {
		whereClauses = append(whereClauses, "snapshot_hour <= ?")
		args = append(args, *params.EndTime)
	}

	if len(whereClauses) > 0 {
		query += " WHERE "
		for i, clause := range whereClauses {
			if i > 0 {
				query += " AND "
			}
			query += clause
		}
	}

	query += " ORDER BY chain_id, snapshot_hour ASC"

	if params.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", params.Limit)
	}

	var snapshots []*indexermodels.TVLSnapshot
	if err := db.Select(ctx, &snapshots, query, args...); err != nil {
		return nil, fmt.Errorf("query TVL snapshots: %w", err)
	}

	return snapshots, nil
}

// TVLHistoryPoint is the aggregated result for charting across all chains.
type TVLHistoryPoint struct {
	SnapshotHour time.Time `ch:"snapshot_hour" json:"timestamp"`
	TotalTVL     uint64    `ch:"total_tvl" json:"value"`
}

// QueryAggregatedTVLHistory retrieves TVL summed across chains, grouped by hour.
// This is the primary query for TVL history charts in launchpad.
// Returns points ordered by snapshot_hour ASC for charting.
func (db *DB) QueryAggregatedTVLHistory(ctx context.Context, params TVLSnapshotQueryParams) ([]TVLHistoryPoint, error) {
	tableName := db.getGlobalTableName(indexermodels.TVLSnapshotsProductionTableName)

	query := fmt.Sprintf(`
		SELECT
			snapshot_hour,
			SUM(total_tvl) as total_tvl
		FROM "%s"."%s" FINAL
	`, db.Name, tableName)

	var whereClauses []string
	var args []any

	if len(params.ChainIDs) > 0 {
		whereClauses = append(whereClauses, "chain_id IN (?)")
		args = append(args, params.ChainIDs)
	}

	if params.StartTime != nil {
		whereClauses = append(whereClauses, "snapshot_hour >= ?")
		args = append(args, *params.StartTime)
	}

	if params.EndTime != nil {
		whereClauses = append(whereClauses, "snapshot_hour <= ?")
		args = append(args, *params.EndTime)
	}

	if len(whereClauses) > 0 {
		query += " WHERE "
		for i, clause := range whereClauses {
			if i > 0 {
				query += " AND "
			}
			query += clause
		}
	}

	query += " GROUP BY snapshot_hour ORDER BY snapshot_hour ASC"

	if params.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", params.Limit)
	}

	var points []TVLHistoryPoint
	if err := db.Select(ctx, &points, query, args...); err != nil {
		return nil, fmt.Errorf("query aggregated TVL history: %w", err)
	}

	return points, nil
}

// GetLatestTVLSnapshot retrieves the most recent snapshot for a specific chain.
// Returns nil if no snapshot exists for the given chain_id.
func (db *DB) GetLatestTVLSnapshot(ctx context.Context, chainID uint64) (*indexermodels.TVLSnapshot, error) {
	tableName := db.getGlobalTableName(indexermodels.TVLSnapshotsProductionTableName)

	query := fmt.Sprintf(`
		SELECT
			chain_id, snapshot_hour, snapshot_height,
			accounts_tvl, pools_tvl, validators_tvl, orders_tvl, dex_orders_tvl, total_tvl,
			computed_at, updated_at
		FROM "%s"."%s" FINAL
		WHERE chain_id = ?
		ORDER BY snapshot_hour DESC
		LIMIT 1
	`, db.Name, tableName)

	var snapshots []*indexermodels.TVLSnapshot
	if err := db.Select(ctx, &snapshots, query, chainID); err != nil {
		return nil, fmt.Errorf("query latest TVL snapshot: %w", err)
	}

	if len(snapshots) == 0 {
		return nil, nil
	}

	return snapshots[0], nil
}

// DeleteTVLSnapshotsBefore deletes snapshots older than the given time.
// Useful for data retention policies.
func (db *DB) DeleteTVLSnapshotsBefore(ctx context.Context, before time.Time) error {
	tableName := db.getGlobalTableName(indexermodels.TVLSnapshotsProductionTableName)

	query := fmt.Sprintf(`
		ALTER TABLE "%s"."%s" %s
		DELETE WHERE snapshot_hour < ?
	`, db.Name, tableName, db.OnCluster())

	if err := db.Exec(ctx, query, before); err != nil {
		return fmt.Errorf("delete old TVL snapshots: %w", err)
	}

	return nil
}

package chain

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initProposalSnapshots creates the proposal_snapshots table with ReplacingMergeTree engine.
// Uses snapshot_time as the deduplication version key.
// The table stores time-series snapshots of governance proposals.
//
// Compression strategy:
// - ZSTD for string fields (proposal hash and JSON data)
// - ZSTD for boolean fields (approve flag)
// - DoubleDelta + LZ4 for DateTime64 (timestamps are monotonic)
//
// ARCHITECTURAL NOTE: Unlike other entities, proposal data is NOT height-based because
// the /v1/gov/proposals RPC endpoint does not support historical queries. Instead, snapshots
// are captured via a scheduled workflow that runs every 5 minutes. No staging table is used.
func (db *DB) initProposalSnapshots(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.ProposalSnapshotColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" %s (
			%s
		) ENGINE = %s
		ORDER BY (proposal_hash, snapshot_time)
	`, db.Name, indexermodels.ProposalSnapshotsProductionTableName, db.OnCluster(), schemaSQL, db.Engine(indexermodels.ProposalSnapshotsProductionTableName, "ReplacingMergeTree", "snapshot_time"))

	if err := db.Exec(ctx, query); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.ProposalSnapshotsProductionTableName, err)
	}

	return nil
}

// InsertProposalSnapshots inserts proposal snapshots directly into the production proposal_snapshots table.
// Unlike other entities, proposal snapshots do NOT use the staging pattern because they are
// captured via scheduled workflow (not height-based indexing).
//
// ARCHITECTURAL NOTE: Proposal snapshots are time-based, captured every 5 minutes via a
// scheduled workflow. The RPC endpoint doesn't support historical queries, so we cannot
// use the typical RPC(H) vs RPC(H-1) comparison pattern.
func (db *DB) InsertProposalSnapshots(ctx context.Context, snapshots []*indexermodels.ProposalSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."proposal_snapshots" (
		proposal_hash, proposal, approve, snapshot_time
	) VALUES`, db.Name)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, snapshot := range snapshots {
		err = batch.Append(
			snapshot.ProposalHash,
			snapshot.Proposal,
			snapshot.Approve,
			snapshot.SnapshotTime,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

// InsertProposalSnapshotsStaging is deprecated - kept for backward compatibility.
// Use InsertProposalSnapshots instead.
func (db *DB) InsertProposalSnapshotsStaging(ctx context.Context, snapshots []*indexermodels.ProposalSnapshot) error {
	return db.InsertProposalSnapshots(ctx, snapshots)
}

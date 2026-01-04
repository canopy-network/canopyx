package chain

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initPollSnapshots creates the poll_snapshots table with ReplacingMergeTree engine.
// Uses snapshot_time as the deduplication version key.
// The table stores time-series snapshots of governance polls.
//
// Compression strategy:
// - Delta + ZSTD for uint64 fields (voting stats change incrementally)
// - ZSTD for string fields (proposal hash/URL)
// - Delta + ZSTD for DateTime64 (timestamps are monotonic)
//
// ARCHITECTURAL NOTE: Unlike other entities, poll data is NOT height-based because
// the /v1/gov/poll RPC endpoint does not support historical queries. Instead, snapshots
// are captured via a scheduled workflow that runs every 20 seconds. No staging table is used.
func (db *DB) initPollSnapshots(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.PollSnapshotColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (proposal_hash, snapshot_time)
	`, db.Name, indexermodels.PollSnapshotsProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "snapshot_time"))

	if err := db.Exec(ctx, query); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.PollSnapshotsProductionTableName, err)
	}

	return nil
}

// InsertPollSnapshots inserts poll snapshots directly into the production poll_snapshots table.
// Unlike other entities, poll snapshots do NOT use the staging pattern because they are
// captured via scheduled workflow (not height-based indexing).
//
// ARCHITECTURAL NOTE: Poll snapshots are time-based, captured every 20 seconds via a
// scheduled workflow. The RPC endpoint doesn't support historical queries, so we cannot
// use the typical RPC(H) vs RPC(H-1) comparison pattern.
func (db *DB) InsertPollSnapshots(ctx context.Context, snapshots []*indexermodels.PollSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		proposal_hash, proposal_url,
		accounts_approve_tokens, accounts_reject_tokens, accounts_total_voted_tokens, accounts_total_tokens,
		accounts_approve_percentage, accounts_reject_percentage, accounts_voted_percentage,
		validators_approve_tokens, validators_reject_tokens, validators_total_voted_tokens, validators_total_tokens,
		validators_approve_percentage, validators_reject_percentage, validators_voted_percentage,
		snapshot_time
	) VALUES`, db.Name, indexermodels.PollSnapshotsProductionTableName)

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
			snapshot.ProposalURL,
			snapshot.AccountsApproveTokens,
			snapshot.AccountsRejectTokens,
			snapshot.AccountsTotalVotedTokens,
			snapshot.AccountsTotalTokens,
			snapshot.AccountsApprovePercentage,
			snapshot.AccountsRejectPercentage,
			snapshot.AccountsVotedPercentage,
			snapshot.ValidatorsApproveTokens,
			snapshot.ValidatorsRejectTokens,
			snapshot.ValidatorsTotalVotedTokens,
			snapshot.ValidatorsTotalTokens,
			snapshot.ValidatorsApprovePercentage,
			snapshot.ValidatorsRejectPercentage,
			snapshot.ValidatorsVotedPercentage,
			snapshot.SnapshotTime,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

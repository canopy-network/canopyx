package chain

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initPollSnapshots creates the poll_snapshots table and its staging table with ReplacingMergeTree engine.
// Uses height as the deduplication version key.
// The table stores governance poll snapshots at each height.
//
// Compression strategy:
// - Delta + ZSTD for uint64 fields (voting stats change incrementally)
// - ZSTD for string fields (proposal hash/URL)
// - Delta + ZSTD for DateTime64 (timestamps are monotonic)
//
// ARCHITECTURAL NOTE: Unlike other entities, poll data is snapshotted at every height
// because the /v1/gov/poll RPC endpoint does not support historical queries.
// We cannot use the typical RPC(H) vs RPC(H-1) comparison pattern.
func (db *DB) initPollSnapshots(ctx context.Context) error {
	queryTemplate := `
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			proposal_hash String CODEC(ZSTD(1)),
			height UInt64 CODEC(Delta, ZSTD(1)),
			proposal_url String CODEC(ZSTD(1)),
			accounts_approve_tokens UInt64 CODEC(Delta, ZSTD(1)),
			accounts_reject_tokens UInt64 CODEC(Delta, ZSTD(1)),
			accounts_total_voted_tokens UInt64 CODEC(Delta, ZSTD(1)),
			accounts_total_tokens UInt64 CODEC(Delta, ZSTD(1)),
			accounts_approve_percentage UInt64 CODEC(Delta, ZSTD(1)),
			accounts_reject_percentage UInt64 CODEC(Delta, ZSTD(1)),
			accounts_voted_percentage UInt64 CODEC(Delta, ZSTD(1)),
			validators_approve_tokens UInt64 CODEC(Delta, ZSTD(1)),
			validators_reject_tokens UInt64 CODEC(Delta, ZSTD(1)),
			validators_total_voted_tokens UInt64 CODEC(Delta, ZSTD(1)),
			validators_total_tokens UInt64 CODEC(Delta, ZSTD(1)),
			validators_approve_percentage UInt64 CODEC(Delta, ZSTD(1)),
			validators_reject_percentage UInt64 CODEC(Delta, ZSTD(1)),
			validators_voted_percentage UInt64 CODEC(Delta, ZSTD(1)),
			height_time DateTime64(6) CODEC(Delta, ZSTD(1))
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (proposal_hash, height)
	`

	// Create production table
	productionQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.PollSnapshotsProductionTableName)
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.PollSnapshotsProductionTableName, err)
	}

	// Create staging table
	stagingQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.PollSnapshotsStagingTableName)
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.PollSnapshotsStagingTableName, err)
	}

	return nil
}

// InsertPollSnapshotsStaging inserts poll snapshots into the poll_snapshots_staging table.
// This follows the two-phase commit pattern for data consistency.
//
// ARCHITECTURAL NOTE: Unlike other entities that use sparse inserts (snapshot-on-change),
// we insert poll snapshots at every height because the RPC endpoint doesn't support
// historical queries. We cannot compare RPC(H) vs RPC(H-1) to detect changes.
func (db *DB) InsertPollSnapshotsStaging(ctx context.Context, snapshots []*indexermodels.PollSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}

	stagingTable := fmt.Sprintf("%s.poll_snapshots_staging", db.Name)
	query := fmt.Sprintf(`INSERT INTO %s (
		proposal_hash, height, proposal_url,
		accounts_approve_tokens, accounts_reject_tokens, accounts_total_voted_tokens, accounts_total_tokens,
		accounts_approve_percentage, accounts_reject_percentage, accounts_voted_percentage,
		validators_approve_tokens, validators_reject_tokens, validators_total_voted_tokens, validators_total_tokens,
		validators_approve_percentage, validators_reject_percentage, validators_voted_percentage,
		height_time
	) VALUES`, stagingTable)

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
			snapshot.Height,
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
			snapshot.HeightTime,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

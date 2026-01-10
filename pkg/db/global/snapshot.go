package global

import (
    "context"
    "fmt"

    "github.com/canopy-network/canopyx/pkg/db/clickhouse"
    indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// --- Poll Snapshots (NO staging table) ---

func (db *DB) initPollSnapshots(ctx context.Context) error {
    schemaSQL := indexermodels.ColumnsToGlobalSchemaSQL(indexermodels.PollSnapshotColumns)

    query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, proposal_hash, snapshot_time)
	`, db.Name, indexermodels.PollSnapshotsProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "snapshot_time"))
    if err := db.Exec(ctx, query); err != nil {
        return fmt.Errorf("create %s: %w", indexermodels.PollSnapshotsProductionTableName, err)
    }

    return nil
}

func (db *DB) InsertPollSnapshots(ctx context.Context, snapshots []*indexermodels.PollSnapshot) error {
    if len(snapshots) == 0 {
        return nil
    }

    query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, proposal_hash, proposal_url,
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
    // Ensure the batch is closed, especially if not all data is sent immediately
    defer func() { _ = batch.Close() }()

    for _, s := range snapshots {
        err = batch.Append(
            db.ChainID,
            s.ProposalHash,
            s.ProposalURL,
            s.AccountsApproveTokens,
            s.AccountsRejectTokens,
            s.AccountsTotalVotedTokens,
            s.AccountsTotalTokens,
            s.AccountsApprovePercentage,
            s.AccountsRejectPercentage,
            s.AccountsVotedPercentage,
            s.ValidatorsApproveTokens,
            s.ValidatorsRejectTokens,
            s.ValidatorsTotalVotedTokens,
            s.ValidatorsTotalTokens,
            s.ValidatorsApprovePercentage,
            s.ValidatorsRejectPercentage,
            s.ValidatorsVotedPercentage,
            s.SnapshotTime,
        )
        if err != nil {
            _ = batch.Abort()
            return err
        }
    }

    return batch.Send()
}

// --- Proposal Snapshots (NO staging table) ---

func (db *DB) initProposalSnapshots(ctx context.Context) error {
    schemaSQL := indexermodels.ColumnsToGlobalSchemaSQL(indexermodels.ProposalSnapshotColumns)

    query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, proposal_hash, snapshot_time)
	`, db.Name, indexermodels.ProposalSnapshotsProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "snapshot_time"))
    if err := db.Exec(ctx, query); err != nil {
        return fmt.Errorf("create %s: %w", indexermodels.ProposalSnapshotsProductionTableName, err)
    }

    return nil
}

func (db *DB) InsertProposalSnapshots(ctx context.Context, snapshots []*indexermodels.ProposalSnapshot) error {
    if len(snapshots) == 0 {
        return nil
    }

    query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, proposal_hash, approve, snapshot_time,
		proposal_type, signer, start_height, end_height,
		parameter_space, parameter_key, parameter_value,
		dao_transfer_address, dao_transfer_amount
	) VALUES`, db.Name, indexermodels.ProposalSnapshotsProductionTableName)

    batch, err := db.PrepareBatch(ctx, query)
    if err != nil {
        return err
    }
    // Ensure the batch is closed, especially if not all data is sent immediately
    defer func() { _ = batch.Close() }()

    for _, s := range snapshots {
        err = batch.Append(
            db.ChainID,
            s.ProposalHash,
            s.Approve,
            s.SnapshotTime,
            s.ProposalType,
            s.Signer,
            s.StartHeight,
            s.EndHeight,
            s.ParameterSpace,
            s.ParameterKey,
            s.ParameterValue,
            s.DaoTransferAddress,
            s.DaoTransferAmount,
        )
        if err != nil {
            _ = batch.Abort()
            return err
        }
    }

    return batch.Send()
}

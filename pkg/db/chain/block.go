package chain

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "time"

    "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
    "github.com/canopy-network/canopyx/pkg/db/clickhouse"
    indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initBlocks initializes the blocks table with time-based skip index for efficient time queries.
// The minmax index on 'time' column allows fast lookups for GetHighestBlockBeforeTime queries.
func (db *DB) initBlocks(ctx context.Context) error {
    schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.BlockColumns)

    productionQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s,
			INDEX idx_time time TYPE minmax GRANULARITY 8192
		) ENGINE = %s
		ORDER BY (height)
	`, db.Name, indexermodels.BlocksProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
    if err := db.Exec(ctx, productionQuery); err != nil {
        return fmt.Errorf("create %s: %w", indexermodels.BlocksProductionTableName, err)
    }

    stagingQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s,
			INDEX idx_time time TYPE minmax GRANULARITY 8192
		) ENGINE = %s
		ORDER BY (height)
	`, db.Name, indexermodels.BlocksStagingTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
    if err := db.Exec(ctx, stagingQuery); err != nil {
        return fmt.Errorf("create %s: %w", indexermodels.BlocksStagingTableName, err)
    }

    return nil
}

// GetBlock returns the latest (deduped) row for the given height.
func (db *DB) GetBlock(ctx context.Context, height uint64) (*indexermodels.Block, error) {
    // ConnOpenInOrder strategy ensures we read from the same replica we wrote to
    // Provides read-after-write consistency without Keeper coordination overhead
    var b indexermodels.Block
    query := fmt.Sprintf(`
		SELECT height, hash, time, network_id, parent_hash, proposer_address, size,
		       num_txs, total_txs, total_vdf_iterations,
		       state_root, transaction_root, validator_root, next_validator_root
		FROM "%s"."%s" FINAL
		WHERE height = ?
		LIMIT 1
	`, db.Name, indexermodels.BlocksProductionTableName)
    err := db.QueryRow(ctx, query, height).Scan(
        &b.Height,
        &b.Hash,
        &b.Time,
        &b.NetworkID,
        &b.LastBlockHash,
        &b.ProposerAddress,
        &b.Size,
        &b.NumTxs,
        &b.TotalTxs,
        &b.TotalVDFIterations,
        &b.StateRoot,
        &b.TransactionRoot,
        &b.ValidatorRoot,
        &b.NextValidatorRoot,
    )
    return &b, err
}

// InsertBlocksStaging persists blocks into the blocks_staging table.
// This follows the two-phase commit pattern for data consistency.
// Consistency is guaranteed by ConnOpenInOrder: we always read from the same replica we wrote to.
func (db *DB) InsertBlocksStaging(ctx context.Context, block *indexermodels.Block) error {
    query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		height, hash, time, network_id, parent_hash, proposer_address, size,
		num_txs, total_txs, total_vdf_iterations,
		state_root, transaction_root, validator_root, next_validator_root
	) VALUES`, db.Name, indexermodels.BlocksStagingTableName)
    batch, err := db.PrepareBatch(ctx, query)
    if err != nil {
        return err
    }
    defer func(batch driver.Batch) {
        _ = batch.Abort()
    }(batch)

    err = batch.Append(
        block.Height,
        block.Hash,
        block.Time,
        block.NetworkID,
        block.LastBlockHash,
        block.ProposerAddress,
        block.Size,
        block.NumTxs,
        block.TotalTxs,
        block.TotalVDFIterations,
        block.StateRoot,
        block.TransactionRoot,
        block.ValidatorRoot,
        block.NextValidatorRoot,
    )
    if err != nil {
        return err
    }

    return batch.Send()
}

// HasBlock reports whether a block exists at the specified height.
func (db *DB) HasBlock(ctx context.Context, height uint64) (bool, error) {
    _, err := db.GetBlock(ctx, height)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return false, nil
        }
        return false, err
    }
    return true, nil
}

// GetHighestBlockBeforeTime returns the highest block with time <= targetTime.
// This is used to find the snapshot height for calendar-day based snapshots.
// Returns the most recent block before targetTime, or nil if no blocks exist before targetTime.
//
// Used by LP position snapshots to find the block at end of day (23:59:59 UTC).
func (db *DB) GetHighestBlockBeforeTime(ctx context.Context, targetTime time.Time) (*indexermodels.Block, error) {
    var b indexermodels.Block
    query := fmt.Sprintf(`
		SELECT height, hash, time, network_id, parent_hash, proposer_address, size,
		       num_txs, total_txs, total_vdf_iterations,
		       state_root, transaction_root, validator_root, next_validator_root
		FROM "%s"."blocks" FINAL
		WHERE time <= ?
		ORDER BY time DESC
		LIMIT 1
	`, db.Name)
    err := db.QueryRow(ctx, query, targetTime).Scan(
        &b.Height,
        &b.Hash,
        &b.Time,
        &b.NetworkID,
        &b.LastBlockHash,
        &b.ProposerAddress,
        &b.Size,
        &b.NumTxs,
        &b.TotalTxs,
        &b.TotalVDFIterations,
        &b.StateRoot,
        &b.TransactionRoot,
        &b.ValidatorRoot,
        &b.NextValidatorRoot,
    )

    if err != nil {
        // Return nil if no block exists before targetTime (not an error)
        if clickhouse.IsNoRows(err) {
            return nil, nil
        }
        return nil, fmt.Errorf("query highest block before time: %w", err)
    }

    return &b, nil
}

// DeleteBlock removes a block record for the given height.
func (db *DB) DeleteBlock(ctx context.Context, height uint64) error {
    stmt := fmt.Sprintf(
        `DELETE FROM "%s"."%s" WHERE height = ?`,
        db.Name, indexermodels.BlocksProductionTableName,
    )
    return db.Db.Exec(ctx, stmt, height)
}

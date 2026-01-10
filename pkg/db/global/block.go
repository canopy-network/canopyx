package global

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "time"

    "github.com/canopy-network/canopyx/pkg/db/clickhouse"
    indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initBlocks initializes the blocks table with chain_id and time-based skip index.
func (db *DB) initBlocks(ctx context.Context) error {
    schemaSQL := indexermodels.ColumnsToGlobalSchemaSQL(indexermodels.BlockColumns)

    query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s,
			INDEX idx_time time TYPE minmax GRANULARITY 8192
		) ENGINE = %s
		ORDER BY (chain_id, height)
	`, db.Name, indexermodels.BlocksProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

    return db.Exec(ctx, query)
}

// GetBlock returns the block for the given height and chain context.
// Does not use FINAL since we query by exact (chain_id, height) which is unique.
func (db *DB) GetBlock(ctx context.Context, height uint64) (*indexermodels.Block, error) {
    var b indexermodels.Block
    query := fmt.Sprintf(`
		SELECT height, hash, time, network_id, parent_hash, proposer_address, size,
		       num_txs, total_txs, total_vdf_iterations,
		       state_root, transaction_root, validator_root, next_validator_root
		FROM "%s"."%s"
        PREWHERE chain_id = ?
		WHERE height = ?
		LIMIT 1
	`, db.Name, indexermodels.BlocksProductionTableName)
    err := db.QueryRow(ctx, query, db.ChainID, height).Scan(
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

// InsertBlocks persists blocks into the blocks table with chain_id.
func (db *DB) InsertBlocks(ctx context.Context, block *indexermodels.Block) error {
    query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, height, hash, time, network_id, parent_hash, proposer_address, size,
		num_txs, total_txs, total_vdf_iterations,
		state_root, transaction_root, validator_root, next_validator_root
	) VALUES`, db.Name, indexermodels.BlocksProductionTableName)
    batch, err := db.PrepareBatch(ctx, query)
    if err != nil {
        return err
    }
    // Ensure the batch is closed, especially if not all data is sent immediately
    defer func() { _ = batch.Close() }()

    err = batch.Append(
        db.ChainID, // chain_id from DB context
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
        _ = batch.Abort()
        return err
    }

    return batch.Send()
}

// GetBlockTime returns only the timestamp for a block at the given height.
// Does not use FINAL since we query by exact (chain_id, height) which is unique.
func (db *DB) GetBlockTime(ctx context.Context, height uint64) (time.Time, error) {
    query := fmt.Sprintf(`
		SELECT time
		FROM "%s"."%s"
		WHERE chain_id = ? AND height = ?
		LIMIT 1
	`, db.Name, indexermodels.BlocksProductionTableName)

    var blockTime time.Time
    err := db.QueryRow(ctx, query, db.ChainID, height).Scan(&blockTime)
    return blockTime, err
}

// HasBlock reports whether a block exists at the specified height for the current chain.
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

// GetHighestBlockBeforeTime returns the highest block with time <= targetTime for the current chain.
func (db *DB) GetHighestBlockBeforeTime(ctx context.Context, targetTime time.Time) (*indexermodels.Block, error) {
    var b indexermodels.Block
    query := fmt.Sprintf(`
		SELECT height, hash, time, network_id, parent_hash, proposer_address, size,
		       num_txs, total_txs, total_vdf_iterations,
		       state_root, transaction_root, validator_root, next_validator_root
		FROM "%s"."blocks" FINAL
		WHERE chain_id = ? AND time <= ?
		ORDER BY time DESC
		LIMIT 1
	`, db.Name)
    err := db.QueryRow(ctx, query, db.ChainID, targetTime).Scan(
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
        if clickhouse.IsNoRows(err) {
            return nil, nil
        }
        return nil, fmt.Errorf("query highest block before time: %w", err)
    }

    return &b, nil
}

// DeleteBlock removes a block record for the given height and chain.
func (db *DB) DeleteBlock(ctx context.Context, height uint64) error {
    stmt := fmt.Sprintf(
        `DELETE FROM "%s"."%s" WHERE chain_id = ? AND height = ?`,
        db.Name, indexermodels.BlocksProductionTableName,
    )
    return db.Db.Exec(ctx, stmt, db.ChainID, height)
}

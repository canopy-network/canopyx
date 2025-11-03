package chain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initBlocks initializes the blocks table.
func (db *DB) initBlocks(ctx context.Context) error {
	productionQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			height UInt64,
			hash String,
			time DateTime64(6),
			parent_hash String,
			proposer_address String,
			size Int32
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (height)
	`, db.Name, indexermodels.BlocksProductionTableName)
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.BlocksProductionTableName, err)
	}

	stagingQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			height UInt64,
			hash String,
			time DateTime64(6),
			parent_hash String,
			proposer_address String,
			size Int32
		) ENGINE = MergeTree()
		ORDER BY (height)
	`, db.Name, indexermodels.BlocksStagingTableName)
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.BlocksStagingTableName, err)
	}

	return nil
}

// GetBlock returns the latest (deduped) row for the given height.
func (db *DB) GetBlock(ctx context.Context, height uint64) (*indexermodels.Block, error) {
	var b indexermodels.Block
	query := `
		SELECT height, hash, time, parent_hash, proposer_address, size
		FROM blocks FINAL
		WHERE height = ?
		LIMIT 1
	`
	err := db.QueryRow(ctx, query, height).Scan(
		&b.Height,
		&b.Hash,
		&b.Time,
		&b.LastBlockHash,
		&b.ProposerAddress,
		&b.Size,
	)
	return &b, err
}

// InsertBlocksStaging persists blocks into the blocks_staging table.
// This follows the two-phase commit pattern for data consistency.
func (db *DB) InsertBlocksStaging(ctx context.Context, block *indexermodels.Block) error {
	query := fmt.Sprintf(`INSERT INTO "%s".blocks_staging (height, hash, time, parent_hash, proposer_address, size) VALUES`, db.Name)
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
		block.LastBlockHash,
		block.ProposerAddress,
		block.Size,
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

// DeleteBlock removes a block record for the given height.
func (db *DB) DeleteBlock(ctx context.Context, height uint64) error {
	stmt := fmt.Sprintf(`ALTER TABLE "%s"."blocks" DELETE WHERE height = ?`, db.Name)
	return db.Db.Exec(ctx, stmt, height)
}

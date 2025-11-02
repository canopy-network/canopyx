package chain

import (
	"context"
	"fmt"
	"time"

	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initBlockSummaries initializes the block_summaries table and its staging table.
func (db *DB) initBlockSummaries(ctx context.Context) error {
	queryTemplate := `
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			height UInt64,
			height_time DateTime64(6),
			num_txs UInt32 DEFAULT 0,
			tx_counts_by_type Map(LowCardinality(String), UInt32)
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (height)
	`

	// Create production table
	productionQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.BlockSummariesProductionTableName)
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.BlockSummariesProductionTableName, err)
	}

	// Create staging table
	stageQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.BlockSummariesStagingTableName)
	if err := db.Exec(ctx, stageQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.BlockSummariesStagingTableName, err)
	}

	return nil
}

// InsertBlockSummariesStaging persists block summary data into the block_summaries_staging table.
// This follows the two-phase commit pattern for data consistency.
func (db *DB) InsertBlockSummariesStaging(ctx context.Context, height uint64, blockTime time.Time, numTxs uint32, txCountsByType map[string]uint32) error {
	query := fmt.Sprintf(`INSERT INTO "%s".block_summaries_staging (height, height_time, num_txs, tx_counts_by_type) VALUES`, db.Name)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func() { _ = batch.Abort() }()

	err = batch.Append(height, blockTime, numTxs, txCountsByType)
	if err != nil {
		return err
	}

	return batch.Send()
}

// InsertBlockSummary persists block summary data (entity counts) into the chain database.
// blockTime is the timestamp from the block, used to populate height_time for time-range queries.
// txCountsByType contains a breakdown of transaction counts by message type.
func (db *DB) GetBlockSummary(ctx context.Context, height uint64) (*indexermodels.BlockSummary, error) {
	var bs indexermodels.BlockSummary

	query := `
        SELECT height, height_time, num_txs, tx_counts_by_type
        FROM block_summaries FINAL
        WHERE height = ?
        LIMIT 1
    `

	err := db.Db.QueryRow(ctx, query, height).Scan(
		&bs.Height,
		&bs.HeightTime,
		&bs.NumTxs,
		&bs.TxCountsByType,
	)

	return &bs, err
}

// QueryBlockSummaries retrieves a paginated list of block summaries ordered by height.
// If sortDesc is true, orders by height DESC (newest first), otherwise ASC (oldest first).
// If cursor > 0 and sortDesc is true, only summaries with height < cursor are returned.
// If cursor > 0 and sortDesc is false, only summaries with height > cursor are returned.
// The limit parameter controls the maximum number of rows returned (+1 for pagination detection).

package chain

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initValidatorSigningInfo initializes the validator_signing_info table and its staging table.
// The production table uses aggressive compression for storage optimization.
// The staging table has the same schema but no TTL (TTL only on production).
//
// Schema design:
// - ReplacingMergeTree(height): Deduplicates by (address, height), keeping latest
// - ORDER BY (address, height): Enables efficient temporal queries
// - Delta codec for monotonically increasing counters and heights
func (db *DB) initValidatorSigningInfo(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.ValidatorSigningInfoColumns)

	// Production table: ORDER BY (address, height) for efficient address lookups
	productionQuery := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS "%s"."%s" %s (
			%s
		) ENGINE = %s
		ORDER BY (address, height)
		SETTINGS index_granularity = 8192
	`, db.Name, indexermodels.ValidatorSigningInfoProductionTableName, db.OnCluster(), schemaSQL, db.Engine(indexermodels.ValidatorSigningInfoProductionTableName, "ReplacingMergeTree", "height"))
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.ValidatorSigningInfoProductionTableName, err)
	}

	// Staging table: ORDER BY (height, address) for efficient cleanup/promotion WHERE height = ?
	stagingQuery := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS "%s"."%s" %s (
			%s
		) ENGINE = %s
		ORDER BY (height, address)
		SETTINGS index_granularity = 8192
	`, db.Name, indexermodels.ValidatorSigningInfoStagingTableName, db.OnCluster(), schemaSQL, db.Engine(indexermodels.ValidatorSigningInfoStagingTableName, "ReplacingMergeTree", "height"))
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.ValidatorSigningInfoStagingTableName, err)
	}

	return nil
}

// InsertValidatorSigningInfoStaging inserts validator signing info snapshots to the staging table.
// Staging tables are used for new data before promotion to production.
// This follows the two-phase commit pattern for data consistency.
func (db *DB) InsertValidatorSigningInfoStaging(ctx context.Context, signingInfos []*indexermodels.ValidatorSigningInfo) error {
	if len(signingInfos) == 0 {
		return nil
	}

	query := fmt.Sprintf(
		`INSERT INTO "%s"."%s" (address, missed_blocks_count, missed_blocks_window, last_signed_height, start_height, height, height_time) VALUES`,
		db.Name, indexermodels.ValidatorSigningInfoStagingTableName,
	)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, info := range signingInfos {
		err = batch.Append(
			info.Address,
			info.MissedBlocksCount,
			info.MissedBlocksWindow,
			info.LastSignedHeight,
			info.StartHeight,
			info.Height,
			info.HeightTime,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

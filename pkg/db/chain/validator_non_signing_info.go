package chain

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initValidatorNonSigningInfo initializes the validator_non_signing_info table and its staging table.
// The production table uses aggressive compression for storage optimization.
// The staging table has the same schema but no TTL (TTL only on production).
//
// Schema design:
// - ReplacingMergeTree(height): Deduplicates by (address, height), keeping latest
// - ORDER BY (address, height): Enables efficient temporal queries
// - Delta codec for monotonically increasing counters and heights
func (db *DB) initValidatorNonSigningInfo(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.ValidatorNonSigningInfoColumns)

	// Production table: ORDER BY (address, height) for efficient address lookups
	productionQuery := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (address, height)
		SETTINGS index_granularity = 8192
	`, db.Name, indexermodels.ValidatorNonSigningInfoProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.ValidatorNonSigningInfoProductionTableName, err)
	}

	// Staging table: ORDER BY (height, address) for efficient cleanup/promotion WHERE height = ?
	stagingQuery := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (height, address)
		SETTINGS index_granularity = 8192
	`, db.Name, indexermodels.ValidatorNonSigningInfoStagingTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.ValidatorNonSigningInfoStagingTableName, err)
	}

	return nil
}

// InsertValidatorNonSigningInfoStaging inserts validator non-signing info snapshots to the staging table.
// Staging tables are used for new data before promotion to production.
// This follows the two-phase commit pattern for data consistency.
// Removed trash properties: missed_blocks_window, start_height (per issues.md)
func (db *DB) InsertValidatorNonSigningInfoStaging(ctx context.Context, nonSigningInfos []*indexermodels.ValidatorNonSigningInfo) error {
	if len(nonSigningInfos) == 0 {
		return nil
	}

	query := fmt.Sprintf(
		`INSERT INTO "%s"."%s" (address, missed_blocks_count, last_signed_height, height, height_time) VALUES`,
		db.Name, indexermodels.ValidatorNonSigningInfoStagingTableName,
	)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, info := range nonSigningInfos {
		err = batch.Append(
			info.Address,
			info.MissedBlocksCount,
			info.LastSignedHeight,
			info.Height,
			info.HeightTime,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

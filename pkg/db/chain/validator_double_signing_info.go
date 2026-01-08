package chain

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initValidatorDoubleSigningInfo initializes the validator_double_signing_info table and its staging table.
// The production table uses aggressive compression for storage optimization.
// The staging table has the same schema but no TTL (TTL only on production).
//
// Schema design:
// - ReplacingMergeTree(height): Deduplicates by (address, height), keeping latest
// - ORDER BY (address, height): Enables efficient temporal queries
// - Delta codec for monotonically increasing counters and heights
func (db *DB) initValidatorDoubleSigningInfo(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.ValidatorDoubleSigningInfoColumns)

	// Production table: ORDER BY (address, height) for efficient address lookups
	productionQuery := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (address, height)
		SETTINGS index_granularity = 8192
	`, db.Name, indexermodels.ValidatorDoubleSigningInfoProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.ValidatorDoubleSigningInfoProductionTableName, err)
	}

	// Staging table: ORDER BY (height, address) for efficient cleanup/promotion WHERE height = ?
	stagingQuery := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (height, address)
		SETTINGS index_granularity = 8192
	`, db.Name, indexermodels.ValidatorDoubleSigningInfoStagingTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.ValidatorDoubleSigningInfoStagingTableName, err)
	}

	return nil
}

// InsertValidatorDoubleSigningInfoStaging inserts validator double signing info snapshots to the staging table.
// Staging tables are used for new data before promotion to production.
// This follows the two-phase commit pattern for data consistency.
func (db *DB) InsertValidatorDoubleSigningInfoStaging(ctx context.Context, doubleSigningInfos []*indexermodels.ValidatorDoubleSigningInfo) error {
	return db.insertValidatorDoubleSigningInfo(ctx, indexermodels.ValidatorDoubleSigningInfoStagingTableName, doubleSigningInfos)
}

// InsertValidatorDoubleSigningInfoProduction inserts validator double signing info snapshots to the production table.
func (db *DB) InsertValidatorDoubleSigningInfoProduction(ctx context.Context, doubleSigningInfos []*indexermodels.ValidatorDoubleSigningInfo) error {
	return db.insertValidatorDoubleSigningInfo(ctx, indexermodels.ValidatorDoubleSigningInfoProductionTableName, doubleSigningInfos)
}

func (db *DB) insertValidatorDoubleSigningInfo(ctx context.Context, tableName string, doubleSigningInfos []*indexermodels.ValidatorDoubleSigningInfo) error {
	if len(doubleSigningInfos) == 0 {
		return nil
	}

	query := fmt.Sprintf(
		`INSERT INTO "%s"."%s" (address, evidence_count, first_evidence_height, last_evidence_height, height, height_time) VALUES`,
		db.Name, tableName,
	)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, info := range doubleSigningInfos {
		err = batch.Append(
			info.Address,
			info.EvidenceCount,
			info.FirstEvidenceHeight,
			info.LastEvidenceHeight,
			info.Height,
			info.HeightTime,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

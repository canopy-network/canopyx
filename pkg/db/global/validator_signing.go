package global

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// --- Validator Non-Signing Info ---

func (db *DB) initValidatorNonSigningInfo(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToGlobalSchemaSQL(indexermodels.ValidatorNonSigningInfoColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, address, height)
		SETTINGS index_granularity = 8192
	`, db.Name, indexermodels.ValidatorNonSigningInfoProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	return db.Exec(ctx, query)
}

// InsertValidatorNonSigningInfo inserts validator non-signing info into the table with chain_id.
func (db *DB) InsertValidatorNonSigningInfo(ctx context.Context, infos []*indexermodels.ValidatorNonSigningInfo) error {
	if len(infos) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, address, missed_blocks_count, last_signed_height, height, height_time
	) VALUES`, db.Name, indexermodels.ValidatorNonSigningInfoProductionTableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	// Ensure the batch is closed, especially if not all data is sent immediately
	defer func() { _ = batch.Close() }()

	for _, info := range infos {
		err = batch.Append(
			db.ChainID,
			info.Address,
			info.MissedBlocksCount,
			info.LastSignedHeight,
			info.Height,
			info.HeightTime,
		)
		if err != nil {
			_ = batch.Abort()
			return err
		}
	}

	return batch.Send()
}

// --- Validator Double-Signing Info ---

func (db *DB) initValidatorDoubleSigningInfo(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToGlobalSchemaSQL(indexermodels.ValidatorDoubleSigningInfoColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, address, height)
		SETTINGS index_granularity = 8192
	`, db.Name, indexermodels.ValidatorDoubleSigningInfoProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	return db.Exec(ctx, query)
}

// InsertValidatorDoubleSigningInfo inserts validator double-signing info into the table with chain_id.
func (db *DB) InsertValidatorDoubleSigningInfo(ctx context.Context, infos []*indexermodels.ValidatorDoubleSigningInfo) error {
	if len(infos) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, address, evidence_count, first_evidence_height, last_evidence_height, height, height_time
	) VALUES`, db.Name, indexermodels.ValidatorDoubleSigningInfoProductionTableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	// Ensure the batch is closed, especially if not all data is sent immediately
	defer func() { _ = batch.Close() }()

	for _, info := range infos {
		err = batch.Append(
			db.ChainID,
			info.Address,
			info.EvidenceCount,
			info.FirstEvidenceHeight,
			info.LastEvidenceHeight,
			info.Height,
			info.HeightTime,
		)
		if err != nil {
			_ = batch.Abort()
			return err
		}
	}

	return batch.Send()
}

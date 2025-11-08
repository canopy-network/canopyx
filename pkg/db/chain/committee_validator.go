package chain

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initCommitteeValidators initializes the committee_validators junction table and staging table.
// Uses ReplacingMergeTree with height as the deduplication key.
// Optimized for queries by committee_id (get all validators in a committee) and validator_address.
func (db *DB) initCommitteeValidators(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.CommitteeValidatorColumns)

	queryTemplate := `
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (committee_id, validator_address, height)
		SETTINGS index_granularity = 8192
	`

	// Create production table
	productionQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.CommitteeValidatorProductionTableName, schemaSQL)
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.CommitteeValidatorProductionTableName, err)
	}

	// Create staging table
	stagingQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.CommitteeValidatorStagingTableName, schemaSQL)
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.CommitteeValidatorStagingTableName, err)
	}

	return nil
}

// InsertCommitteeValidatorsStaging inserts committee-validator relationships into the staging table.
// This follows the two-phase commit pattern for data consistency.
// Relationships are only inserted when validator committee membership changes.
func (db *DB) InsertCommitteeValidatorsStaging(ctx context.Context, cvs []*indexermodels.CommitteeValidator) error {
	if len(cvs) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s".committee_validators_staging (
		committee_id, validator_address, staked_amount, status, delegate, compound, height, height_time
	) VALUES`, db.Name)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, cv := range cvs {
		err = batch.Append(
			cv.CommitteeID,
			cv.ValidatorAddress,
			cv.StakedAmount,
			cv.Status,
			cv.Delegate,
			cv.Compound,
			cv.Height,
			cv.HeightTime,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

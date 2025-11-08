package chain

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initValidators initializes the validators table and its staging table.
// The production table uses aggressive compression for storage optimization.
// The staging table has the same schema but no TTL (TTL only on production).
//
// Schema design:
// - ReplacingMergeTree(height): Deduplicates by (address, height), keeping latest
// - ORDER BY (address, height): Enables efficient temporal queries
// - UInt8 for boolean fields (delegate, compound): ClickHouse doesn't have native bool
// - Committee membership is tracked in committee_validators junction table instead
func (db *DB) initValidators(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.ValidatorColumns)
	queryTemplate := `
        CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (address, height)
		SETTINGS index_granularity = 8192
	`

	// Create production table
	productionQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.ValidatorsProductionTableName, schemaSQL)
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.ValidatorsProductionTableName, err)
	}

	// Create staging table
	stagingQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.ValidatorsStagingTableName, schemaSQL)
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.ValidatorsStagingTableName, err)
	}

	return nil
}

// InsertValidatorsStaging inserts validator snapshots to the staging table.
// Staging tables are used for new data before promotion to production.
// This follows the two-phase commit pattern for data consistency.
func (db *DB) InsertValidatorsStaging(ctx context.Context, validators []*indexermodels.Validator) error {
	if len(validators) == 0 {
		return nil
	}

	query := fmt.Sprintf(
		`INSERT INTO %s (address, public_key, net_address, staked_amount, max_paused_height, unstaking_height, output, delegate, compound, status, height, height_time) VALUES`,
		indexermodels.ValidatorsStagingTableName,
	)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, validator := range validators {
		err = batch.Append(
			validator.Address,
			validator.PublicKey,
			validator.NetAddress,
			validator.StakedAmount,
			validator.MaxPausedHeight,
			validator.UnstakingHeight,
			validator.Output,
			validator.Delegate,
			validator.Compound,
			validator.Status,
			validator.Height,
			validator.HeightTime,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

// initValidatorCreatedHeightView creates a materialized view to calculate the minimum height
// at which each validator was created. This replaces the need to store created_height in every snapshot.
//
// The materialized view automatically updates as new data is inserted into the validators table,
// providing an efficient way to query validator creation heights without storing the value
// in every validator snapshot row.
//
// Query usage: SELECT address, created_height FROM validator_created_height WHERE address = ?
func (db *DB) initValidatorCreatedHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."validator_created_height"
		ENGINE = AggregatingMergeTree()
		ORDER BY address
		AS SELECT
			address,
			min(height) as created_height
		FROM "%s"."validators"
		GROUP BY address
	`, db.Name, db.Name)

	return db.Exec(ctx, query)
}

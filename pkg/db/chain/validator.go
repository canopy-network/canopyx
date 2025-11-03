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
// - Array(UInt64) for committees: ClickHouse native array type for committee IDs
// - UInt8 for boolean fields (delegate, compound): ClickHouse doesn't have native bool
func (db *DB) initValidators(ctx context.Context) error {
	queryTemplate := `
        CREATE TABLE IF NOT EXISTS "%s"."%s" (
			address String CODEC(ZSTD(1)),
			public_key String CODEC(ZSTD(1)),
			net_address String CODEC(ZSTD(1)),
			staked_amount UInt64 CODEC(Delta, ZSTD(3)),
			committees Array(UInt64) CODEC(ZSTD(1)),
			max_paused_height UInt64 CODEC(Delta, ZSTD(3)),
			unstaking_height UInt64 CODEC(Delta, ZSTD(3)),
			output String CODEC(ZSTD(1)),
			delegate UInt8 DEFAULT 0,
			compound UInt8 DEFAULT 0,
			status String CODEC(ZSTD(1)),
			height UInt64 CODEC(Delta, ZSTD(3)),
			height_time DateTime64(3) CODEC(DoubleDelta, ZSTD(1))
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (address, height)
		SETTINGS index_granularity = 8192
	`

	// Create production table
	productionQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.ValidatorsProductionTableName)
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.ValidatorsProductionTableName, err)
	}

	// Create staging table
	stagingQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.ValidatorsStagingTableName)
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
		`INSERT INTO %s (address, public_key, net_address, staked_amount, committees, max_paused_height, unstaking_height, output, delegate, compound, status, height, height_time) VALUES`,
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
			validator.Committees,
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

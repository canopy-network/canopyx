package global

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// --- Committees ---

func (db *DB) initCommittees(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToGlobalCrossChainSchemaSQL(indexermodels.CommitteeColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, committee_chain_id, height)
	`, db.Name, indexermodels.CommitteeProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	return db.Exec(ctx, query)
}

// InsertCommittees inserts committees into the committees table with chain_id.
func (db *DB) InsertCommittees(ctx context.Context, committees []*indexermodels.Committee) error {
	if len(committees) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, committee_chain_id, last_root_height_updated, last_chain_height_updated,
		number_of_samples, subsidized, retired, height, height_time
	) VALUES`, db.Name, indexermodels.CommitteeProductionTableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	// Ensure the batch is closed, especially if not all data is sent immediately
	defer func() { _ = batch.Close() }()

	for _, c := range committees {
		err = batch.Append(
			db.ChainID,
			c.ChainID, // committee_chain_id
			c.LastRootHeightUpdated,
			c.LastChainHeightUpdated,
			c.NumberOfSamples,
			c.Subsidized,
			c.Retired,
			c.Height,
			c.HeightTime,
		)
		if err != nil {
			_ = batch.Abort()
			return err
		}
	}

	return batch.Send()
}

func (db *DB) initCommitteeCreatedHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."committee_created_height"
		ENGINE = %s
		ORDER BY (chain_id, committee_chain_id)
		AS SELECT
			chain_id,
			committee_chain_id,
			minSimpleState(height) as created_height
		FROM "%s"."committees"
		GROUP BY chain_id, committee_chain_id
	`, db.Name, clickhouse.ReplicatedEngine(clickhouse.AggregatingMergeTree, ""), db.Name)

	return db.Exec(ctx, query)
}

// --- Committee Validators ---

func (db *DB) initCommitteeValidators(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToGlobalSchemaSQL(indexermodels.CommitteeValidatorColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, committee_id, validator_address, height)
		SETTINGS index_granularity = 8192
	`, db.Name, indexermodels.CommitteeValidatorProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	return db.Exec(ctx, query)
}

// InsertCommitteeValidators inserts committee validators into the table with chain_id.
func (db *DB) InsertCommitteeValidators(ctx context.Context, cvs []*indexermodels.CommitteeValidator) error {
	if len(cvs) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, committee_id, validator_address, staked_amount, status, delegate, compound, height, height_time, subsidized, retired
	) VALUES`, db.Name, indexermodels.CommitteeValidatorProductionTableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	// Ensure the batch is closed, especially if not all data is sent immediately
	defer func() { _ = batch.Close() }()

	for _, cv := range cvs {
		err = batch.Append(
			db.ChainID,
			cv.CommitteeID,
			cv.ValidatorAddress,
			cv.StakedAmount,
			cv.Status,
			cv.Delegate,
			cv.Compound,
			cv.Height,
			cv.HeightTime,
			cv.Subsidized,
			cv.Retired,
		)
		if err != nil {
			_ = batch.Abort()
			return err
		}
	}

	return batch.Send()
}

// --- Committee Payments ---

func (db *DB) initCommitteePayments(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToGlobalSchemaSQL(indexermodels.CommitteePaymentColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, committee_id, address, height)
	`, db.Name, indexermodels.CommitteePaymentsProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	return db.Exec(ctx, query)
}

// InsertCommitteePayments inserts committee payments into the table with chain_id.
func (db *DB) InsertCommitteePayments(ctx context.Context, payments []*indexermodels.CommitteePayment) error {
	if len(payments) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, committee_id, address, percent, height, height_time
	) VALUES`, db.Name, indexermodels.CommitteePaymentsProductionTableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	// Ensure the batch is closed, especially if not all data is sent immediately
	defer func() { _ = batch.Close() }()

	for _, p := range payments {
		err = batch.Append(
			db.ChainID,
			p.CommitteeID,
			p.Address,
			p.Percent,
			p.Height,
			p.HeightTime,
		)
		if err != nil {
			_ = batch.Abort()
			return err
		}
	}

	return batch.Send()
}
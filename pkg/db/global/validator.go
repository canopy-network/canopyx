package global

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

func (db *DB) initValidators(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToGlobalSchemaSQL(indexermodels.ValidatorColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, address, height)
		SETTINGS index_granularity = 8192
	`, db.Name, indexermodels.ValidatorsProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	return db.Exec(ctx, query)
}

// InsertValidators inserts validators into the validators table with chain_id.
func (db *DB) InsertValidators(ctx context.Context, validators []*indexermodels.Validator) error {
	if len(validators) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, address, public_key, net_address, staked_amount, max_paused_height, unstaking_height,
		output, delegate, compound, status, height, height_time
	) VALUES`, db.Name, indexermodels.ValidatorsProductionTableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	// Ensure the batch is closed, especially if not all data is sent immediately
	defer func() { _ = batch.Close() }()

	for _, v := range validators {
		err = batch.Append(
			db.ChainID,
			v.Address,
			v.PublicKey,
			v.NetAddress,
			v.StakedAmount,
			v.MaxPausedHeight,
			v.UnstakingHeight,
			v.Output,
			v.Delegate,
			v.Compound,
			v.Status,
			v.Height,
			v.HeightTime,
		)
		if err != nil {
			_ = batch.Abort()
			return err
		}
	}

	return batch.Send()
}

func (db *DB) initValidatorCreatedHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."validator_created_height"
		ENGINE = %s
		ORDER BY (chain_id, address)
		AS SELECT
			chain_id,
			address,
			minSimpleState(height) as created_height
		FROM "%s"."validators"
		GROUP BY chain_id, address
	`, db.Name, clickhouse.ReplicatedEngine(clickhouse.AggregatingMergeTree, ""), db.Name)

	return db.Exec(ctx, query)
}

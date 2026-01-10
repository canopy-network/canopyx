package global

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

func (db *DB) initSupply(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToGlobalSchemaSQL(indexermodels.SupplyColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, height)
		SETTINGS index_granularity = 8192
	`, db.Name, indexermodels.SupplyProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	return db.Exec(ctx, query)
}

// InsertSupply inserts supply records into the supply table with chain_id.
func (db *DB) InsertSupply(ctx context.Context, supplies []*indexermodels.Supply) error {
	if len(supplies) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, total, staked, delegated_only, height, height_time
	) VALUES`, db.Name, indexermodels.SupplyProductionTableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	// Ensure the batch is closed, especially if not all data is sent immediately
	defer func() { _ = batch.Close() }()

	for _, s := range supplies {
		err = batch.Append(
			db.ChainID,
			s.Total,
			s.Staked,
			s.DelegatedOnly,
			s.Height,
			s.HeightTime,
		)
		if err != nil {
			_ = batch.Abort()
			return err
		}
	}

	return batch.Send()
}

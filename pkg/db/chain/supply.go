package chain

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initSupply initializes the supply table and its staging table.
// Uses ReplacingMergeTree with height as the deduplication key.
// Optimized for temporal queries (ORDER BY height).
func (db *DB) initSupply(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.SupplyColumns)

	queryTemplate := `
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY height
		SETTINGS index_granularity = 8192
	`

	// Create production table
	productionQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.SupplyProductionTableName, schemaSQL)
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.SupplyProductionTableName, err)
	}

	// Create staging table
	stagingQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.SupplyStagingTableName, schemaSQL)
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.SupplyStagingTableName, err)
	}

	return nil
}

// InsertSupplyStaging inserts supply snapshots to the staging table.
// This follows the two-phase commit pattern for data consistency.
// Only changed supply metrics are inserted (snapshot-on-change pattern).
func (db *DB) InsertSupplyStaging(ctx context.Context, supplies []*indexermodels.Supply) error {
	if len(supplies) == 0 {
		return nil
	}

	query := fmt.Sprintf(
		`INSERT INTO "%s".supply_staging (total, staked, delegated_only, height, height_time) VALUES`,
		db.Name,
	)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, supply := range supplies {
		err = batch.Append(
			supply.Total,
			supply.Staked,
			supply.DelegatedOnly,
			supply.Height,
			supply.HeightTime,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

package chain

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initDexPrices creates the dex_prices table and its staging table.
// This follows the same pattern as other indexer entities using ReplacingMergeTree
// for deduplication and efficient updates.
func (db *DB) initDexPrices(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.DexPriceColumns)

	// Production table: ORDER BY (local_chain_id, remote_chain_id, height) for efficient chain pair lookups
	productionQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (local_chain_id, remote_chain_id, height)
	`, db.Name, indexermodels.DexPricesProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.DexPricesProductionTableName, err)
	}

	// Staging table: ORDER BY (height, local_chain_id, remote_chain_id) for efficient cleanup/promotion WHERE height = ?
	stagingQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (height, local_chain_id, remote_chain_id)
	`, db.Name, indexermodels.DexPricesStagingTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.DexPricesStagingTableName, err)
	}

	return nil
}

// InsertDexPricesStaging inserts DEX prices into the dex_prices_staging table.
// This follows the two-phase commit pattern for data consistency.
func (db *DB) InsertDexPricesStaging(ctx context.Context, prices []*indexermodels.DexPrice) error {
	return db.insertDexPrices(ctx, indexermodels.DexPricesStagingTableName, prices)
}

// InsertDexPricesProduction inserts DEX prices into the dex_prices production table.
func (db *DB) InsertDexPricesProduction(ctx context.Context, prices []*indexermodels.DexPrice) error {
	return db.insertDexPrices(ctx, indexermodels.DexPricesProductionTableName, prices)
}

func (db *DB) insertDexPrices(ctx context.Context, tableName string, prices []*indexermodels.DexPrice) error {
	if len(prices) == 0 {
		return nil
	}

	query := fmt.Sprintf(
		`INSERT INTO "%s"."%s" (local_chain_id, remote_chain_id, height, local_pool, remote_pool, price_e6, height_time, price_delta, local_pool_delta, remote_pool_delta) VALUES`,
		db.Name,
		tableName,
	)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, price := range prices {
		err = batch.Append(
			price.LocalChainID,
			price.RemoteChainID,
			price.Height,
			price.LocalPool,
			price.RemotePool,
			price.PriceE6,
			price.HeightTime,
			price.PriceDelta,
			price.LocalPoolDelta,
			price.RemotePoolDelta,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

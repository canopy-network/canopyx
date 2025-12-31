package chain

import (
	"context"
	"fmt"

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
		CREATE TABLE IF NOT EXISTS "%s"."%s" %s (
			%s
		) ENGINE = %s
		ORDER BY (local_chain_id, remote_chain_id, height)
	`, db.Name, indexermodels.DexPricesProductionTableName, db.OnCluster(), schemaSQL, db.Engine(indexermodels.DexPricesProductionTableName, "ReplacingMergeTree", "height"))
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.DexPricesProductionTableName, err)
	}

	// Staging table: ORDER BY (height, local_chain_id, remote_chain_id) for efficient cleanup/promotion WHERE height = ?
	stagingQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" %s (
			%s
		) ENGINE = %s
		ORDER BY (height, local_chain_id, remote_chain_id)
	`, db.Name, indexermodels.DexPricesStagingTableName, db.OnCluster(), schemaSQL, db.Engine(indexermodels.DexPricesStagingTableName, "ReplacingMergeTree", "height"))
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.DexPricesStagingTableName, err)
	}

	return nil
}

// InsertDexPricesStaging inserts DEX prices into the dex_prices_staging table.
// This follows the two-phase commit pattern for data consistency.
func (db *DB) InsertDexPricesStaging(ctx context.Context, prices []*indexermodels.DexPrice) error {
	if len(prices) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."dex_prices_staging" (local_chain_id, remote_chain_id, height, local_pool, remote_pool, price_e6, height_time, price_delta, local_pool_delta, remote_pool_delta) VALUES`, db.Name)
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

// QueryDexPrices retrieves a paginated list of DEX prices ordered by height with optional chain pair filtering.
// If sortDesc is true, orders by height DESC (newest first), otherwise ASC (oldest first).
// If cursor > 0 and sortDesc is true, only prices with height < cursor are returned.
// If cursor > 0 and sortDesc is false, only prices with height > cursor are returned.
// If localChainID > 0 and remoteChainID > 0, filters to only prices for that specific chain pair.
// The limit parameter controls the maximum number of rows returned (+1 for pagination detection).

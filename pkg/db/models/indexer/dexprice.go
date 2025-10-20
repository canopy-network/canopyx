package indexer

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// DexPrice stores DEX price and liquidity pool information for chain pairs.
// Each record represents the state of a DEX pool at a specific block height.
// Uses ReplacingMergeTree to allow re-indexing and maintain latest state per (local_chain_id, remote_chain_id, height).
type DexPrice struct {
	// Primary key (composite)
	LocalChainID  uint64 `ch:"local_chain_id" json:"local_chain_id"`
	RemoteChainID uint64 `ch:"remote_chain_id" json:"remote_chain_id"`
	Height        uint64 `ch:"height" json:"height"`

	// Pool liquidity amounts
	LocalPool  uint64 `ch:"local_pool" json:"local_pool"`   // Liquidity in local chain tokens
	RemotePool uint64 `ch:"remote_pool" json:"remote_pool"` // Liquidity in remote chain tokens

	// Price information
	PriceE6 uint64 `ch:"price_e6" json:"price_e6"` // Price scaled by 1e6 (e.g., 500000 = 0.5)

	// Time-range query field
	HeightTime time.Time `ch:"height_time" json:"height_time"` // Block timestamp for time-range queries
}

// InitDexPrices creates the dex_prices table.
// This follows the same pattern as other indexer entities using ReplacingMergeTree
// for deduplication and efficient updates.
func InitDexPrices(ctx context.Context, db driver.Conn) error {
	query := `
		CREATE TABLE IF NOT EXISTS dex_prices (
			local_chain_id UInt64,
			remote_chain_id UInt64,
			height UInt64,
			local_pool UInt64,
			remote_pool UInt64,
			price_e6 UInt64,
			height_time DateTime64(6)
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (local_chain_id, remote_chain_id, height)
	`
	return db.Exec(ctx, query)
}

// InsertDexPricesStaging inserts dex prices to the staging table.
// Staging tables are used for new data before promotion to production.
// This follows the two-phase commit pattern for data consistency.
func InsertDexPricesStaging(ctx context.Context, db driver.Conn, tableName string, prices []*DexPrice) error {
	if len(prices) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO %s (local_chain_id, remote_chain_id, height, local_pool, remote_pool, price_e6, height_time) VALUES`, tableName)
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
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

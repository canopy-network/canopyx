package indexer

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Pool stores pool state snapshots at each height.
// Uses snapshot-on-change pattern: a new row is created only when pool state changes.
// ReplacingMergeTree deduplicates by (pool_id, height), keeping the latest state.
//
// Query patterns:
//   - Latest state: SELECT * FROM pools FINAL WHERE pool_id = ? ORDER BY height DESC LIMIT 1
//   - Historical state: SELECT * FROM pools FINAL WHERE pool_id = ? AND height <= ? ORDER BY height DESC LIMIT 1
//   - All pools at height: SELECT * FROM pools FINAL WHERE height <= ? GROUP BY pool_id HAVING height = max(height)
type Pool struct {
	// Primary key - composite key for deduplication
	PoolID uint64 `ch:"pool_id" json:"pool_id"`
	Height uint64 `ch:"height" json:"height"`

	// Chain identifier for multi-chain support
	ChainID uint64 `ch:"chain_id" json:"chain_id"`

	// Pool state fields
	Amount      uint64 `ch:"amount" json:"amount"`             // Total amount in pool
	TotalPoints uint64 `ch:"total_points" json:"total_points"` // Total pool points
	LPCount     uint32 `ch:"lp_count" json:"lp_count"`         // Number of liquidity providers

	// Time fields for range queries
	HeightTime time.Time `ch:"height_time" json:"height_time"` // Block timestamp
}

// InitPools creates the pools table with ReplacingMergeTree engine.
// Uses height as the deduplication version key.
// The table stores pool state snapshots that change at each height.
func InitPools(ctx context.Context, db driver.Conn) error {
	query := `
		CREATE TABLE IF NOT EXISTS pools (
			pool_id UInt64,
			height UInt64,
			chain_id UInt64,
			amount UInt64,
			total_points UInt64,
			lp_count UInt32,
			height_time DateTime64(6)
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (pool_id, height)
	`
	return db.Exec(ctx, query)
}

// InsertPoolsStaging inserts pools to the staging table.
// Staging tables are used for new data before promotion to production.
// This follows the two-phase commit pattern for data consistency.
func InsertPoolsStaging(ctx context.Context, db driver.Conn, tableName string, pools []*Pool) error {
	if len(pools) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO %s (pool_id, height, chain_id, amount, total_points, lp_count, height_time) VALUES`, tableName)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, pool := range pools {
		err = batch.Append(
			pool.PoolID,
			pool.Height,
			pool.ChainID,
			pool.Amount,
			pool.TotalPoints,
			pool.LPCount,
			pool.HeightTime,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

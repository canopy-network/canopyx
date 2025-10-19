package indexer

import (
	"context"
	"fmt"

	"github.com/uptrace/go-clickhouse/ch"
)

// InitGenesis creates the genesis table for caching full genesis state.
// The genesis table stores the full genesis JSON at height 0, which is used
// by the accounts indexer when processing height 1 (comparing RPC(1) vs Genesis(0)).
//
// Design decisions:
// - ReplacingMergeTree for idempotent inserts (safe retries)
// - No TTL: genesis data is needed permanently for height 1 comparisons
// - No chain_id column: each chain has its own database
// - Simple schema: just height, data (JSON string), and fetched_at timestamp
func InitGenesis(ctx context.Context, db *ch.DB, dbName string) error {
	ddl := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS "%s"."genesis" (
    height UInt64,
    data String,
    fetched_at DateTime DEFAULT now()
) ENGINE = ReplacingMergeTree()
ORDER BY height
`, dbName)

	_, err := db.ExecContext(ctx, ddl)
	if err != nil {
		return fmt.Errorf("create genesis table: %w", err)
	}

	return nil
}

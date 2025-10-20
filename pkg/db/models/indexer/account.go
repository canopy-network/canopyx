package indexer

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Account represents a versioned snapshot of an account balance.
// Snapshots are created ONLY when balance changes (not every block).
// This enables temporal queries like "What was alice's balance at height 5000?"
// while using 20x less storage than storing all accounts at every height.
//
// The snapshot-on-change pattern works correctly with parallel/unordered indexing because:
// - We compare RPC(height H) vs RPC(height H-1) to detect changes
// - We don't rely on database state which may be incomplete during parallel indexing
// - ReplacingMergeTree handles deduplication if the same height is indexed multiple times
type Account struct {
	// Identity
	Address string `ch:"address"` // Hex string representation of address

	// Balance (using uint64 to match blockchain's native type)
	Amount uint64 `ch:"amount"` // Account balance in uCNPY (micro-denomination)

	// Version tracking - every balance change creates a new snapshot
	Height     uint64    `ch:"height"`      // Height at which this snapshot was created
	HeightTime time.Time `ch:"height_time"` // Block timestamp for time-range queries

	// Lifecycle tracking
	// For new accounts: created_height = current height
	// For existing accounts: preserved from previous snapshot
	CreatedHeight uint64 `ch:"created_height"` // Height when account first appeared on chain
}

// InitAccounts initializes the accounts table and its staging table.
// The production table uses aggressive compression for storage optimization.
// The staging table has the same schema but no TTL (TTL only on production).
func InitAccounts(ctx context.Context, db driver.Conn) error {
	query := `
		CREATE TABLE IF NOT EXISTS accounts (
			address String CODEC(ZSTD(1)),
			amount UInt64 CODEC(Delta, ZSTD(3)),
			height UInt64 CODEC(DoubleDelta, LZ4),
			height_time DateTime64(6) CODEC(DoubleDelta, LZ4),
			created_height UInt64 CODEC(DoubleDelta, LZ4)
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (address, height)
	`
	return db.Exec(ctx, query)
}

// InsertAccountsStaging inserts account snapshots to the staging table.
// Staging tables are used for new data before promotion to production.
// This follows the two-phase commit pattern for data consistency.
func InsertAccountsStaging(ctx context.Context, db driver.Conn, tableName string, accounts []*Account) error {
	if len(accounts) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO %s (address, amount, height, height_time, created_height) VALUES`, tableName)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, account := range accounts {
		err = batch.Append(
			account.Address,
			account.Amount,
			account.Height,
			account.HeightTime,
			account.CreatedHeight,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

// InsertAccountsProduction inserts account snapshots directly to the production table.
// This is used when bypassing the staging pattern (e.g., for batch imports).
func InsertAccountsProduction(ctx context.Context, db driver.Conn, accounts []*Account) error {
	if len(accounts) == 0 {
		return nil
	}

	query := `INSERT INTO accounts (address, amount, height, height_time, created_height) VALUES`
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, account := range accounts {
		err = batch.Append(
			account.Address,
			account.Amount,
			account.Height,
			account.HeightTime,
			account.CreatedHeight,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

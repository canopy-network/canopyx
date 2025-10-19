package indexer

import (
	"context"
	"time"

	"github.com/uptrace/go-clickhouse/ch"
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
	ch.CHModel `ch:"table:accounts,engine:ReplacingMergeTree(height),order_by:(address,height)"`

	// Identity
	Address string `ch:"address,pk,codec:ZSTD(1)"` // Hex string representation of address

	// Balance (using uint64 to match blockchain's native type)
	Amount uint64 `ch:"amount,codec:Delta,ZSTD(3)"` // Account balance in uCNPY (micro-denomination)

	// Version tracking - every balance change creates a new snapshot
	Height     uint64    `ch:"height,pk,codec:DoubleDelta,LZ4"` // Height at which this snapshot was created
	HeightTime time.Time `ch:"height_time,type:DateTime64(6),codec:DoubleDelta,LZ4"` // Block timestamp for time-range queries

	// Lifecycle tracking
	// For new accounts: created_height = current height
	// For existing accounts: preserved from previous snapshot
	CreatedHeight uint64 `ch:"created_height,codec:DoubleDelta,LZ4"` // Height when account first appeared on chain
}

// InitAccounts initializes the accounts table and its staging table.
// The production table uses aggressive compression for storage optimization.
// The staging table has the same schema but no TTL (TTL only on production).
func InitAccounts(ctx context.Context, db *ch.DB) error {
	// Create production table using CHModel builder
	if _, err := db.NewCreateTable().
		Model((*Account)(nil)).
		IfNotExists().
		Exec(ctx); err != nil {
		return err
	}

	return nil
}

// InsertAccountsStaging inserts account snapshots to the staging table.
// Staging tables are used for new data before promotion to production.
// This follows the two-phase commit pattern for data consistency.
func InsertAccountsStaging(ctx context.Context, db *ch.DB, tableName string, accounts []*Account) error {
	if len(accounts) == 0 {
		return nil
	}
	_, err := db.NewInsert().
		Model(&accounts).
		Table(tableName).
		Exec(ctx)
	return err
}

// InsertAccountsProduction inserts account snapshots directly to the production table.
// This is used when bypassing the staging pattern (e.g., for batch imports).
func InsertAccountsProduction(ctx context.Context, db *ch.DB, accounts []*Account) error {
	if len(accounts) == 0 {
		return nil
	}
	_, err := db.NewInsert().Model(&accounts).Exec(ctx)
	return err
}
package indexer

import (
	"time"
)

const AccountsProductionTableName = "accounts"
const AccountsStagingTableName = AccountsProductionTableName + "_staging"

// Account represents a versioned snapshot of an account balance.
// Snapshots are created ONLY when the balance changes (not every block).
// This enables temporal queries like "What was alice's balance at height 5000?"
// while using 20x less storage than storing all accounts at every height.
//
// The snapshot-on-change pattern works correctly with parallel/unordered indexing because:
// - We compare RPC(height H) vs RPC(height H-1) to detect changes
// - We don't rely on database state which may be incomplete during parallel indexing
// - ReplacingMergeTree handles deduplication if the same height is indexed multiple times
//
// Note: Account creation time is tracked via the account_created_height materialized view,
// which calculates MIN(height) for each address. Consumers should JOIN with this view
// if they need to know when an account was created.
type Account struct {
	// Identity
	Address string `ch:"address"` // Hex string representation of address

	// Balance (using uint64 to match the blockchain's native type)
	Amount uint64 `ch:"amount"` // Account balance in uCNPY (micro-denomination)

	// Version tracking - every balance change creates a new snapshot
	Height     uint64    `ch:"height"`      // Height at which this snapshot was created
	HeightTime time.Time `ch:"height_time"` // Block timestamp for time-range queries
}

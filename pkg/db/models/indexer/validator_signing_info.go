package indexer

import (
	"time"
)

const ValidatorSigningInfoProductionTableName = "validator_signing_info"
const ValidatorSigningInfoStagingTableName = "validator_signing_info_staging"

// ValidatorSigningInfo represents a versioned snapshot of a validator's signing performance.
// This tracks non-signing counters for validators within a sliding window.
// Snapshots are created using the snapshot-on-change pattern: a new row is created
// only when the signing info changes (counter increments or window advances).
//
// The snapshot-on-change pattern works correctly with parallel/unordered indexing because:
// - We compare RPC(height H) vs RPC(height H-1) to detect changes
// - We don't rely on database state which may be incomplete during parallel indexing
// - ReplacingMergeTree handles deduplication if the same height is indexed multiple times
//
// Data Source: Canopy /v1/query/non-signers endpoint returns RpcNonSigner with:
// - Address: validator address
// - Counter: number of missed blocks in the current window
// - WindowStart: must be derived as (CurrentHeight - NonSignWindow param)
//
// This data is correlated with validator events like:
// - EventAutoPause: when a validator is automatically paused for missing too many blocks
// - EventAutoBeginUnstaking: when a validator is automatically unstaked
// - EventFinishUnstaking: when unstaking completes
type ValidatorSigningInfo struct {
	// Identity
	Address string `ch:"address" json:"address"` // Hex string representation of validator address

	// Signing performance tracking (from Canopy RpcNonSigner)
	MissedBlocksCount uint64 `ch:"missed_blocks_count" json:"missed_blocks_count"` // Maps from RpcNonSigner.Counter - count of missed blocks in current window

	// Derived window tracking (not from RPC - computed from chain params)
	MissedBlocksWindow uint64 `ch:"missed_blocks_window" json:"missed_blocks_window"` // Computed as: CurrentHeight - NonSignWindow (from validator params)

	// Additional tracking fields (if available from other sources)
	LastSignedHeight uint64 `ch:"last_signed_height" json:"last_signed_height"` // Last height at which validator signed (if tracked)
	StartHeight      uint64 `ch:"start_height" json:"start_height"`             // Height at which validator started (if tracked)

	// Version tracking - every change creates a new snapshot
	Height     uint64    `ch:"height" json:"height"`           // Height at which this snapshot was created
	HeightTime time.Time `ch:"height_time" json:"height_time"` // Block timestamp for time-range queries
}

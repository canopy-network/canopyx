package indexer

import (
	"time"
)

const CommitteeValidatorProductionTableName = "committee_validators"
const CommitteeValidatorStagingTableName = "committee_validators_staging"

// CommitteeValidator represents the junction table between committees and validators.
// This denormalized table enables efficient queries like:
// - "Get all validators in committee X at height Y"
// - "Get all committees that validator X belongs to at height Y"
//
// While the Validator entity already contains a Committees array, this junction table
// provides O(1) lookups when querying by committee_id, which is critical for:
// - Committee dashboard views showing all members
// - Historical committee composition analysis
// - Validator set queries for specific chains
//
// Snapshots are created using the snapshot-on-change pattern: a new row is created
// only when a validator's committee membership changes.
//
// Note: This table is derived from the Validators.Committees array field.
// It should be populated during IndexValidators activity when committee membership changes.
type CommitteeValidator struct {
	// Relationship
	CommitteeID      uint64 `ch:"committee_id"`      // Committee (chain) ID
	ValidatorAddress string `ch:"validator_address"` // Hex string representation of validator address

	// Validator metadata (denormalized for query efficiency)
	StakedAmount uint64 `ch:"staked_amount"` // Amount staked by validator in uCNPY
	Status       string `ch:"status"`        // Validator status: active/paused/unstaking (derived from MaxPausedHeight/UnstakingHeight)

	// Version tracking - every membership change creates a new snapshot
	Height     uint64    `ch:"height"`      // Height at which this snapshot was created
	HeightTime time.Time `ch:"height_time"` // Block timestamp for time-range queries
}

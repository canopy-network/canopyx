package indexer

import "time"

const (
	// CommitteeProductionTableName is the production table name for committees
	CommitteeProductionTableName = "committees"
	// CommitteeStagingTableName is the staging table name for committees
	CommitteeStagingTableName = "committees_staging"
)

// Committee represents committee data at a specific height.
// A committee is a group of validators responsible for a specific chain.
// Committees are only inserted when their data changes to maintain a sparse historical record.
type Committee struct {
	// Primary identifier
	ChainID uint64 `ch:"chain_id"` // Unique identifier of the chain/committee

	// Committee state tracking
	LastRootHeightUpdated  uint64 `ch:"last_root_height_updated"`  // Canopy height of most recent Certificate Results tx
	LastChainHeightUpdated uint64 `ch:"last_chain_height_updated"` // 3rd party chain height of most recent Certificate Results
	NumberOfSamples        uint64 `ch:"number_of_samples"`         // Count of processed Certificate Result Transactions

	// Status flags
	Subsidized bool `ch:"subsidized"` // Whether committee meets stake requirement for subsidies
	Retired    bool `ch:"retired"`    // Whether committee has shut down and is forever unsubsidized

	// Block context
	Height     uint64    `ch:"height"`      // Block height when this committee state became effective
	HeightTime time.Time `ch:"height_time"` // Timestamp of the block
}

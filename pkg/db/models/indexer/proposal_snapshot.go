package indexer

import (
	"time"
)

const ProposalSnapshotsProductionTableName = "proposal_snapshots"

// ProposalSnapshotColumns defines the schema for the proposal_snapshots table.
// NOTE: Proposal snapshots are NOT height-based since /v1/gov/proposals endpoint doesn't support height queries.
// Snapshots are time-based and inserted via scheduled workflow every 5 minutes.
// Codecs are optimized for compression:
// - DoubleDelta,LZ4 for monotonic timestamps
// - ZSTD(1) for strings (proposal hash and JSON data)
// - ZSTD(1) for boolean (approval flag)
var ProposalSnapshotColumns = []ColumnDef{
	{Name: "proposal_hash", Type: "String", Codec: "ZSTD(1)"},
	{Name: "proposal", Type: "String", Codec: "ZSTD(1)"}, // JSON proposal data
	{Name: "approve", Type: "Bool", Codec: "ZSTD(1)"},    // Node approval flag
	{Name: "snapshot_time", Type: "DateTime64(6)", Codec: "DoubleDelta, LZ4"},
}

// ProposalSnapshot stores time-series snapshots of governance proposals.
// Unlike other entities, proposal snapshots are NOT height-based because the /v1/gov/proposals
// endpoint does not support height parameters - it always returns current state.
//
// Snapshots are captured via a scheduled workflow that runs every 5 minutes.
// ReplacingMergeTree deduplicates by (proposal_hash, snapshot_time), keeping the latest state.
//
// Query patterns:
//   - Latest proposal state: SELECT * FROM proposal_snapshots FINAL WHERE proposal_hash = ? ORDER BY snapshot_time DESC LIMIT 1
//   - Historical state: SELECT * FROM proposal_snapshots FINAL WHERE proposal_hash = ? AND snapshot_time <= ? ORDER BY snapshot_time DESC LIMIT 1
//   - All proposals at time: SELECT DISTINCT proposal_hash FROM proposal_snapshots FINAL WHERE snapshot_time = ?
type ProposalSnapshot struct {
	// Primary key - composite key for deduplication
	ProposalHash string `ch:"proposal_hash" json:"proposal_hash"` // Hex hash of proposal (transaction hash)

	// Proposal data
	Proposal string `ch:"proposal" json:"proposal"` // JSON-encoded proposal (from fsm.GovProposalWithVote.Proposal)
	Approve  bool   `ch:"approve" json:"approve"`   // Whether this node approves the proposal

	// Time field for snapshot tracking
	SnapshotTime time.Time `ch:"snapshot_time" json:"snapshot_time"` // When this snapshot was captured
}

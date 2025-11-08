package indexer

import (
	"time"
)

const PollSnapshotsProductionTableName = "poll_snapshots"
const PollSnapshotsStagingTableName = "poll_snapshots_staging"

// PollSnapshotColumns defines the schema for the poll_snapshots table.
var PollSnapshotColumns = []ColumnDef{
	{Name: "proposal_hash", Type: "String", Codec: "ZSTD(1)"},
	{Name: "height", Type: "UInt64", Codec: "Delta, ZSTD(1)"},
	{Name: "proposal_url", Type: "String", Codec: "ZSTD(1)"},
	{Name: "accounts_approve_tokens", Type: "UInt64", Codec: "Delta, ZSTD(1)"},
	{Name: "accounts_reject_tokens", Type: "UInt64", Codec: "Delta, ZSTD(1)"},
	{Name: "accounts_total_voted_tokens", Type: "UInt64", Codec: "Delta, ZSTD(1)"},
	{Name: "accounts_total_tokens", Type: "UInt64", Codec: "Delta, ZSTD(1)"},
	{Name: "accounts_approve_percentage", Type: "UInt64", Codec: "Delta, ZSTD(1)"},
	{Name: "accounts_reject_percentage", Type: "UInt64", Codec: "Delta, ZSTD(1)"},
	{Name: "accounts_voted_percentage", Type: "UInt64", Codec: "Delta, ZSTD(1)"},
	{Name: "validators_approve_tokens", Type: "UInt64", Codec: "Delta, ZSTD(1)"},
	{Name: "validators_reject_tokens", Type: "UInt64", Codec: "Delta, ZSTD(1)"},
	{Name: "validators_total_voted_tokens", Type: "UInt64", Codec: "Delta, ZSTD(1)"},
	{Name: "validators_total_tokens", Type: "UInt64", Codec: "Delta, ZSTD(1)"},
	{Name: "validators_approve_percentage", Type: "UInt64", Codec: "Delta, ZSTD(1)"},
	{Name: "validators_reject_percentage", Type: "UInt64", Codec: "Delta, ZSTD(1)"},
	{Name: "validators_voted_percentage", Type: "UInt64", Codec: "Delta, ZSTD(1)"},
	{Name: "height_time", Type: "DateTime64(6)", Codec: "Delta, ZSTD(1)"},
}

// PollSnapshot stores historical poll results for governance proposals at each height.
// Uses snapshot-on-change pattern: a new row is created only when poll state changes for a proposal.
// ReplacingMergeTree deduplicates by (proposal_hash, height), keeping the latest state.
//
// ARCHITECTURAL NOTE: Unlike other entities that use RPC(H) vs RPC(H-1) comparison,
// the /v1/gov/poll endpoint does NOT support height parameters. It always returns current state.
// Therefore, we snapshot the current poll state at each height instead of detecting changes.
//
// Query patterns:
//   - Latest poll state: SELECT * FROM poll_snapshots FINAL WHERE proposal_hash = ? ORDER BY height DESC LIMIT 1
//   - Historical state: SELECT * FROM poll_snapshots FINAL WHERE proposal_hash = ? AND height <= ? ORDER BY height DESC LIMIT 1
//   - All proposals at height: SELECT DISTINCT proposal_hash FROM poll_snapshots FINAL WHERE height = ?
type PollSnapshot struct {
	// Primary key - composite key for deduplication
	ProposalHash string `ch:"proposal_hash" json:"proposal_hash"` // Hex hash of proposal

	// Proposal metadata
	ProposalURL string `ch:"proposal_url" json:"proposal_url"` // URL to proposal document

	// Account voting statistics
	AccountsApproveTokens     uint64 `ch:"accounts_approve_tokens" json:"accounts_approve_tokens"`
	AccountsRejectTokens      uint64 `ch:"accounts_reject_tokens" json:"accounts_reject_tokens"`
	AccountsTotalVotedTokens  uint64 `ch:"accounts_total_voted_tokens" json:"accounts_total_voted_tokens"`
	AccountsTotalTokens       uint64 `ch:"accounts_total_tokens" json:"accounts_total_tokens"`
	AccountsApprovePercentage uint64 `ch:"accounts_approve_percentage" json:"accounts_approve_percentage"`
	AccountsRejectPercentage  uint64 `ch:"accounts_reject_percentage" json:"accounts_reject_percentage"`
	AccountsVotedPercentage   uint64 `ch:"accounts_voted_percentage" json:"accounts_voted_percentage"`

	// Validator voting statistics
	ValidatorsApproveTokens     uint64 `ch:"validators_approve_tokens" json:"validators_approve_tokens"`
	ValidatorsRejectTokens      uint64 `ch:"validators_reject_tokens" json:"validators_reject_tokens"`
	ValidatorsTotalVotedTokens  uint64 `ch:"validators_total_voted_tokens" json:"validators_total_voted_tokens"`
	ValidatorsTotalTokens       uint64 `ch:"validators_total_tokens" json:"validators_total_tokens"`
	ValidatorsApprovePercentage uint64 `ch:"validators_approve_percentage" json:"validators_approve_percentage"`
	ValidatorsRejectPercentage  uint64 `ch:"validators_reject_percentage" json:"validators_reject_percentage"`
	ValidatorsVotedPercentage   uint64 `ch:"validators_voted_percentage" json:"validators_voted_percentage"`

	Height uint64 `ch:"height" json:"height"` // REMOVE
	// Time field for range queries
	HeightTime time.Time `ch:"height_time" json:"height_time"` // REMOVE
}

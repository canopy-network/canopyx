package crosschain

import (
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// Cross-chain wrapper types that include chain_id and updated_at fields.
// These are used for scanning query results from global tables.
// Global tables add: chain_id (for multi-chain tracking) and updated_at (sync timestamp).
//
// The embedded indexer types provide all the entity-specific fields,
// and Go's struct embedding promotes those fields for direct access.

type AccountCrossChain struct {
	ChainID uint64 `ch:"chain_id" json:"chain_id"`
	indexer.Account
	UpdatedAt time.Time `ch:"updated_at" json:"updated_at"`
}

type ValidatorCrossChain struct {
	ChainID uint64 `ch:"chain_id" json:"chain_id"`
	indexer.Validator
	UpdatedAt time.Time `ch:"updated_at" json:"updated_at"`
}

type ValidatorSigningInfoCrossChain struct {
	ChainID uint64 `ch:"chain_id" json:"chain_id"`
	indexer.ValidatorSigningInfo
	UpdatedAt time.Time `ch:"updated_at" json:"updated_at"`
}

type ValidatorDoubleSigningInfoCrossChain struct {
	ChainID uint64 `ch:"chain_id" json:"chain_id"`
	indexer.ValidatorDoubleSigningInfo
	UpdatedAt time.Time `ch:"updated_at" json:"updated_at"`
}

type PoolCrossChain struct {
	// Cross-chain tracking column
	ChainID uint64 `ch:"chain_id" json:"chain_id"`

	// Pool fields - note that Pool.ChainID becomes pool_chain_id in cross-chain tables
	// This is not using the embedded struct to avoid that collision with chain_id
	PoolID          uint64    `ch:"pool_id" json:"pool_id"`
	Height          uint64    `ch:"height" json:"height"`
	PoolChainID     uint64    `ch:"pool_chain_id" json:"pool_chain_id"` // Renamed from Pool.ChainID
	Amount          uint64    `ch:"amount" json:"amount"`
	TotalPoints     uint64    `ch:"total_points" json:"total_points"`
	LPCount         uint32    `ch:"lp_count" json:"lp_count"`
	HeightTime      time.Time `ch:"height_time" json:"height_time"`
	LiquidityPoolID uint64    `ch:"liquidity_pool_id" json:"liquidity_pool_id"`
	HoldingPoolID   uint64    `ch:"holding_pool_id" json:"holding_pool_id"`
	EscrowPoolID    uint64    `ch:"escrow_pool_id" json:"escrow_pool_id"`
	RewardPoolID    uint64    `ch:"reward_pool_id" json:"reward_pool_id"`

	UpdatedAt time.Time `ch:"updated_at" json:"updated_at"`
}

type PoolPointsByHolderCrossChain struct {
	ChainID uint64 `ch:"chain_id" json:"chain_id"`
	indexer.PoolPointsByHolder
	UpdatedAt time.Time `ch:"updated_at" json:"updated_at"`
}

type OrderCrossChain struct {
	ChainID uint64 `ch:"chain_id" json:"chain_id"`
	indexer.Order
	UpdatedAt time.Time `ch:"updated_at" json:"updated_at"`
}

type DexOrderCrossChain struct {
	ChainID uint64 `ch:"chain_id" json:"chain_id"`
	indexer.DexOrder
	UpdatedAt time.Time `ch:"updated_at" json:"updated_at"`
}

type DexDepositCrossChain struct {
	ChainID uint64 `ch:"chain_id" json:"chain_id"`
	indexer.DexDeposit
	UpdatedAt time.Time `ch:"updated_at" json:"updated_at"`
}

type DexWithdrawalCrossChain struct {
	ChainID uint64 `ch:"chain_id" json:"chain_id"`
	indexer.DexWithdrawal
	UpdatedAt time.Time `ch:"updated_at" json:"updated_at"`
}

type BlockSummaryCrossChain struct {
	ChainID uint64 `ch:"chain_id" json:"chain_id"`
	indexer.BlockSummary
	UpdatedAt time.Time `ch:"updated_at" json:"updated_at"`
}

// QueryOptions contains parameters for cross-chain entity queries.
type QueryOptions struct {
	// ChainIDs filters results to specific chains. Empty means all chains.
	ChainIDs []uint64

	// Filters contains field-based filters (e.g., {"address": "abc123", "status": "active"})
	// Supports exact match filtering on any column.
	Filters map[string]string

	// Limit is the maximum number of records to return.
	// Default: 50, Max: 1000
	Limit int

	// Offset is the number of records to skip for pagination.
	// Default: 0
	Offset uint64

	// OrderBy is the column name to sort by.
	// Default: "height"
	OrderBy string

	// Desc controls sort direction. true = descending, false = ascending.
	// Default: true (newest first)
	Desc bool
}

// QueryMetadata contains metadata about query results.
type QueryMetadata struct {
	// Total is the total number of matching records (before pagination).
	Total uint64 `json:"total"`

	// HasMore indicates whether more records exist beyond the current limit.
	HasMore bool `json:"has_more"`

	// Limit is the limit that was applied to this query.
	Limit int `json:"limit"`

	// Offset is the offset that was applied to this query.
	Offset uint64 `json:"offset"`

	// ChainStats contains per-chain record counts (optional, populated if requested).
	ChainStats []ChainStats `json:"chain_stats,omitempty"`
}

// ChainStats contains statistics for a specific chain.
type ChainStats struct {
	ChainID uint64 `json:"chain_id"`
	Count   uint64 `json:"count"`
}

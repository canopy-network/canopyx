package indexer

import (
	"time"
)

const DexPricesProductionTableName = "dex_prices"
const DexPricesStagingTableName = "dex_prices_staging"

// DexPriceColumns defines the schema for the dex_prices table.
var DexPriceColumns = []ColumnDef{
	{Name: "local_chain_id", Type: "UInt64"},
	{Name: "remote_chain_id", Type: "UInt64"},
	{Name: "height", Type: "UInt64"},
	{Name: "local_pool", Type: "UInt64"},
	{Name: "remote_pool", Type: "UInt64"},
	{Name: "price_e6", Type: "UInt64"},
	{Name: "height_time", Type: "DateTime64(6)"},
	{Name: "price_delta", Type: "Int64", Codec: "Delta, ZSTD(3)"},
	{Name: "local_pool_delta", Type: "Int64", Codec: "Delta, ZSTD(3)"},
	{Name: "remote_pool_delta", Type: "Int64", Codec: "Delta, ZSTD(3)"},
}

// DexPrice stores DEX price and liquidity pool information for chain pairs.
// Each record represents the state of a DEX pool at a specific block height.
// Uses ReplacingMergeTree to allow re-indexing and maintain latest state per (local_chain_id, remote_chain_id, height).
type DexPrice struct {
	// Primary key (composite)
	LocalChainID  uint64 `ch:"local_chain_id" json:"local_chain_id"`
	RemoteChainID uint64 `ch:"remote_chain_id" json:"remote_chain_id"`

	// Pool liquidity amounts
	LocalPool  uint64 `ch:"local_pool" json:"local_pool"`   // Liquidity in local chain tokens
	RemotePool uint64 `ch:"remote_pool" json:"remote_pool"` // Liquidity in remote chain tokens

	// Price information
	PriceE6 uint64 `ch:"price_e6" json:"price_e6"` // Price scaled by 1e6 (e.g., 500000 = 0.5)

	// H-1 delta fields (change from previous height)
	PriceDelta      int64 `ch:"price_delta" json:"price_delta"`             // Change in price from H-1 to H
	LocalPoolDelta  int64 `ch:"local_pool_delta" json:"local_pool_delta"`   // Change in local pool from H-1 to H
	RemotePoolDelta int64 `ch:"remote_pool_delta" json:"remote_pool_delta"` // Change in remote pool from H-1 to H

	Height uint64 `ch:"height" json:"height"`
	// Time-range query field
	HeightTime time.Time `ch:"height_time" json:"height_time"` // Block timestamp for time-range queries
}

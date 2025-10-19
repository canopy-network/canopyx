package types

import (
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// BlockIndexedEvent represents a block that has been fully indexed and promoted to production.
// This event is published to Redis Pub/Sub after the index_progress watermark is updated,
// ensuring that all data is queryable when the event is received.
type BlockIndexedEvent struct {
	Event     string               `json:"event"`     // Always "block.indexed"
	ChainID   string               `json:"chainId"`   // Chain identifier
	Height    uint64               `json:"height"`    // Block height
	Timestamp time.Time            `json:"timestamp"` // Event publication time (UTC)
	Block     BlockInfo            `json:"block"`     // Block details
	Summary   indexer.BlockSummary `json:"summary"`   // Complete block summary with entity counts
}

// BlockInfo contains key block metadata included in the event.
type BlockInfo struct {
	Hash            string    `json:"hash"`            // Block hash
	Time            time.Time `json:"time"`            // Block timestamp
	ProposerAddress string    `json:"proposerAddress"` // Block proposer
}

// GetChannel returns the Redis Pub/Sub channel name for a given chain and event type.
// Channel format: canopy:{chainId}:{eventType}
// Example: canopy:mainnet:block.indexed
func GetChannel(chainID, eventType string) string {
	return "canopy:" + chainID + ":" + eventType
}

// GetBlockIndexedChannel returns the Redis channel for block.indexed events.
func GetBlockIndexedChannel(chainID string) string {
	return GetChannel(chainID, "block.indexed")
}

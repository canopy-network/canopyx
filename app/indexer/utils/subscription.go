package utils

import "fmt"

// GetChannel returns the Redis Pub/Sub channel name for a given chain and event type.
// Channel format: canopy:{chainId}:{eventType}
// Example: canopy:1:block.indexed
func GetChannel(chainID uint64, eventType string) string {
	return fmt.Sprintf("canopy:%d:%s", chainID, eventType)
}

// GetBlockIndexedChannel returns the Redis channel for block.indexed events.
func GetBlockIndexedChannel(chainID uint64) string {
	return GetChannel(chainID, "block.indexed")
}

// Stream name constants for Redis Streams
const (
	// StreamBlockIndexed is the Redis stream for block.indexed events.
	// Unlike Pub/Sub channels (per-chain), we use a single stream for all chains
	// to simplify consumer group management. Chain ID is included in each entry.
	StreamBlockIndexed = "canopy:stream:block.indexed"
)

// GetStream returns the Redis stream name for a given event type.
func GetStream(eventType string) string {
	return fmt.Sprintf("canopy:stream:%s", eventType)
}

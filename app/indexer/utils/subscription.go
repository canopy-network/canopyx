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

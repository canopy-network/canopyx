package utils

import "fmt"

// ---- Redis Pub/Sub Channels ----

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

// ---- Redis Streams ----

// Stream names for durable message delivery
const (
	// BlockIndexedStreamPrefix is the prefix for block indexed streams.
	// Full stream name: canopy:stream:{chainId}:block.indexed
	BlockIndexedStreamPrefix = "canopy:stream"

	// DefaultStreamMaxLen is the default maximum length for streams.
	// Older entries are trimmed when this limit is exceeded.
	DefaultStreamMaxLen = 10000
)

// GetStreamName returns the Redis Stream name for a given chain and event type.
// Stream format: canopy:stream:{chainId}:{eventType}
// Example: canopy:stream:1:block.indexed
func GetStreamName(chainID uint64, eventType string) string {
	return fmt.Sprintf("%s:%d:%s", BlockIndexedStreamPrefix, chainID, eventType)
}

// GetBlockIndexedStream returns the Redis Stream name for block.indexed events.
func GetBlockIndexedStream(chainID uint64) string {
	return GetStreamName(chainID, "block.indexed")
}

// GetConsumerGroupName returns the consumer group name for a given purpose.
// Example: canopy:consumers:explorer
func GetConsumerGroupName(purpose string) string {
	return fmt.Sprintf("canopy:consumers:%s", purpose)
}

package utils

const (
	// LiveBlockThreshold defines the number of blocks from the chain head
	// that are considered "live" for queue routing purposes.
	// Blocks within this threshold are routed to the live queue for low-latency indexing.
	// Blocks beyond this threshold are routed to the historical queue for bulk processing.
	LiveBlockThreshold = 200 // Blocks within 200 of head are considered "live"
)

// IsLiveBlock determines if a block should be routed to the live queue.
// Blocks within LiveBlockThreshold of the chain head are considered live.
// This function is used to route blocks to either the live queue (for low-latency indexing)
// or the historical queue (for bulk processing).
func IsLiveBlock(latest, height uint64) bool {
	if height > latest {
		return true // Future blocks are always considered live
	}
	return latest-height <= LiveBlockThreshold
}

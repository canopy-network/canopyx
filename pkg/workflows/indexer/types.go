package indexer

// Input types for triggering workflows from other apps

type ChainIdInput struct {
	ChainID uint64
}

type IndexBlockInput struct {
	ChainID     uint64
	Height      uint64
	Reindex     bool
	PriorityKey string
}

type HeadScanInput struct {
	ChainID uint64
}

type GapScanInput struct {
	ChainID uint64
}

type SchedulerInput struct {
	ChainID uint64
}

// IsLiveBlock determines if a block is recent enough for live queue
func IsLiveBlock(latestHeight, blockHeight uint64) bool {
	const LiveBlockThreshold = 200
	if blockHeight > latestHeight {
		return true // Future blocks are always considered live
	}
	return latestHeight-blockHeight <= LiveBlockThreshold
}

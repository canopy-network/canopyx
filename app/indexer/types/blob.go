package types

import "github.com/canopy-network/canopy/fsm"

// ActivityFetchBlobOutput contains indexer blobs fetched from RPC and execution duration.
type ActivityFetchBlobOutput struct {
	Blobs      *fsm.IndexerBlobs `json:"blobs"`
	DurationMs float64           `json:"durationMs"`
}

// ActivityIndexBlockFromBlobInput provides the data needed to index from a blob.
type ActivityIndexBlockFromBlobInput struct {
	Height            uint64            `json:"height"`
	Blobs             *fsm.IndexerBlobs `json:"blobs"`
	PrepareDurationMs float64           `json:"prepareDurationMs"`
	FetchDurationMs   float64           `json:"fetchDurationMs"`
}

package types

import (
	"time"

	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/rpc"
)

type WorkflowIndexBlockInput struct {
	Height      uint64 `json:"height"`
	PriorityKey string `json:"priorityKey"`
	Reindex     bool   `json:"reindex"`
}

type ActivityIndexBlockInput struct {
	Height      uint64 `json:"height"`
	PriorityKey string `json:"priorityKey"`
	Reindex     bool   `json:"reindex"`
}

// ActivityFetchBlockInput represents the input required to fetch a block by its height from an RPC endpoint.
// Height specifies the block height to be fetched.
type ActivityFetchBlockInput struct {
	Height uint64 `json:"height"`
}

// ActivityFetchBlockOutput contains the fetched block data along with execution duration.
// This is used by the FetchBlockFromRPC local activity.
type ActivityFetchBlockOutput struct {
	Block      *rpc.BlockByHeight `json:"block"`      // The fetched block
	DurationMs float64            `json:"durationMs"` // Execution time in milliseconds
}

// ActivitySaveBlockInput contains the parameters for saving a block to the database.
type ActivitySaveBlockInput struct {
	Height uint64             `json:"height"`
	Block  *rpc.BlockByHeight `json:"block"` // The block to save
}

// ActivitySaveBlockOutput contains the indexed block height along with execution duration.
type ActivitySaveBlockOutput struct {
	Height     uint64  `json:"height"`     // The indexed block height
	DurationMs float64 `json:"durationMs"` // Execution time in milliseconds
}

// ActivitySaveBlockSummaryInput contains the parameters for saving block summaries.
type ActivitySaveBlockSummaryInput struct {
	Height    uint64    `json:"height"`
	BlockTime time.Time `json:"blockTime"` // Block timestamp for time-range queries
	// use the model directly to avoid the need to map every single property which could grow later
	Summary *indexermodels.BlockSummary `json:"summaries"`
}

// ActivitySaveBlockSummaryOutput contains the result of saving block summaries along with execution duration.
type ActivitySaveBlockSummaryOutput struct {
	DurationMs float64 `json:"durationMs"` // Execution time in milliseconds
}

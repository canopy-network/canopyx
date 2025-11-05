package activity

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"go.temporal.io/sdk/temporal"
)

// SaveBlockSummary saves aggregated entity counts for a block after all entities have been indexed.
// This activity should run after all entity indexing activities (IndexTransactions, IndexEvents, etc.) complete.
// Returns output containing execution duration in milliseconds.
func (ac *Context) SaveBlockSummary(ctx context.Context, in types.ActivitySaveBlockSummaryInput) (types.ActivitySaveBlockSummaryOutput, error) {
	start := time.Now()

	chainDb, chainDbErr := ac.GetChainDb(ctx, ac.ChainID)
	if chainDbErr != nil {
		return types.ActivitySaveBlockSummaryOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	if in.Summary == nil {
		return types.ActivitySaveBlockSummaryOutput{}, temporal.NewNonRetryableApplicationError("missing block summary", "missing_data", nil)
	}

	// Insert block summary to staging table (two-phase commit pattern)
	if err := chainDb.InsertBlockSummariesStaging(ctx, in.Summary); err != nil {
		return types.ActivitySaveBlockSummaryOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.ActivitySaveBlockSummaryOutput{DurationMs: durationMs}, nil
}

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
func (c *Context) SaveBlockSummary(ctx context.Context, in types.SaveBlockSummaryInput) (types.SaveBlockSummaryOutput, error) {
	start := time.Now()

	// Acquire (or ping) the chain DB
	chainDb, chainDbErr := c.NewChainDb(ctx, in.ChainID)
	if chainDbErr != nil {
		return types.SaveBlockSummaryOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	// Insert block summary to staging table (two-phase commit pattern)
	if err := chainDb.InsertBlockSummariesStaging(ctx, in.Height, in.BlockTime, in.Summaries.NumTxs, in.Summaries.TxCountsByType); err != nil {
		return types.SaveBlockSummaryOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.SaveBlockSummaryOutput{DurationMs: durationMs}, nil
}

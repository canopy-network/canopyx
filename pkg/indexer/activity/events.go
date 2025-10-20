package activity

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// IndexEvents indexes events for a given block.
// Returns output containing the number of indexed events, counts by type, and execution duration in milliseconds.
func (c *Context) IndexEvents(ctx context.Context, in types.IndexEventsInput) (types.IndexEventsOutput, error) {
	start := time.Now()

	ch, err := c.IndexerDB.GetChain(ctx, in.ChainID)
	if err != nil {
		return types.IndexEventsOutput{}, err
	}

	// Acquire (or ping) the chain DB just to validate it exists.
	chainDb, chainDbErr := c.NewChainDb(ctx, in.ChainID)
	if chainDbErr != nil {
		return types.IndexEventsOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	// Fetch and parse events from RPC (single-table design)
	cli := c.rpcClient(ch.RPCEndpoints)
	events, err := cli.EventsByHeight(ctx, in.Height)
	if err != nil {
		return types.IndexEventsOutput{}, err
	}

	// Populate HeightTime field for all events using the block timestamp
	for i := range events {
		events[i].HeightTime = in.BlockTime
	}

	// Count events by type for analytics
	eventCountsByType := make(map[string]uint32)
	for _, event := range events {
		eventCountsByType[event.EventType]++
	}

	numEvents := uint32(len(events))
	c.Logger.Debug("IndexEvents fetched from RPC",
		zap.Uint64("height", in.Height),
		zap.Uint32("numEvents", numEvents),
		zap.Any("eventCountsByType", eventCountsByType))

	// Insert events to staging table (two-phase commit pattern)
	if err := chainDb.InsertEventsStaging(ctx, events); err != nil {
		return types.IndexEventsOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.IndexEventsOutput{
		NumEvents:         numEvents,
		EventCountsByType: eventCountsByType,
		DurationMs:        durationMs,
	}, nil
}

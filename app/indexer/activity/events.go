package activity

import (
	"context"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/db/transform"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// IndexEvents indexes events for a given block.
// Returns output containing the number of indexed events, counts by type, and execution duration in milliseconds.
func (ac *Context) IndexEvents(ctx context.Context, in types.ActivityIndexAtHeight) (types.ActivityIndexEventsOutput, error) {
	start := time.Now()

	cli, err := ac.rpcClient(ctx)
	if err != nil {
		return types.ActivityIndexEventsOutput{}, err
	}

	// Acquire (or ping) the chain DB just to validate it exists.
	chainDb, chainDbErr := ac.GetChainDb(ctx, ac.ChainID)
	if chainDbErr != nil {
		return types.ActivityIndexEventsOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	// Fetch and parse events from RPC (single-table design)
	rpcEvents, err := cli.EventsByHeight(ctx, in.Height)
	if err != nil {
		return types.ActivityIndexEventsOutput{}, err
	}

	// Convert RPC events to indexer models
	events := make([]*indexer.Event, 0, len(rpcEvents))
	for _, rpcEvent := range rpcEvents {
		event, err := transform.Event(rpcEvent)
		if err != nil {
			// Fail fast - conversion errors mean corrupted/incomplete data
			return types.ActivityIndexEventsOutput{}, fmt.Errorf("convert event at height %d, type %s: %w", in.Height, rpcEvent.EventType, err)
		}
		// Populate HeightTime field using the block timestamp
		event.HeightTime = in.BlockTime
		events = append(events, event)
	}

	// Count events by type for analytics
	eventCountsByType := make(map[string]uint32)
	for _, event := range events {
		eventCountsByType[event.EventType]++
	}

	numEvents := uint32(len(events))
	ac.Logger.Debug("IndexEvents fetched from RPC",
		zap.Uint64("height", in.Height),
		zap.Uint32("numEvents", numEvents),
		zap.Any("eventCountsByType", eventCountsByType))

	// Insert events to the staging table (two-phase commit pattern)
	if err := chainDb.InsertEventsStaging(ctx, events); err != nil {
		return types.ActivityIndexEventsOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.ActivityIndexEventsOutput{
		NumEvents:         numEvents,
		EventCountsByType: eventCountsByType,
		DurationMs:        durationMs,
	}, nil
}

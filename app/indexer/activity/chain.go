package activity

import (
	"context"
	"encoding/json"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/utils"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"go.uber.org/zap"
)

// RecordIndexed records the height of the last block indexed for a given chain along with timing metrics.
// After updating the index_progress watermark, it publishes a block.indexed event to Redis for real-time notifications.
func (ac *Context) RecordIndexed(ctx context.Context, in types.ActivityRecordIndexedInput) error {
	// Get chain database to fetch block time
	chainDb, err := ac.GetChainDb(ctx, ac.ChainID)
	if err != nil {
		return err
	}

	// Query block to get its timestamp
	block, err := chainDb.GetBlock(ctx, in.Height)
	if err != nil {
		return err
	}

	// Update index_progress watermark first - this makes data queryable
	if err := ac.AdminDB.RecordIndexed(ctx, ac.ChainID, in.Height, block.Time, in.IndexingTimeMs, in.IndexingDetail); err != nil {
		return err
	}

	// Publish block.indexed event to Redis (best-effort, don't fail if Redis unavailable)
	if ac.RedisClient != nil {
		// Build event
		// Query block summary to get entity counts
		summary, summaryErr := chainDb.GetBlockSummary(ctx, in.Height, false)
		if summaryErr != nil {
			// fallback to staging just in case the database is still processing the propagation
			summary, summaryErr = chainDb.GetBlockSummary(ctx, in.Height, true)
			if summaryErr != nil {
				ac.Logger.Warn(
					"Fail to load block summary to publish block.indexed event (non-fatal)",
					zap.Uint64("chainId", ac.ChainID),
					zap.Uint64("height", in.Height),
					zap.Error(summaryErr),
				)
				return nil
			}
		}

		event := &types.BlockIndexedEvent{
			Event:     "block.indexed",
			ChainID:   ac.ChainID,
			Height:    in.Height,
			Timestamp: time.Now().UTC(),
			Block: types.BlockInfo{
				Hash:            block.Hash,
				Time:            block.Time,
				ProposerAddress: block.ProposerAddress,
			},
			Summary: *summary, // Include a complete BlockSummary
		}

		if err := ac.publishBlockIndexedEvent(ctx, event); err != nil {
			ac.Logger.Warn("Failed to publish block.indexed event (non-fatal)",
				zap.Uint64("chainId", ac.ChainID),
				zap.Uint64("height", in.Height),
				zap.Error(err))
			// Don't return error - Redis publish is best-effort
		}
	}

	return nil
}

// publishBlockIndexedEvent queries block and summary data, then publishes to Redis.
func (ac *Context) publishBlockIndexedEvent(ctx context.Context, event *types.BlockIndexedEvent) error {
	// Marshal to JSON
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// Publish to a Redis channel (if Redis is available)
	// The Publish method handles errors internally and logs them
	if ac.RedisClient != nil {
		channel := utils.GetBlockIndexedChannel(ac.ChainID)
		ac.RedisClient.Publish(ctx, channel, payload)

		ac.Logger.Debug("Published block.indexed event",
			zap.Uint64("chainId", ac.ChainID),
			zap.Uint64("height", event.Height),
			zap.String("channel", channel))
	} else {
		ac.Logger.Debug("Redis client not available, skipping event publication",
			zap.Uint64("chainId", ac.ChainID),
			zap.Uint64("height", event.Height))
	}

	return nil
}

package activity

import (
	"context"
	"encoding/json"
	"time"

	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"go.uber.org/zap"
)

// RecordIndexed records the height of the last block indexed for a given chain along with timing metrics.
// After updating the index_progress watermark, it publishes a block.indexed event to Redis for real-time notifications.
func (c *Context) RecordIndexed(ctx context.Context, in types.RecordIndexedInput) error {
	// Update index_progress watermark first - this makes data queryable
	if err := c.IndexerDB.RecordIndexed(ctx, in.ChainID, in.Height, in.IndexingTimeMs, in.IndexingDetail); err != nil {
		return err
	}

	// Publish block.indexed event to Redis (best-effort, don't fail if Redis unavailable)
	if c.RedisClient != nil {
		if err := c.publishBlockIndexedEvent(ctx, in); err != nil {
			c.Logger.Warn("Failed to publish block.indexed event (non-fatal)",
				zap.String("chainId", in.ChainID),
				zap.Uint64("height", in.Height),
				zap.Error(err))
			// Don't return error - Redis publish is best-effort
		}
	}

	return nil
}

// publishBlockIndexedEvent queries block and summary data, then publishes to Redis.
func (c *Context) publishBlockIndexedEvent(ctx context.Context, in types.RecordIndexedInput) error {
	// Get chain database
	chainDb, err := c.NewChainDb(ctx, in.ChainID)
	if err != nil {
		return err
	}

	// Query block to get hash, time, proposer
	block, err := chainDb.GetBlock(ctx, in.Height)
	if err != nil {
		return err
	}

	// Query block summary to get entity counts
	summary, err := chainDb.GetBlockSummary(ctx, in.Height)
	if err != nil {
		return err
	}

	// Build event
	event := types.BlockIndexedEvent{
		Event:     "block.indexed",
		ChainID:   in.ChainID,
		Height:    in.Height,
		Timestamp: time.Now().UTC(),
		Block: types.BlockInfo{
			Hash:            block.Hash,
			Time:            block.Time,
			ProposerAddress: block.ProposerAddress,
		},
		Summary: *summary, // Include complete BlockSummary
	}

	// Marshal to JSON
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// Publish to Redis channel (if Redis is available)
	// The Publish method handles errors internally and logs them
	if c.RedisClient != nil {
		channel := types.GetBlockIndexedChannel(in.ChainID)
		c.RedisClient.Publish(ctx, channel, payload)

		c.Logger.Debug("Published block.indexed event",
			zap.String("chainId", in.ChainID),
			zap.Uint64("height", in.Height),
			zap.String("channel", channel))
	} else {
		c.Logger.Debug("Redis client not available, skipping event publication",
			zap.String("chainId", in.ChainID),
			zap.Uint64("height", in.Height))
	}

	return nil
}

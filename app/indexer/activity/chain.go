package activity

import (
	"context"
	"encoding/json"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"github.com/canopy-network/canopyx/app/indexer/utils"
	adminmodels "github.com/canopy-network/canopyx/pkg/db/models/admin"
	"go.uber.org/zap"
)

// RecordIndexed records the height of the last block indexed for a given chain along with timing metrics.
// After updating the index_progress watermark, it publishes a block.indexed event to Redis for real-time notifications.
func (ac *Context) RecordIndexed(ctx context.Context, in types.ActivityRecordIndexedInput) error {
	// Convert input timing fields to IndexProgress struct for columnar storage
	timing := &adminmodels.IndexProgress{
		TimingFetchBlockMs:        in.TimingFetchBlockMs,
		TimingPrepareIndexMs:      in.TimingPrepareIndexMs,
		TimingIndexAccountsMs:     in.TimingIndexAccountsMs,
		TimingIndexCommitteesMs:   in.TimingIndexCommitteesMs,
		TimingIndexDexBatchMs:     in.TimingIndexDexBatchMs,
		TimingIndexDexPricesMs:    in.TimingIndexDexPricesMs,
		TimingIndexEventsMs:       in.TimingIndexEventsMs,
		TimingIndexOrdersMs:       in.TimingIndexOrdersMs,
		TimingIndexParamsMs:       in.TimingIndexParamsMs,
		TimingIndexPoolsMs:        in.TimingIndexPoolsMs,
		TimingIndexSupplyMs:       in.TimingIndexSupplyMs,
		TimingIndexTransactionsMs: in.TimingIndexTransactionsMs,
		TimingIndexValidatorsMs:   in.TimingIndexValidatorsMs,
		TimingSaveBlockMs:         in.TimingSaveBlockMs,
		TimingSaveBlockSummaryMs:  in.TimingSaveBlockSummaryMs,
	}

	// Update index_progress watermark
	// IndexingTimeMs already contains the accurate workflow execution time
	if err := ac.AdminDB.RecordIndexed(ctx, ac.ChainID, in.Height, in.IndexingTimeMs, timing); err != nil {
		return err
	}

	// Publish block.indexed event to Redis (best-effort, don't fail if Redis unavailable)
	if ac.RedisClient != nil {
		// Get chain database for block summary (needed for Redis event)
		chainDb, err := ac.GetGlobalDb(ctx)
		if err != nil {
			ac.Logger.Warn("Failed to get global db for block summary (non-fatal)",
				zap.Uint64("chainId", ac.ChainID),
				zap.Uint64("height", in.Height),
				zap.Error(err))
			return nil
		}

		// Query block summary to get entity counts
		summary, summaryErr := chainDb.GetBlockSummary(ctx, in.Height)
		if summaryErr != nil {
			ac.Logger.Warn(
				"Fail to load block summary to publish block.indexed event (non-fatal)",
				zap.Uint64("chainId", ac.ChainID),
				zap.Uint64("height", in.Height),
				zap.Error(summaryErr),
			)
			return nil
		}

		// Use block info passed from IndexBlockFromBlob (no ClickHouse re-query needed)
		event := &types.BlockIndexedEvent{
			Event:     "block.indexed",
			ChainID:   ac.ChainID,
			Height:    in.Height,
			Timestamp: time.Now().UTC(),
			Block: types.BlockInfo{
				Hash:            in.BlockHash,
				Time:            in.BlockTime,
				ProposerAddress: in.BlockProposerAddress,
			},
			Summary: *summary,
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

// publishBlockIndexedEvent publishes to both Redis Pub/Sub (real-time) and Streams (durable).
func (ac *Context) publishBlockIndexedEvent(ctx context.Context, event *types.BlockIndexedEvent) error {
	// Marshal to JSON
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	if ac.RedisClient == nil {
		ac.Logger.Debug("Redis client not available, skipping event publication",
			zap.Uint64("chainId", ac.ChainID),
			zap.Uint64("height", event.Height))
		return nil
	}

	// 1. Publish to Pub/Sub channel for real-time subscribers
	channel := utils.GetBlockIndexedChannel(ac.ChainID)
	ac.RedisClient.Publish(ctx, channel, payload)

	// 2. Publish to Stream for durable delivery (consumer groups)
	stream := utils.GetBlockIndexedStream(ac.ChainID)
	_, streamErr := ac.RedisClient.XAddWithMaxLen(ctx, stream, utils.DefaultStreamMaxLen, map[string]interface{}{
		"event":    event.Event,
		"chain_id": event.ChainID,
		"height":   event.Height,
		"payload":  string(payload),
	})
	if streamErr != nil {
		ac.Logger.Warn("Failed to publish to stream (non-fatal)",
			zap.Uint64("chainId", ac.ChainID),
			zap.Uint64("height", event.Height),
			zap.String("stream", stream),
			zap.Error(streamErr))
	}

	ac.Logger.Debug("Published block.indexed event",
		zap.Uint64("chainId", ac.ChainID),
		zap.Uint64("height", event.Height),
		zap.String("channel", channel),
		zap.String("stream", stream))

	return nil
}

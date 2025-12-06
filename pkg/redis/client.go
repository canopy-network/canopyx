package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/pkg/utils"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Default stream configuration
const (
	DefaultStreamMaxLen = 10000 // Default max entries per stream
)

// Client wraps the Redis client for real-time event notifications (Pub/Sub and Streams).
type Client struct {
	client       *redis.Client
	logger       *zap.Logger
	streamMaxLen int64 // Max entries per stream (0 = unlimited)
}

// NewClient creates a new Redis client using environment variables for configuration.
// Environment variables:
//   - REDIS_HOST: Redis host (default: "localhost")
//   - REDIS_PORT: Redis port (default: "6379")
//   - REDIS_PASSWORD: Redis password (default: "")
//   - REDIS_DB: Redis database number (default: "0")
//   - REDIS_STREAM_MAXLEN: Max entries per stream (default: 10000, 0 = unlimited)
func NewClient(ctx context.Context, logger *zap.Logger) (*Client, error) {
	host := utils.Env("REDIS_HOST", "localhost")
	port := utils.Env("REDIS_PORT", "6379")
	password := utils.Env("REDIS_PASSWORD", "")
	db := utils.EnvInt("REDIS_DB", 0)
	streamMaxLen := utils.EnvInt64("REDIS_STREAM_MAXLEN", DefaultStreamMaxLen)

	addr := fmt.Sprintf("%s:%s", host, port)

	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,

		// Connection pool
		PoolSize:     10,
		MinIdleConns: 2,

		// Timeouts
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis at %s: %w", addr, err)
	}

	logger.Info("Connected to Redis",
		zap.String("addr", addr),
		zap.Int("db", db),
		zap.Int64("streamMaxLen", streamMaxLen))

	return &Client{
		client:       rdb,
		logger:       logger,
		streamMaxLen: streamMaxLen,
	}, nil
}

// Close closes the Redis connection.
func (c *Client) Close() error {
	return c.client.Close()
}

// GetClient returns the underlying Redis client.
// This allows activities and other components to use the full Redis API if needed.
func (c *Client) GetClient() *redis.Client {
	return c.client
}

// Publish publishes a message to a Redis Pub/Sub channel.
// This is a best-effort operation - errors are logged but not returned
// to prevent failures from affecting critical workflows.
func (c *Client) Publish(ctx context.Context, channel string, message interface{}) {
	if err := c.client.Publish(ctx, channel, message).Err(); err != nil {
		c.logger.Warn("Failed to publish Redis message",
			zap.String("channel", channel),
			zap.Error(err))
	}
}

// Subscribe subscribes to one or more Redis Pub/Sub channels.
// Returns a PubSub object that can be used to receive messages.
// The caller is responsible for closing the PubSub object when done.
func (c *Client) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return c.client.Subscribe(ctx, channels...)
}

// PSubscribe subscribes to one or more Redis Pub/Sub channel patterns.
// Patterns support wildcards like "*" and "?".
// For example: "canopy:*:block.indexed" will match "canopy:chainA:block.indexed", "canopy:chainB:block.indexed", etc.
// Returns a PubSub object that can be used to receive messages.
// The caller is responsible for closing the PubSub object when done.
func (c *Client) PSubscribe(ctx context.Context, patterns ...string) *redis.PubSub {
	c.logger.Debug("Subscribing to Redis patterns", zap.Strings("patterns", patterns))
	return c.client.PSubscribe(ctx, patterns...)
}

// Health checks if Redis is healthy.
func (c *Client) Health(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// =============================================================================
// Redis Streams API
// =============================================================================

// XAdd adds an entry to a stream. Uses MAXLEN to cap stream size if configured.
// Returns the entry ID (e.g., "1234567890123-0") or error.
// This is best-effort - errors are logged but not returned to prevent failures
// from affecting critical workflows.
func (c *Client) XAdd(ctx context.Context, stream string, values map[string]interface{}) string {
	args := &redis.XAddArgs{
		Stream: stream,
		Values: values,
	}

	// Apply MAXLEN if configured (approximate for performance)
	if c.streamMaxLen > 0 {
		args.MaxLen = c.streamMaxLen
		args.Approx = true
	}

	id, err := c.client.XAdd(ctx, args).Result()
	if err != nil {
		c.logger.Warn("Failed to add to Redis stream",
			zap.String("stream", stream),
			zap.Error(err))
		return ""
	}
	return id
}

// XRead reads entries from one or more streams starting after the given IDs.
// Use "0" to read from the beginning, "$" to read only new entries.
// Block specifies how long to wait for new entries (0 = no blocking).
// Returns entries grouped by stream, or error.
func (c *Client) XRead(ctx context.Context, streams []string, lastIDs []string, count int64, block time.Duration) ([]redis.XStream, error) {
	// XRead expects streams and IDs interleaved: XREAD STREAMS stream1 stream2 id1 id2
	streamsArg := append(streams, lastIDs...)

	return c.client.XRead(ctx, &redis.XReadArgs{
		Streams: streamsArg,
		Count:   count,
		Block:   block,
	}).Result()
}

// XReadGroup reads entries from streams using a consumer group.
// Supports at-least-once delivery with acknowledgments.
// Use ">" as lastID to read only new (undelivered) entries.
// Use "0" to re-read pending entries for this consumer.
func (c *Client) XReadGroup(ctx context.Context, group, consumer string, streams []string, lastIDs []string, count int64, block time.Duration) ([]redis.XStream, error) {
	streamsArg := append(streams, lastIDs...)

	return c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  streamsArg,
		Count:    count,
		Block:    block,
	}).Result()
}

// XAck acknowledges that entries have been processed by a consumer group.
// Returns the number of entries acknowledged.
func (c *Client) XAck(ctx context.Context, stream, group string, ids ...string) (int64, error) {
	return c.client.XAck(ctx, stream, group, ids...).Result()
}

// XGroupCreateMkStream creates a consumer group, creating the stream if it doesn't exist.
// Use "$" as start to only receive new messages, "0" to receive all messages.
// Returns nil if successful, or error (ignores "BUSYGROUP" error if group already exists).
func (c *Client) XGroupCreateMkStream(ctx context.Context, stream, group, start string) error {
	err := c.client.XGroupCreateMkStream(ctx, stream, group, start).Err()
	if err != nil && err.Error() == "BUSYGROUP Consumer Group name already exists" {
		// Group already exists, that's fine
		return nil
	}
	return err
}

// XLen returns the number of entries in a stream.
func (c *Client) XLen(ctx context.Context, stream string) (int64, error) {
	return c.client.XLen(ctx, stream).Result()
}

// XRange returns entries from a stream between two IDs (inclusive).
// Use "-" for start to begin at the earliest entry.
// Use "+" for end to read to the latest entry.
func (c *Client) XRange(ctx context.Context, stream, start, end string, count int64) ([]redis.XMessage, error) {
	return c.client.XRangeN(ctx, stream, start, end, count).Result()
}

package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/pkg/utils"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Client wraps the Redis client for real-time event notifications (Pub/Sub).
type Client struct {
	client *redis.Client
	logger *zap.Logger
}

// NewClient creates a new Redis client using environment variables for configuration.
// Environment variables:
//   - REDIS_HOST: Redis host (default: "localhost")
//   - REDIS_PORT: Redis port (default: "6379")
//   - REDIS_PASSWORD: Redis password (default: "")
//   - REDIS_DB: Redis database number (default: "0")
func NewClient(ctx context.Context, logger *zap.Logger) (*Client, error) {
	host := utils.Env("REDIS_HOST", "localhost")
	port := utils.Env("REDIS_PORT", "6379")
	password := utils.Env("REDIS_PASSWORD", "")
	db := utils.EnvInt("REDIS_DB", 0)

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

	logger.Info("Connected to Redis", zap.String("addr", addr), zap.Int("db", db))

	return &Client{
		client: rdb,
		logger: logger,
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

package redis

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// StreamConsumerConfig configures a StreamConsumer.
type StreamConsumerConfig struct {
	// Stream is the Redis stream name to consume from (required).
	Stream string

	// Group is the consumer group name. If empty, uses XREAD (simple consumer).
	// If set, uses XREADGROUP (consumer group with acknowledgments).
	Group string

	// Consumer is the consumer name within the group. Required if Group is set.
	Consumer string

	// LastID is the starting position:
	//   - "0" = read from beginning (or pending messages if using group)
	//   - "$" = read only new messages
	//   - "<id>" = read after specific ID (e.g., "1234567890123-0")
	//   - ">" = only new messages (only valid with consumer groups)
	// Default: "0"
	LastID string

	// Count is the max number of entries to read per batch. Default: 100.
	Count int64

	// Block is how long to wait for new entries. Default: 5 seconds.
	// Set to 0 for non-blocking reads.
	Block time.Duration

	// AutoAck automatically acknowledges messages after successful processing.
	// Only applies when using consumer groups. Default: true.
	AutoAck bool

	// RetryInterval is how long to wait before retrying after an error.
	// Default: 1 second.
	RetryInterval time.Duration

	// MaxRetryInterval is the maximum retry interval (with exponential backoff).
	// Default: 30 seconds.
	MaxRetryInterval time.Duration

	// Logger for logging. If nil, uses a no-op logger.
	Logger *zap.Logger
}

// MessageHandler processes a stream message. Return nil to acknowledge (if AutoAck),
// or return an error to skip acknowledgment and retry later.
type MessageHandler func(ctx context.Context, msg Message) error

// Message represents a single stream entry with parsed fields.
type Message struct {
	// ID is the Redis stream entry ID (e.g., "1234567890123-0").
	ID string

	// Stream is the stream name this message came from.
	Stream string

	// Values contains the entry fields as key-value pairs.
	Values map[string]interface{}
}

// StreamConsumer consumes messages from a Redis stream with automatic
// reconnection and optional consumer group support.
type StreamConsumer struct {
	client *Client
	config StreamConsumerConfig
	logger *zap.Logger
}

// NewStreamConsumer creates a new stream consumer.
func NewStreamConsumer(client *Client, config StreamConsumerConfig) (*StreamConsumer, error) {
	if client == nil {
		return nil, errors.New("redis client is required")
	}
	if config.Stream == "" {
		return nil, errors.New("stream name is required")
	}
	if config.Group != "" && config.Consumer == "" {
		return nil, errors.New("consumer name is required when using consumer groups")
	}

	// Apply defaults
	if config.LastID == "" {
		config.LastID = "0"
	}
	if config.Count == 0 {
		config.Count = 100
	}
	if config.Block == 0 {
		config.Block = 5 * time.Second
	}
	if config.RetryInterval == 0 {
		config.RetryInterval = 1 * time.Second
	}
	if config.MaxRetryInterval == 0 {
		config.MaxRetryInterval = 30 * time.Second
	}
	if config.Group != "" && !config.AutoAck {
		// Default to auto-ack for consumer groups unless explicitly disabled
		config.AutoAck = true
	}

	logger := config.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	return &StreamConsumer{
		client: client,
		config: config,
		logger: logger,
	}, nil
}

// Run starts consuming messages and calls handler for each message.
// Blocks until context is cancelled. Automatically handles reconnection.
func (sc *StreamConsumer) Run(ctx context.Context, handler MessageHandler) error {
	// Create consumer group if configured
	if sc.config.Group != "" {
		if err := sc.ensureConsumerGroup(ctx); err != nil {
			return err
		}
	}

	lastID := sc.config.LastID
	retryInterval := sc.config.RetryInterval

	for {
		select {
		case <-ctx.Done():
			sc.logger.Info("Stream consumer shutting down",
				zap.String("stream", sc.config.Stream),
				zap.String("group", sc.config.Group))
			return ctx.Err()
		default:
		}

		// Read messages
		messages, newLastID, err := sc.readMessages(ctx, lastID)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			if err == redis.Nil {
				// No messages available (timeout), continue
				continue
			}

			sc.logger.Warn("Error reading from stream, will retry",
				zap.String("stream", sc.config.Stream),
				zap.Error(err),
				zap.Duration("retryIn", retryInterval))

			select {
			case <-time.After(retryInterval):
				// Exponential backoff
				retryInterval = min(retryInterval*2, sc.config.MaxRetryInterval)
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}

		// Reset retry interval on success
		retryInterval = sc.config.RetryInterval

		// Update lastID for next read (only for simple consumers)
		if sc.config.Group == "" && newLastID != "" {
			lastID = newLastID
		}

		// Process messages
		for _, msg := range messages {
			if err := sc.processMessage(ctx, handler, msg); err != nil {
				sc.logger.Error("Error processing message",
					zap.String("stream", sc.config.Stream),
					zap.String("id", msg.ID),
					zap.Error(err))
				// Continue processing other messages
			}
		}
	}
}

// ensureConsumerGroup creates the consumer group if it doesn't exist.
func (sc *StreamConsumer) ensureConsumerGroup(ctx context.Context) error {
	err := sc.client.XGroupCreateMkStream(ctx, sc.config.Stream, sc.config.Group, sc.config.LastID)
	if err != nil {
		sc.logger.Error("Failed to create consumer group",
			zap.String("stream", sc.config.Stream),
			zap.String("group", sc.config.Group),
			zap.Error(err))
		return err
	}

	sc.logger.Info("Consumer group ready",
		zap.String("stream", sc.config.Stream),
		zap.String("group", sc.config.Group),
		zap.String("consumer", sc.config.Consumer))
	return nil
}

// readMessages reads a batch of messages from the stream.
func (sc *StreamConsumer) readMessages(ctx context.Context, lastID string) ([]Message, string, error) {
	var streams []redis.XStream
	var err error

	if sc.config.Group != "" {
		// Consumer group mode - use XREADGROUP
		streams, err = sc.client.XReadGroup(ctx,
			sc.config.Group,
			sc.config.Consumer,
			[]string{sc.config.Stream},
			[]string{">"}, // Always read new messages for group
			sc.config.Count,
			sc.config.Block,
		)
	} else {
		// Simple consumer mode - use XREAD
		streams, err = sc.client.XRead(ctx,
			[]string{sc.config.Stream},
			[]string{lastID},
			sc.config.Count,
			sc.config.Block,
		)
	}

	if err != nil {
		return nil, "", err
	}

	// Convert to our Message type
	var messages []Message
	var newLastID string

	for _, stream := range streams {
		for _, xmsg := range stream.Messages {
			messages = append(messages, Message{
				ID:     xmsg.ID,
				Stream: stream.Stream,
				Values: xmsg.Values,
			})
			newLastID = xmsg.ID
		}
	}

	return messages, newLastID, nil
}

// processMessage processes a single message and optionally acknowledges it.
func (sc *StreamConsumer) processMessage(ctx context.Context, handler MessageHandler, msg Message) error {
	err := handler(ctx, msg)
	if err != nil {
		return err
	}

	// Auto-acknowledge if using consumer groups and AutoAck is enabled
	if sc.config.Group != "" && sc.config.AutoAck {
		if _, ackErr := sc.client.XAck(ctx, sc.config.Stream, sc.config.Group, msg.ID); ackErr != nil {
			sc.logger.Warn("Failed to acknowledge message",
				zap.String("stream", sc.config.Stream),
				zap.String("id", msg.ID),
				zap.Error(ackErr))
		}
	}

	return nil
}

// GetData is a helper to extract the "data" field from a message as a string.
// Returns empty string if not found or not a string.
func (m *Message) GetData() string {
	if data, ok := m.Values["data"].(string); ok {
		return data
	}
	return ""
}

// GetChainID is a helper to extract the "chainId" field from a message.
// Returns 0 if not found or not parseable.
func (m *Message) GetChainID() uint64 {
	switch v := m.Values["chainId"].(type) {
	case uint64:
		return v
	case int64:
		return uint64(v)
	case float64:
		return uint64(v)
	case string:
		// Redis returns numbers as strings
		var id uint64
		for _, c := range v {
			if c >= '0' && c <= '9' {
				id = id*10 + uint64(c-'0')
			}
		}
		return id
	}
	return 0
}

// GetHeight is a helper to extract the "height" field from a message.
// Returns 0 if not found or not parseable.
func (m *Message) GetHeight() uint64 {
	switch v := m.Values["height"].(type) {
	case uint64:
		return v
	case int64:
		return uint64(v)
	case float64:
		return uint64(v)
	case string:
		var h uint64
		for _, c := range v {
			if c >= '0' && c <= '9' {
				h = h*10 + uint64(c-'0')
			}
		}
		return h
	}
	return 0
}

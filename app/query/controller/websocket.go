package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: In production, restrict to specific origins
		return true
	},
}

// ClientMessage represents messages sent by WebSocket clients.
type ClientMessage struct {
	Action  string `json:"action"`  // "subscribe" or "unsubscribe"
	ChainID string `json:"chainId"` // Chain ID to subscribe to, or "*" for all chains
}

// ServerMessage represents messages sent to WebSocket clients.
type ServerMessage struct {
	Type    string      `json:"type"`    // "block.indexed", "subscribed", "unsubscribed", "error", "ping"
	Payload interface{} `json:"payload"` // Event-specific data
}

// clientSubscriptions tracks what chains a client is subscribed to.
type clientSubscriptions struct {
	mu     sync.RWMutex
	chains map[string]bool // chainId -> subscribed
}

// NewClientSubscriptions creates a new clientSubscriptions tracker.
// Exported for testing.
func NewClientSubscriptions() *clientSubscriptions {
	return &clientSubscriptions{
		chains: make(map[string]bool),
	}
}

// Subscribe adds a chain ID to the subscription list.
// Exported for testing.
func (cs *clientSubscriptions) Subscribe(chainID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.chains[chainID] = true
}

// Unsubscribe removes a chain ID from the subscription list.
// Exported for testing.
func (cs *clientSubscriptions) Unsubscribe(chainID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.chains, chainID)
}

// IsSubscribed checks if a chain ID is subscribed. Wildcard (*) matches all chains.
// Exported for testing.
func (cs *clientSubscriptions) IsSubscribed(chainID string) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	// Check for wildcard subscription
	if cs.chains["*"] {
		return true
	}
	// Check for specific chain subscription
	return cs.chains[chainID]
}

// HandleWebSocket upgrades HTTP connection to WebSocket and streams real-time events.
//
// Protocol:
// Client sends: {"action": "subscribe", "chainId": "canopy_local"}  // Subscribe to specific chain
// Client sends: {"action": "subscribe", "chainId": "*"}             // Subscribe to ALL chains
// Client sends: {"action": "unsubscribe", "chainId": "canopy_local"}
//
// Server sends:
// - {"type": "block.indexed", "payload": {...}}
// - {"type": "subscribed", "payload": {"chainId": "canopy_local"}}
// - {"type": "unsubscribed", "payload": {"chainId": "canopy_local"}}
// - {"type": "ping", "payload": {"timestamp": 1234567890}}
// - {"type": "error", "payload": {"message": "..."}}
//
// IMPORTANT: All goroutines have panic recovery to prevent crashes.
func (c *Controller) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Check if Redis is available
	if c.App.RedisClient == nil {
		http.Error(w, "Real-time events not available (Redis disabled)", http.StatusServiceUnavailable)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		c.App.Logger.Error("Failed to upgrade WebSocket connection", zap.Error(err))
		return
	}
	defer func(conn *websocket.Conn) {
		err := conn.Close()
		if err != nil {
			c.App.Logger.Error("Failed to close WebSocket connection", zap.Error(err))
		}
	}(conn)

	c.App.Logger.Info("WebSocket client connected", zap.String("remote_addr", r.RemoteAddr))

	// Create cancellable context for this connection
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Track client subscriptions
	subs := NewClientSubscriptions()

	// Channel for outgoing messages
	send := make(chan ServerMessage, 256)

	// Wait group to coordinate goroutines
	var wg sync.WaitGroup

	// Start Redis subscriber with panic recovery
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if rec := recover(); rec != nil {
				c.App.Logger.Error("Panic in Redis subscriber goroutine",
					zap.Any("panic", rec),
					zap.String("stack", string(debug.Stack())),
					zap.String("remote_addr", r.RemoteAddr))
				// Signal shutdown on panic
				cancel()
			}
		}()
		c.subscribeToRedis(ctx, send, subs)
	}()

	// Start ping ticker (keep-alive) with panic recovery
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if rec := recover(); rec != nil {
				c.App.Logger.Error("Panic in ping ticker goroutine",
					zap.Any("panic", rec),
					zap.String("stack", string(debug.Stack())),
					zap.String("remote_addr", r.RemoteAddr))
				// Signal shutdown on panic
				cancel()
			}
		}()
		c.sendPings(ctx, conn)
	}()

	// Start message writer with panic recovery
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if rec := recover(); rec != nil {
				c.App.Logger.Error("Panic in message writer goroutine",
					zap.Any("panic", rec),
					zap.String("stack", string(debug.Stack())),
					zap.String("remote_addr", r.RemoteAddr))
				// Signal shutdown on panic
				cancel()
			}
		}()
		c.writeMessages(conn, send)
	}()

	// Read messages from client (for subscriptions and close detection)
	// This blocks until the connection closes
	c.readClientMessages(ctx, conn, cancel, subs, send)

	// Connection closed - cleanup
	close(send)
	wg.Wait()

	c.App.Logger.Info("WebSocket client disconnected", zap.String("remote_addr", r.RemoteAddr))
}

// subscribeToRedis subscribes to Redis Pub/Sub pattern and forwards matching events to the send channel.
// Uses PSUBSCRIBE with pattern "canopy:*:block.indexed" to receive events from all chains.
// Filters events server-side based on client subscriptions.
//
// This function implements automatic reconnection with exponential backoff:
// - If Redis connection is lost, it will retry with increasing delays
// - Clients are notified when Redis is unavailable
// - Automatically restores subscription when Redis recovers
// - Respects context cancellation for clean shutdown
func (c *Controller) subscribeToRedis(ctx context.Context, send chan<- ServerMessage, subs *clientSubscriptions) {
	pattern := "canopy:*:block.indexed"

	// Retry configuration
	const (
		initialBackoff = 1 * time.Second
		maxBackoff     = 30 * time.Second
		backoffFactor  = 2.0
		jitterFactor   = 0.1 // 10% jitter
	)

	backoff := initialBackoff
	attemptNum := 0

	for {
		// Check if context is cancelled before attempting connection
		select {
		case <-ctx.Done():
			c.App.Logger.Info("Redis subscription cancelled")
			return
		default:
		}

		attemptNum++

		// Try to establish subscription
		subscriptionErr := c.attemptRedisSubscription(ctx, pattern, send, subs, attemptNum)

		// If context was cancelled, exit cleanly
		if ctx.Err() != nil {
			c.App.Logger.Info("Redis subscription cancelled")
			return
		}

		// Subscription ended (either due to error or channel closure)
		// Log the reason and prepare to retry
		if subscriptionErr != nil {
			c.App.Logger.Warn("Redis subscription failed, will retry",
				zap.Error(subscriptionErr),
				zap.Int("attempt", attemptNum),
				zap.Duration("backoff", backoff))
		} else {
			c.App.Logger.Warn("Redis subscription channel closed, will retry",
				zap.Int("attempt", attemptNum),
				zap.Duration("backoff", backoff))
		}

		// Notify client that Redis is unavailable
		select {
		case send <- ServerMessage{
			Type: "error",
			Payload: map[string]interface{}{
				"message":     "Redis connection lost, attempting to reconnect...",
				"retryIn":     backoff.Seconds(),
				"attempt":     attemptNum,
				"recoverable": true,
			},
		}:
		case <-ctx.Done():
			return
		}

		// Wait before retrying (with context cancellation check)
		select {
		case <-time.After(backoff):
			// Continue to retry
		case <-ctx.Done():
			c.App.Logger.Info("Redis subscription cancelled during backoff")
			return
		}

		// Calculate next backoff with exponential increase and jitter
		backoff = CalculateNextBackoff(backoff, maxBackoff, backoffFactor, jitterFactor)
	}
}

// attemptRedisSubscription attempts a single Redis subscription and processes messages until
// the subscription fails or context is cancelled. Returns error if subscription setup fails,
// or nil if the subscription was established but the channel closed.
func (c *Controller) attemptRedisSubscription(
	ctx context.Context,
	pattern string,
	send chan<- ServerMessage,
	subs *clientSubscriptions,
	attemptNum int,
) error {
	c.App.Logger.Info("Attempting Redis subscription",
		zap.String("pattern", pattern),
		zap.Int("attempt", attemptNum))

	// Create PubSub with pattern subscription
	pubsub := c.App.RedisClient.PSubscribe(ctx, pattern)
	defer func() {
		if err := pubsub.Close(); err != nil {
			c.App.Logger.Error("Error closing Redis subscription", zap.Error(err))
		}
	}()

	// Wait for confirmation of subscription with timeout
	// Use a separate context with timeout to avoid blocking forever
	receiveCtx, receiveCancel := context.WithTimeout(ctx, 5*time.Second)
	defer receiveCancel()

	_, err := pubsub.Receive(receiveCtx)
	if err != nil {
		return fmt.Errorf("failed to confirm Redis subscription: %w", err)
	}

	c.App.Logger.Info("Successfully subscribed to Redis pattern",
		zap.String("pattern", pattern),
		zap.Int("attempt", attemptNum))

	// Notify client that Redis connection is restored
	select {
	case send <- ServerMessage{
		Type: "info",
		Payload: map[string]interface{}{
			"message": "Redis connection established",
			"attempt": attemptNum,
		},
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	// Listen for messages until subscription fails or context is cancelled
	return c.processRedisMessages(ctx, pubsub, send, subs)
}

// processRedisMessages processes messages from the Redis PubSub channel until
// the channel closes or context is cancelled. Returns nil when channel closes,
// or context error when cancelled.
func (c *Controller) processRedisMessages(
	ctx context.Context,
	pubsub *redis.PubSub,
	send chan<- ServerMessage,
	subs *clientSubscriptions,
) error {
	ch := pubsub.Channel()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case msg, ok := <-ch:
			if !ok {
				// Channel closed - this is the normal Redis disconnection case
				return nil
			}

			// Extract chainID from channel name: "canopy:chainId:block.indexed"
			chainID := ExtractChainIDFromChannel(msg.Channel)
			if chainID == "" {
				c.App.Logger.Warn("Failed to extract chainID from channel",
					zap.String("channel", msg.Channel))
				continue
			}

			// Server-side filtering: only forward if client is subscribed
			if !subs.IsSubscribed(chainID) {
				continue
			}

			// Parse JSON payload
			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
				c.App.Logger.Error("Failed to parse Redis message",
					zap.Error(err),
					zap.String("channel", msg.Channel))
				continue
			}

			// Send to WebSocket client
			select {
			case send <- ServerMessage{Type: "block.indexed", Payload: payload}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// CalculateNextBackoff calculates the next backoff duration with exponential growth and jitter.
// Exported for testing.
func CalculateNextBackoff(current, max time.Duration, factor, jitterFactor float64) time.Duration {
	// Calculate exponential increase
	next := time.Duration(float64(current) * factor)

	// Cap at maximum
	if next > max {
		next = max
	}

	// Add jitter: random value between -jitterFactor and +jitterFactor
	// This prevents all clients from retrying at exactly the same time
	jitter := float64(next) * jitterFactor * (2*rand.Float64() - 1)
	nextWithJitter := time.Duration(float64(next) + jitter)

	// Ensure we never go below initial or above max
	if nextWithJitter < current {
		nextWithJitter = current
	}
	if nextWithJitter > max {
		nextWithJitter = max
	}

	return nextWithJitter
}

// ExtractChainIDFromChannel extracts the chain ID from a Redis channel name.
// Exported for testing.
func ExtractChainIDFromChannel(channel string) string {
	parts := strings.Split(channel, ":")
	if len(parts) != 3 {
		return ""
	}
	return parts[1]
}

// sendPings sends periodic WebSocket ping frames to keep the connection alive.
// The client will automatically respond with pong frames, which resets the read deadline.
func (c *Controller) sendPings(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Send WebSocket PING frame (not a JSON message)
			// Client will automatically respond with PONG, resetting read deadline
			if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
				c.App.Logger.Error("Failed to send ping", zap.Error(err))
				return
			}
		}
	}
}

// writeMessages writes messages from the send channel to the WebSocket connection.
func (c *Controller) writeMessages(conn *websocket.Conn, send <-chan ServerMessage) {
	for msg := range send {
		if err := conn.WriteJSON(msg); err != nil {
			c.App.Logger.Error("Failed to write WebSocket message", zap.Error(err))
			return
		}
	}
}

// readClientMessages reads messages from the WebSocket connection.
// Handles subscription/unsubscription requests and detects connection closure.
func (c *Controller) readClientMessages(ctx context.Context, conn *websocket.Conn, cancel context.CancelFunc, subs *clientSubscriptions, send chan<- ServerMessage) {
	// Set a read deadline for detecting dead connections
	err := conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	if err != nil {
		c.App.Logger.Error("Failed to set read deadline", zap.Error(err))
		return
	}

	// Set pong handler to reset read deadline
	conn.SetPongHandler(func(string) error {
		err := conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		if err != nil {
			c.App.Logger.Error("Failed to reset read deadline", zap.Error(err))
			return err
		}
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Read message
			var msg ClientMessage
			err := conn.ReadJSON(&msg)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					c.App.Logger.Error("WebSocket read error", zap.Error(err))
				}
				cancel() // Signal shutdown
				return
			}

			// Reset read deadline after successful read
			err = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			if err != nil {
				c.App.Logger.Error("Failed to reset read deadline", zap.Error(err))
				return
			}

			// Handle subscription actions
			switch msg.Action {
			case "subscribe":
				if msg.ChainID == "" {
					send <- ServerMessage{Type: "error", Payload: map[string]string{"message": "chainId is required"}}
					continue
				}
				subs.Subscribe(msg.ChainID)
				c.App.Logger.Debug("Client subscribed", zap.String("chainId", msg.ChainID))
				send <- ServerMessage{Type: "subscribed", Payload: map[string]string{"chainId": msg.ChainID}}

			case "unsubscribe":
				if msg.ChainID == "" {
					send <- ServerMessage{Type: "error", Payload: map[string]string{"message": "chainId is required"}}
					continue
				}
				subs.Unsubscribe(msg.ChainID)
				c.App.Logger.Debug("Client unsubscribed", zap.String("chainId", msg.ChainID))
				send <- ServerMessage{Type: "unsubscribed", Payload: map[string]string{"chainId": msg.ChainID}}

			default:
				send <- ServerMessage{Type: "error", Payload: map[string]string{"message": "unknown action: " + msg.Action}}
			}
		}
	}
}

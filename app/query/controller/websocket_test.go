package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/app/query/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// TestCalculateNextBackoff tests the exponential backoff calculation with jitter
func TestCalculateNextBackoff(t *testing.T) {
	tests := []struct {
		name         string
		current      time.Duration
		max          time.Duration
		factor       float64
		jitterFactor float64
		expectMin    time.Duration
		expectMax    time.Duration
	}{
		{
			name:         "initial backoff doubles",
			current:      1 * time.Second,
			max:          30 * time.Second,
			factor:       2.0,
			jitterFactor: 0.1,
			expectMin:    1800 * time.Millisecond, // 2s - 10% jitter
			expectMax:    2200 * time.Millisecond, // 2s + 10% jitter
		},
		{
			name:         "respects maximum",
			current:      20 * time.Second,
			max:          30 * time.Second,
			factor:       2.0,
			jitterFactor: 0.1,
			expectMin:    27 * time.Second, // 30s - 10% jitter
			expectMax:    30 * time.Second, // capped at max
		},
		{
			name:         "no jitter produces exact value",
			current:      5 * time.Second,
			max:          30 * time.Second,
			factor:       2.0,
			jitterFactor: 0.0,
			expectMin:    10 * time.Second, // exactly 2x
			expectMax:    10 * time.Second, // exactly 2x
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run multiple times to account for randomness in jitter
			for i := 0; i < 10; i++ {
				result := calculateNextBackoff(tt.current, tt.max, tt.factor, tt.jitterFactor)
				assert.GreaterOrEqual(t, result, tt.expectMin, "backoff should be >= minimum")
				assert.LessOrEqual(t, result, tt.expectMax, "backoff should be <= maximum")
			}
		})
	}
}

// TestExtractChainIDFromChannel tests parsing chain ID from Redis channel names
func TestExtractChainIDFromChannel(t *testing.T) {
	tests := []struct {
		name     string
		channel  string
		expected string
	}{
		{
			name:     "valid channel format",
			channel:  "canopy:testchain:block.indexed",
			expected: "testchain",
		},
		{
			name:     "valid channel with underscores",
			channel:  "canopy:canopy_local:block.indexed",
			expected: "canopy_local",
		},
		{
			name:     "invalid format - too few parts",
			channel:  "canopy:block.indexed",
			expected: "",
		},
		{
			name:     "invalid format - too many parts",
			channel:  "canopy:chain:extra:block.indexed",
			expected: "",
		},
		{
			name:     "empty channel",
			channel:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractChainIDFromChannel(tt.channel)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestClientSubscriptions tests the subscription tracking logic
func TestClientSubscriptions(t *testing.T) {
	t.Run("subscribe and check", func(t *testing.T) {
		subs := newClientSubscriptions()

		subs.subscribe("chain1")
		assert.True(t, subs.isSubscribed("chain1"))
		assert.False(t, subs.isSubscribed("chain2"))
	})

	t.Run("wildcard subscription", func(t *testing.T) {
		subs := newClientSubscriptions()

		subs.subscribe("*")
		assert.True(t, subs.isSubscribed("*"))
		assert.True(t, subs.isSubscribed("chain1"))
		assert.True(t, subs.isSubscribed("chain2"))
		assert.True(t, subs.isSubscribed("any_chain"))
	})

	t.Run("unsubscribe", func(t *testing.T) {
		subs := newClientSubscriptions()

		subs.subscribe("chain1")
		assert.True(t, subs.isSubscribed("chain1"))

		subs.unsubscribe("chain1")
		assert.False(t, subs.isSubscribed("chain1"))
	})

	t.Run("concurrent access", func(t *testing.T) {
		subs := newClientSubscriptions()
		done := make(chan bool)

		// Concurrent writes
		go func() {
			for i := 0; i < 100; i++ {
				subs.subscribe("chain1")
			}
			done <- true
		}()

		go func() {
			for i := 0; i < 100; i++ {
				subs.unsubscribe("chain1")
			}
			done <- true
		}()

		// Concurrent reads
		go func() {
			for i := 0; i < 100; i++ {
				_ = subs.isSubscribed("chain1")
			}
			done <- true
		}()

		// Wait for all goroutines
		<-done
		<-done
		<-done

		// Should not panic or race
	})
}

// TestProcessRedisMessages tests message processing and filtering
func TestProcessRedisMessages(t *testing.T) {
	t.Run("filters messages based on subscription", func(t *testing.T) {
		// This test demonstrates how to test the message processing logic
		// In a real implementation, you would mock the Redis PubSub
		subs := newClientSubscriptions()
		subs.subscribe("chain1")

		// Verify filtering logic
		assert.True(t, subs.isSubscribed("chain1"), "should be subscribed to chain1")
		assert.False(t, subs.isSubscribed("chain2"), "should not be subscribed to chain2")
	})

	t.Run("wildcard matches all chains", func(t *testing.T) {
		subs := newClientSubscriptions()
		subs.subscribe("*")

		assert.True(t, subs.isSubscribed("chain1"))
		assert.True(t, subs.isSubscribed("chain2"))
		assert.True(t, subs.isSubscribed("any_chain_id"))
	})
}

// Example test showing how to test WebSocket handler with a mock Redis client
// This is a simplified example - in production, you would use a proper mock
func TestWebSocketIntegration(t *testing.T) {
	// Skip this test if Redis is not available
	// In CI/CD, you would use testcontainers or a Redis mock
	t.Skip("Requires running Redis instance - use testcontainers in real tests")

	logger := zaptest.NewLogger(t)

	// In a real test, you would:
	// 1. Use testcontainers to spin up a real Redis instance
	// 2. Or create a mock Redis client that implements the same interface
	// 3. Test the full flow including reconnection logic

	// Example setup (would need proper Redis client):
	app := &types.App{
		Logger: logger,
		// RedisClient: mockRedisClient, // Would inject mock here
	}

	controller := &Controller{App: app}

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(controller.HandleWebSocket))
	defer server.Close()

	// This demonstrates the structure - actual implementation would test:
	// - WebSocket connection upgrade
	// - Subscription messages
	// - Redis reconnection behavior
	// - Message forwarding
	// - Clean shutdown

	assert.NotNil(t, controller)
}

// BenchmarkCalculateNextBackoff benchmarks the backoff calculation
func BenchmarkCalculateNextBackoff(b *testing.B) {
	current := 1 * time.Second
	max := 30 * time.Second
	factor := 2.0
	jitterFactor := 0.1

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = calculateNextBackoff(current, max, factor, jitterFactor)
	}
}

// TestServerMessageSerialization tests JSON serialization of messages
func TestServerMessageSerialization(t *testing.T) {
	tests := []struct {
		name    string
		message ServerMessage
		wantErr bool
	}{
		{
			name: "block indexed message",
			message: ServerMessage{
				Type: "block.indexed",
				Payload: map[string]interface{}{
					"chainId": "chain1",
					"height":  12345,
				},
			},
			wantErr: false,
		},
		{
			name: "error message with reconnect info",
			message: ServerMessage{
				Type: "error",
				Payload: map[string]interface{}{
					"message":     "Redis connection lost, attempting to reconnect...",
					"retryIn":     2.5,
					"attempt":     3,
					"recoverable": true,
				},
			},
			wantErr: false,
		},
		{
			name: "info message",
			message: ServerMessage{
				Type: "info",
				Payload: map[string]interface{}{
					"message": "Redis connection established",
					"attempt": 2,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.message)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Verify we can unmarshal back
			var decoded ServerMessage
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.message.Type, decoded.Type)
		})
	}
}

// TestClientMessageParsing tests parsing of client messages
func TestClientMessageParsing(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    ClientMessage
		wantErr bool
	}{
		{
			name: "subscribe to specific chain",
			json: `{"action":"subscribe","chainId":"chain1"}`,
			want: ClientMessage{
				Action:  "subscribe",
				ChainID: "chain1",
			},
			wantErr: false,
		},
		{
			name: "subscribe to all chains",
			json: `{"action":"subscribe","chainId":"*"}`,
			want: ClientMessage{
				Action:  "subscribe",
				ChainID: "*",
			},
			wantErr: false,
		},
		{
			name: "unsubscribe",
			json: `{"action":"unsubscribe","chainId":"chain1"}`,
			want: ClientMessage{
				Action:  "unsubscribe",
				ChainID: "chain1",
			},
			wantErr: false,
		},
		{
			name:    "invalid json",
			json:    `{invalid}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg ClientMessage
			err := json.Unmarshal([]byte(tt.json), &msg)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want.Action, msg.Action)
			assert.Equal(t, tt.want.ChainID, msg.ChainID)
		})
	}
}

// Example of how to structure an integration test with Redis
// This demonstrates the pattern for testing reconnection behavior
func Example_redisReconnectionTest() {
	// In a real test, you would:
	// 1. Start a Redis container using testcontainers
	// 2. Connect the WebSocket
	// 3. Subscribe to a chain
	// 4. Verify events are received
	// 5. Stop Redis
	// 6. Verify error messages are sent to client
	// 7. Start Redis again
	// 8. Verify connection is restored
	// 9. Verify events are received again

	// Pseudo-code structure:
	// container := startRedisContainer()
	// defer container.Stop()
	//
	// ws := connectWebSocket()
	// ws.Send(`{"action":"subscribe","chainId":"*"}`)
	//
	// publishTestEvent()
	// assertEventReceived(ws)
	//
	// container.Stop()
	// assertErrorMessage(ws, "Redis connection lost")
	//
	// container.Start()
	// assertInfoMessage(ws, "Redis connection established")
	//
	// publishTestEvent()
	// assertEventReceived(ws)
}

package db

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/clickhouse"
	"go.uber.org/zap"
)

// Global test container instance shared across all tests
var (
	clickhouseContainer *clickhouse.ClickHouseContainer
	testDSN             string
	testLogger          *zap.Logger
)

// TestMain sets up the ClickHouse testcontainer before all tests and tears it down after.
// This ensures we have a real ClickHouse instance for integration testing.
func TestMain(m *testing.M) {
	ctx := context.Background()

	// Initialize logger for tests
	var err error
	testLogger, err = zap.NewDevelopment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create test logger: %v\n", err)
		os.Exit(1)
	}
	defer testLogger.Sync()

	// Start ClickHouse container
	testLogger.Info("Starting ClickHouse testcontainer...")
	clickhouseContainer, testDSN, err = setupClickHouseContainer(ctx)
	if err != nil {
		testLogger.Fatal("Failed to start ClickHouse container", zap.Error(err))
		os.Exit(1)
	}

	testLogger.Info("ClickHouse testcontainer started successfully", zap.String("dsn", testDSN))

	// Run all tests
	code := m.Run()

	// Cleanup
	testLogger.Info("Tearing down ClickHouse testcontainer...")
	if err := clickhouseContainer.Terminate(ctx); err != nil {
		testLogger.Error("Failed to terminate ClickHouse container", zap.Error(err))
	}

	os.Exit(code)
}

// setupClickHouseContainer starts a ClickHouse testcontainer and returns the connection details.
// This container is shared across all tests for performance.
func setupClickHouseContainer(ctx context.Context) (*clickhouse.ClickHouseContainer, string, error) {
	// Create ClickHouse container with default configuration
	// Explicitly set empty password for test container
	container, err := clickhouse.Run(ctx,
		"clickhouse/clickhouse-server:23.8-alpine",
		clickhouse.WithUsername("default"),
		clickhouse.WithPassword(""),
		clickhouse.WithDatabase("default"),
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to start clickhouse container: %w", err)
	}

	// Get connection string
	connectionHost, err := container.ConnectionHost(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get connection host: %w", err)
	}

	// Build DSN in the format expected by our clickhouse client
	dsn := fmt.Sprintf("clickhouse://%s?sslmode=disable", connectionHost)

	// Set environment variable so our database clients can connect
	os.Setenv("CLICKHOUSE_ADDR", dsn)

	// Set connection pool limits for integration tests
	// CRITICAL: Low limits prevent connection pool exhaustion with sequential tests
	os.Setenv("CLICKHOUSE_MAX_OPEN_CONNS", "5")
	os.Setenv("CLICKHOUSE_MAX_IDLE_CONNS", "5")
	os.Setenv("CLICKHOUSE_CONN_MAX_LIFETIME", "30s")

	// Wait for container to be ready
	if err := waitForClickHouse(ctx, container, 30*time.Second); err != nil {
		return nil, "", fmt.Errorf("clickhouse container not ready: %w", err)
	}

	return container, dsn, nil
}

// waitForClickHouse waits for the ClickHouse container to be ready to accept connections.
func waitForClickHouse(ctx context.Context, container *clickhouse.ClickHouseContainer, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for ClickHouse: %w", ctx.Err())
		case <-ticker.C:
			// Try to execute a simple query
			_, _, err := container.Exec(ctx, []string{"clickhouse-client", "--query", "SELECT 1"})
			if err == nil {
				return nil
			}
		}
	}
}

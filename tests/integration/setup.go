//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/clickhouse"
	"go.uber.org/zap"
)

var (
	testDB        *db.AdminDB
	testContainer *clickhouse.ClickHouseContainer
	testLogger    *zap.Logger
)

// TestMain sets up the ClickHouse container and runs all integration tests
func TestMain(m *testing.M) {
	var exitCode int
	defer func() {
		os.Exit(exitCode)
	}()

	ctx := context.Background()

	// Initialize logger
	var err error
	testLogger, err = zap.NewDevelopment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		exitCode = 1
		return
	}

	// Check if Docker is available
	if !isDockerAvailable(ctx) {
		fmt.Println("Docker not available, skipping integration tests")
		exitCode = 0
		return
	}

	// Start ClickHouse container
	testLogger.Info("Starting ClickHouse container...")
	testContainer, err = clickhouse.Run(ctx,
		"clickhouse/clickhouse-server:24.1",
		clickhouse.WithUsername("default"),
		clickhouse.WithPassword(""),
		clickhouse.WithDatabase("test_canopyx"),
		testcontainers.WithEnv(map[string]string{
			"CLICKHOUSE_DB":       "test_canopyx",
			"CLICKHOUSE_USER":     "default",
			"CLICKHOUSE_PASSWORD": "",
		}),
	)
	if err != nil {
		testLogger.Error("Failed to start ClickHouse container", zap.Error(err))
		exitCode = 1
		return
	}

	// Get connection details
	host, err := testContainer.Host(ctx)
	if err != nil {
		testLogger.Error("Failed to get container host", zap.Error(err))
		cleanup(ctx)
		exitCode = 1
		return
	}

	port, err := testContainer.MappedPort(ctx, "9000/tcp")
	if err != nil {
		testLogger.Error("Failed to get container port", zap.Error(err))
		cleanup(ctx)
		exitCode = 1
		return
	}

	// Set environment variable for database connection
	dsn := fmt.Sprintf("clickhouse://%s:%s/test_canopyx?sslmode=disable", host, port.Port())
	os.Setenv("CLICKHOUSE_ADDR", dsn)

	testLogger.Info("ClickHouse container started",
		zap.String("host", host),
		zap.String("port", port.Port()),
		zap.String("dsn", dsn),
	)

	// Initialize database connection
	client, err := db.NewDB(ctx, testLogger, "test_canopyx")
	if err != nil {
		testLogger.Error("Failed to connect to ClickHouse", zap.Error(err))
		cleanup(ctx)
		exitCode = 1
		return
	}

	testDB = &db.AdminDB{
		Client: client,
		Name:   "test_canopyx",
	}

	// Initialize database schema
	testLogger.Info("Initializing database schema...")
	if err := testDB.InitializeDB(ctx); err != nil {
		testLogger.Error("Failed to initialize database", zap.Error(err))
		cleanup(ctx)
		exitCode = 1
		return
	}

	testLogger.Info("Database initialized successfully")

	// Run tests
	exitCode = m.Run()

	// Cleanup
	cleanup(ctx)
}

// cleanup terminates the container and closes database connections
func cleanup(ctx context.Context) {
	if testDB != nil {
		testLogger.Info("Closing database connection...")
		if err := testDB.Close(); err != nil {
			testLogger.Error("Failed to close database connection", zap.Error(err))
		}
	}

	if testContainer != nil {
		testLogger.Info("Terminating ClickHouse container...")
		terminateCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		if err := testContainer.Terminate(terminateCtx); err != nil {
			testLogger.Error("Failed to terminate container", zap.Error(err))
		}
	}

	testLogger.Info("Cleanup completed")
}

// isDockerAvailable checks if Docker is available on the system
func isDockerAvailable(ctx context.Context) bool {
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return false
	}
	defer provider.Close()

	return true
}

// cleanDB truncates all tables in the test database
func cleanDB(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	// Truncate chains table
	if _, err := testDB.Db.ExecContext(ctx, `TRUNCATE TABLE IF EXISTS test_canopyx.chains`); err != nil {
		t.Logf("Warning: failed to truncate chains table: %v", err)
	}

	// Truncate index_progress table
	if _, err := testDB.Db.ExecContext(ctx, `TRUNCATE TABLE IF EXISTS test_canopyx.index_progress`); err != nil {
		t.Logf("Warning: failed to truncate index_progress table: %v", err)
	}

	// Truncate index_progress_agg table
	if _, err := testDB.Db.ExecContext(ctx, `TRUNCATE TABLE IF EXISTS test_canopyx.index_progress_agg`); err != nil {
		t.Logf("Warning: failed to truncate index_progress_agg table: %v", err)
	}

	// Truncate reindex_requests table
	if _, err := testDB.Db.ExecContext(ctx, `TRUNCATE TABLE IF EXISTS test_canopyx.reindex_requests`); err != nil {
		t.Logf("Warning: failed to truncate reindex_requests table: %v", err)
	}

	t.Log("Database cleaned successfully")
}

// seedData inserts test data into the database
func seedData(t *testing.T, opts ...SeedOption) {
	t.Helper()
	ctx := context.Background()

	config := &seedConfig{}
	for _, opt := range opts {
		opt(config)
	}

	// Seed chains if provided
	for _, chain := range config.chains {
		if err := testDB.UpsertChain(ctx, chain); err != nil {
			t.Fatalf("Failed to seed chain %s: %v", chain.ChainID, err)
		}
	}

	// Seed index progress if provided
	for _, progress := range config.indexProgress {
		if err := testDB.RecordIndexed(ctx, progress.ChainID, progress.Height); err != nil {
			t.Fatalf("Failed to seed index progress for chain %s height %d: %v",
				progress.ChainID, progress.Height, err)
		}
	}

	t.Logf("Seeded %d chains and %d index progress records",
		len(config.chains), len(config.indexProgress))
}

// SeedOption is a functional option for seeding test data
type SeedOption func(*seedConfig)

type seedConfig struct {
	chains        []*admin.Chain
	indexProgress []*admin.IndexProgress
}

// WithChains adds chains to seed
func WithChains(chains ...*admin.Chain) SeedOption {
	return func(c *seedConfig) {
		c.chains = append(c.chains, chains...)
	}
}

// WithIndexProgress adds index progress records to seed
func WithIndexProgress(progress ...*admin.IndexProgress) SeedOption {
	return func(c *seedConfig) {
		c.indexProgress = append(c.indexProgress, progress...)
	}
}

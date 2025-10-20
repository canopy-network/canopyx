package helpers

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/clickhouse"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type AdminDBWrapper struct {
	*db.AdminDB
}

func (a *AdminDBWrapper) RecordIndexed(ctx context.Context, chainID string, height uint64) error {
	return a.AdminDB.RecordIndexed(ctx, chainID, height, 0, "")
}

func (a *AdminDBWrapper) RecordIndexedWithMetrics(ctx context.Context, chainID string, height uint64, indexingTimeMs float64, indexingDetail string) error {
	return a.AdminDB.RecordIndexed(ctx, chainID, height, indexingTimeMs, indexingDetail)
}

var (
	testDB        *AdminDBWrapper
	testContainer *clickhouse.ClickHouseContainer
	testLogger    *zap.Logger
)

// Run bootstraps the ClickHouse container, executes the supplied tests, and tears everything down.
func Run(m *testing.M) int {
	ctx := context.Background()

	var err error
	if os.Getenv("CHDEBUG") == "" {
		_ = os.Setenv("CHDEBUG", "0")
	}

	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	testLogger, err = cfg.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		return 1
	}

	if !isDockerAvailable(ctx) {
		fmt.Println("Docker not available, skipping integration tests")
		return 0
	}

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
		return 1
	}

	cleanupCtx := ctx
	cleanupNeeded := true
	defer func() {
		if cleanupNeeded {
			cleanup(cleanupCtx)
		}
	}()

	host, err := testContainer.Host(ctx)
	if err != nil {
		testLogger.Error("Failed to get container host", zap.Error(err))
		return 1
	}

	port, err := testContainer.MappedPort(ctx, "9000/tcp")
	if err != nil {
		testLogger.Error("Failed to get container port", zap.Error(err))
		return 1
	}

	dsn := fmt.Sprintf("clickhouse://%s:%s/test_canopyx?sslmode=disable", host, port.Port())
	_ = os.Setenv("CLICKHOUSE_ADDR", dsn)
	testLogger.Info("ClickHouse container started",
		zap.String("host", host),
		zap.String("port", port.Port()),
		zap.String("dsn", dsn),
	)

	client, err := db.NewDB(ctx, testLogger, "test_canopyx")
	if err != nil {
		testLogger.Error("Failed to connect to ClickHouse", zap.Error(err))
		return 1
	}

	baseDB := &db.AdminDB{
		Client: client,
		Name:   "test_canopyx",
	}
	testDB = &AdminDBWrapper{AdminDB: baseDB}

	testLogger.Info("Initializing database schema...")
	if err := testDB.InitializeDB(ctx); err != nil {
		testLogger.Error("Failed to initialize database", zap.Error(err))
		return 1
	}

	testLogger.Info("Database initialized successfully")

	exitCode := m.Run()
	cleanupNeeded = false
	cleanup(cleanupCtx)

	return exitCode
}

func TestDB() *AdminDBWrapper {
	return testDB
}

func NewChainDb(ctx context.Context, chainID string) (*db.ChainDB, error) {
	return db.NewChainDb(ctx, testLogger, chainID)
}

func SkipIfNoDB(ctx context.Context, t *testing.T) bool {
	t.Helper()
	if testDB == nil || testDB.Db == nil {
		t.Skip("test database not initialized (Docker unavailable?)")
		return true
	}
	return false
}

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

func isDockerAvailable(ctx context.Context) bool {
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return false
	}
	defer func(provider *testcontainers.DockerProvider) {
		_ = provider.Close()
	}(provider)
	return true
}

package types

import (
	"context"
	"errors"
	"fmt"
	"github.com/canopy-network/canopyx/pkg/temporal/indexer"
	"net/http"
	"strconv"
	"time"

	adminstore "github.com/canopy-network/canopyx/pkg/db/admin"
	chainstore "github.com/canopy-network/canopyx/pkg/db/chain"
	"github.com/canopy-network/canopyx/pkg/redis"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"github.com/puzpuzpuz/xsync/v4"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
)

type App struct {
	// Database Client wrappers
	AdminDB  *adminstore.DB
	ChainsDB *xsync.Map[string, chainstore.Store]

	// Temporal Client wrapper
	TemporalClient *temporal.Client

	// Redis Client (for WebSocket real-time events)
	RedisClient *redis.Client

	// Zap Logger
	Logger *zap.Logger

	// HTTP Server
	Server *http.Server

	// Queue stats cache (30s TTL to reduce Temporal API calls)
	QueueStatsCache *xsync.Map[string, CachedQueueStats]
}

// Start starts the application.
func (a *App) Start(ctx context.Context) {
	go func() { _ = a.Server.ListenAndServe() }()
	<-ctx.Done()

	a.Logger.Info("closing admin database connection")
	err := a.AdminDB.Close()
	if err != nil {
		a.Logger.Error("Failed to close database connection", zap.Error(err))
	}

	// Close all chain database connections
	a.ChainsDB.Range(func(key string, chainStore chainstore.Store) bool {
		a.Logger.Info("closing chain database connection", zap.String("chainID", chainStore.ChainKey()))
		err = chainStore.Close()
		if err != nil {
			a.Logger.Error("Failed to close database connection", zap.Error(err))
		}
		return true
	})

	a.Logger.Info("shutting down server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = a.Server.Shutdown(shutdownCtx)

	time.Sleep(200 * time.Millisecond)
	a.Logger.Info("さようなら!")
}

// NewChainDb initializes or retrieves an instance of ChainDB for a given blockchain identified by chainID.
// It ensures the database and required tables are created if not already present.
// Returns the ChainDB instance or an error in case of failure.
func (a *App) NewChainDb(ctx context.Context, chainID string) (chainstore.Store, error) {
	if chainDb, ok := a.ChainsDB.Load(chainID); ok {
		// chainDb is already loaded
		return chainDb, nil
	}

	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return nil, fmt.Errorf("invalid chain ID format: %w", parseErr)
	}

	chainDB, chainDBErr := chainstore.New(ctx, a.Logger, chainIDUint)
	if chainDBErr != nil {
		return nil, chainDBErr
	}

	a.ChainsDB.Store(chainID, chainDB)

	return chainDB, nil
}

// ReconcileSchedules ensures the required schedules for indexing are created if they do not already exist.
func (a *App) ReconcileSchedules(ctx context.Context) error {
	chains, err := a.AdminDB.ListChain(ctx)
	if err != nil {
		return err
	}

	for _, c := range chains {
		chainIDStr := strconv.FormatUint(c.ChainID, 10)
		if chainScheduleErr := a.EnsureChainSchedules(ctx, chainIDStr); chainScheduleErr != nil {
			return chainScheduleErr
		}
	}
	return nil
}

// EnsureHeadSchedule ensures the required schedules for indexing are created if they do not already exist.
func (a *App) EnsureHeadSchedule(ctx context.Context, chainID string) error {
	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}
	id := a.TemporalClient.GetHeadScheduleID(chainIDUint)
	h := a.TemporalClient.TSClient.GetHandle(ctx, id)
	_, err := h.Describe(ctx)
	if err == nil {
		a.Logger.Info("Head scan schedule already exists", zap.String("id", id), zap.String("chainID", chainID))
		return nil
	}

	var notFound *serviceerror.NotFound
	if errors.As(err, &notFound) {
		_, scheduleErr := a.TemporalClient.TSClient.Create(
			ctx, client.ScheduleOptions{
				ID:   id,
				Spec: a.TemporalClient.TwoSecondSpec(),
				Action: &client.ScheduleWorkflowAction{
					Workflow:  indexer.HeadScanWorkflowName,
					Args:      []interface{}{indexer.HeadScanInput{ChainID: chainIDUint}},
					TaskQueue: a.TemporalClient.GetIndexerOpsQueue(chainIDUint),
				},
			},
		)
		return scheduleErr
	}
	return err
}

// EnsureGapScanSchedule ensures the required schedules for indexing are created if they do not already exist.
func (a *App) EnsureGapScanSchedule(ctx context.Context, chainID string) error {
	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}
	id := a.TemporalClient.GetGapScanScheduleID(chainIDUint)
	h := a.TemporalClient.TSClient.GetHandle(ctx, id)
	_, err := h.Describe(ctx)
	if err == nil {
		a.Logger.Info("Gap scan schedule already exists", zap.String("id", id), zap.String("chainID", chainID))
		return nil
	}

	var notFound *serviceerror.NotFound
	if errors.As(err, &notFound) {
		a.Logger.Info("Creating gap scan schedule", zap.String("id", id))
		_, scheduleErr := a.TemporalClient.TSClient.Create(
			ctx,
			client.ScheduleOptions{
				ID:   id,
				Spec: a.TemporalClient.OneHourSpec(),
				Action: &client.ScheduleWorkflowAction{
					Workflow:  indexer.GapScanWorkflowName,
					Args:      []interface{}{indexer.GapScanInput{ChainID: chainIDUint}},
					TaskQueue: a.TemporalClient.GetIndexerOpsQueue(chainIDUint),
				},
			},
		)
		return scheduleErr
	}
	return err
}

// EnsureChainSchedules ensures the required schedules for indexing are created if they do not already exist.
func (a *App) EnsureChainSchedules(ctx context.Context, chainID string) error {
	if err := a.EnsureHeadSchedule(ctx, chainID); err != nil {
		return err
	}

	if err := a.EnsureGapScanSchedule(ctx, chainID); err != nil {
		return err
	}

	return nil
}

// LoadChainStore loads or creates a chain store for the given chain ID.
// Returns the store and a boolean indicating if the chain was loaded successfully.
func (a *App) LoadChainStore(ctx context.Context, chainID string) (chainstore.Store, bool) {
	if store, ok := a.ChainsDB.Load(chainID); ok {
		return store, true
	}

	// Try to create the store if it doesn't exist
	store, err := a.NewChainDb(ctx, chainID)
	if err != nil {
		a.Logger.Error("Failed to load chain store", zap.String("chainID", chainID), zap.Error(err))
		return nil, false
	}

	return store, true
}

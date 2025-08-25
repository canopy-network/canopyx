package types

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/canopy-network/canopyx/pkg/indexer/types"

	"github.com/canopy-network/canopyx/app/admin/workflow"
	"github.com/canopy-network/canopyx/pkg/db"
	reporterworkflows "github.com/canopy-network/canopyx/pkg/reporter/workflow"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"github.com/puzpuzpuz/xsync/v4"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.uber.org/zap"
)

type App struct {
	// Database Client wrappers
	AdminDB  *db.AdminDB
	ReportDB *db.ReportsDB
	ChainsDB *xsync.Map[string, *db.ChainDB]

	// Temporal Client wrapper
	TemporalClient *temporal.Client

	// Temporal Worker Client
	Worker worker.Worker

	// Zap Logger
	Logger *zap.Logger

	// Contexts
	AdminWorkflowContext    *workflow.Context
	ReporterWorkflowContext *reporterworkflows.Context

	// HTTP Server
	Server *http.Server
}

// Start starts the application.
func (a *App) Start(ctx context.Context) {
	go func() { _ = a.Server.ListenAndServe() }()
	<-ctx.Done()

	a.Logger.Info("closing admin database connection")
	err := a.AdminDB.Db.Close()
	if err != nil {
		a.Logger.Error("Failed to close database connection", zap.Error(err))
	}

	a.Logger.Info("closing reports database connection")
	err = a.ReportDB.Db.Close()
	if err != nil {
		a.Logger.Error("Failed to close database connection", zap.Error(err))
	}

	// Close all chain database connections
	a.ChainsDB.Range(func(key string, db *db.ChainDB) bool {
		a.Logger.Info("closing chain database connection", zap.String("chainID", db.ChainID))
		err = db.Db.Close()
		if err != nil {
			a.Logger.Error("Failed to close database connection", zap.Error(err))
		}
		return true
	})

	a.Logger.Info("closing temporal worker")
	a.Worker.Stop()

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
func (a *App) NewChainDb(ctx context.Context, chainID string) (*db.ChainDB, error) {
	if chainDb, ok := a.ChainsDB.Load(chainID); ok {
		// chainDb is already loaded
		return chainDb, nil
	}

	chainDB, chainDBErr := db.NewChainDb(ctx, a.Logger, chainID)
	if chainDBErr != nil {
		return nil, chainDBErr
	}

	a.ChainsDB.Store(chainID, chainDB)

	return chainDB, nil
}

// ReconcileSchedules ensures the required schedules for indexing are created if they do not already exist.
func (a *App) ReconcileSchedules(ctx context.Context) error {
	globalReportsScheduleErr := a.EnsureGlobalReportsSchedule(ctx)
	if globalReportsScheduleErr != nil {
		return globalReportsScheduleErr
	}

	chains, err := a.AdminDB.ListChain(ctx)
	if err != nil {
		return err
	}

	for _, c := range chains {
		if chainScheduleErr := a.EnsureChainSchedules(ctx, c.ChainID); chainScheduleErr != nil {
			return chainScheduleErr
		}
	}
	return nil
}

// EnsureGlobalReportsSchedule ensures the global reports schedule is created if it does not already exist.
func (a *App) EnsureGlobalReportsSchedule(ctx context.Context) error {
	id := a.TemporalClient.GetGlobalReportsScheduleID()
	h := a.TemporalClient.TSClient.GetHandle(ctx, id)
	_, err := h.Describe(ctx)
	if err == nil {
		return nil
	}

	var notFound *serviceerror.NotFound
	if errors.As(err, &notFound) {
		a.Logger.Info("Creating global reports schedule", zap.String("id", id))
		_, scheduleErr := a.TemporalClient.TSClient.Create(
			ctx,
			client.ScheduleOptions{
				ID:   id,
				Spec: a.TemporalClient.ThreeMinuteSpec(),
				Action: &client.ScheduleWorkflowAction{
					Workflow:  a.ReporterWorkflowContext.ComputeTxStatsWorkflow,
					TaskQueue: a.TemporalClient.ReportsQueue,
				},
			},
		)
		return scheduleErr
	}
	return err
}

// EnsureHeadSchedule ensures the required schedules for indexing are created if they do not already exist.
func (a *App) EnsureHeadSchedule(ctx context.Context, chainID string) error {
	id := a.TemporalClient.GetHeadScheduleID(chainID)
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
				Spec: a.TemporalClient.TenSecondSpec(),
				Action: &client.ScheduleWorkflowAction{
					Workflow:  a.AdminWorkflowContext.HeadScan,
					Args:      []interface{}{types.ChainIdInput{ChainID: chainID}},
					TaskQueue: a.TemporalClient.ManagerQueue,
				},
			},
		)
		return scheduleErr
	}
	return err
}

// EnsureGapScanSchedule ensures the required schedules for indexing are created if they do not already exist.
func (a *App) EnsureGapScanSchedule(ctx context.Context, chainID string) error {
	id := a.TemporalClient.GetGapScanScheduleID(chainID)
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
					Workflow:  a.AdminWorkflowContext.GapScanWorkflow,
					Args:      []interface{}{types.ChainIdInput{ChainID: chainID}},
					TaskQueue: a.TemporalClient.ManagerQueue,
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

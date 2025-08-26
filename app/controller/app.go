package controller

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/admin"

	"github.com/canopy-network/canopyx/pkg/logging"
	"github.com/canopy-network/canopyx/pkg/utils"
	"github.com/gorilla/mux"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/puzpuzpuz/xsync/v4"
)

const queuePrefix = "index:"

// App reads the desired state from IndexerDB (chain table) and reconciles
// the real world via a Provider (e.g. Kubernetes), every Cron tick.
type App struct {
	// Clickhouse DB
	DBClient  *db.Client
	IndexerDB *db.AdminDB

	// Cron is the scheduler that triggers reconciliation tasks at specified intervals, according to CronSpec.
	Cron     *cron.Cron
	CronSpec string

	// Provider (fake or k8s)
	Provider Provider

	// Running tracks chains we believe are applied; helps us decide to stop /delete.
	Running *xsync.Map[string, *Chain]

	// Logger is used to log messages, errors, and events during the application's lifecycle and operations.
	Logger *zap.Logger

	// Server is the HTTP server that serves the API.
	Server *http.Server
}

// Initialize initializes the App.
func Initialize(ctx context.Context, provider Provider) (*App, error) {
	logger, err := logging.New()
	if err != nil {
		// nothing else to do here, we'll just log to stderr'
		panic(err)
	}
	// Scope the logger for this component
	logger = logger.With(zap.String("component", "controller"))

	indexerDb, _, basicDbsErr := db.NewBasicDbs(ctx, logger)
	if basicDbsErr != nil {
		logger.Fatal("unable to initialize basic databases", zap.Error(basicDbsErr))
	}

	app := &App{
		IndexerDB: indexerDb,
		Cron:      nil,
		CronSpec:  "*/15 * * * * *", // TODO: allow this to be set via env var?
		Provider:  provider,
		Running:   xsync.NewMap[string, *Chain](),
		Logger:    logger,
	}

	if err := app.SetupScheduler(ctx, cron.DefaultLogger, app.CronSpec); err != nil {
		logger.Error("failed to setup scheduler", zap.Error(err), zap.String("cronSpec", app.CronSpec))
		return nil, err
	}

	logger.Info("app initialized", zap.String("cronSpec", app.CronSpec))
	return app, nil
}

// SetupServer sets up the HTTP server.
func (a *App) SetupServer() {
	// use <ip>:<port> to bind to a specific interface or :<port> to bind to all interfaces
	addr := utils.Env("ADDR", ":3002")

	r := mux.NewRouter()

	r.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })).Methods("GET")
	r.Handle("/readyz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if a.Ready() {
			w.WriteHeader(200)
		} else {
			w.WriteHeader(503)
		}
	})).Methods("GET")

	a.Server = &http.Server{Addr: addr, Handler: r}
	a.Logger.Info("http server configured", zap.String("addr", addr))
}

// SetupScheduler sets up the cron scheduler.
func (a *App) SetupScheduler(ctx context.Context, logger cron.Logger, cronSpec string) error {
	// Seconds field, optional
	a.Cron = cron.New(cron.WithSeconds(), cron.WithChain(cron.Recover(logger)))

	_, err := a.Cron.AddFunc(cronSpec, func() {
		start := time.Now()
		// keep each run bounded
		rctx, cancel := context.WithTimeout(ctx, 25*time.Second)
		defer cancel()

		a.Logger.Info("reconcile tick started",
			zap.String("cronSpec", cronSpec),
			zap.Duration("timeout", 25*time.Second),
		)

		if err := a.Reconcile(rctx); err != nil {
			a.Logger.Error("reconcile tick failed",
				zap.Error(err),
				zap.Duration("elapsed", time.Since(start)),
			)
			return
		}

		a.Logger.Info("reconcile tick finished", zap.Duration("elapsed", time.Since(start)))
	})
	if err != nil {
		return err
	}

	a.Logger.Info("scheduler configured", zap.String("cronSpec", cronSpec))
	return nil
}

// StartCron starts the cron scheduler.
func (a *App) StartCron() {
	a.Cron.Start()
	a.Logger.Info("cron started", zap.String("cronSpec", a.CronSpec))
}

// StopCron stops the cron scheduler.
func (a *App) StopCron() {
	if a.Cron != nil {
		<-a.Cron.Stop().Done()
	}
	if err := a.Provider.Close(); err != nil {
		a.Logger.Warn("provider close returned error", zap.Error(err))
	}
	a.Logger.Info("cron stopped")
}

// Reconcile fetches the desired state from ClickHouse and applies it via Provider.
func (a *App) Reconcile(ctx context.Context) error {
	runStart := time.Now()
	a.Logger.Info("reconcile started")

	des, err := a.loadDesired(ctx)
	if err != nil {
		a.Logger.Error("load desired failed", zap.Error(err))
		return err
	}
	a.Logger.Info("desired loaded", zap.Int("count", len(des)))

	// Compute and apply changes.
	desiredSet := make(map[string]*Chain, len(des))

	var ensured, paused, deleted, unchanged int

	for idx := range des {
		ch := &des[idx]
		desiredSet[ch.ID] = ch

		queueId := fmt.Sprintf("%s%s", queuePrefix, ch.ID)
		fields := []zap.Field{
			zap.String("chain_id", ch.ID),
			zap.Bool("paused", ch.Paused),
			zap.Bool("deleted", ch.Deleted),
			zap.String("queue_id", queueId),
		}

		switch {
		case ch.Deleted:
			a.Logger.Info("delete calculated", fields...)
			if err := a.Provider.DeleteChain(ctx, ch.ID); err != nil {
				a.Logger.Error("delete failed", append(fields, zap.Error(err))...)
				return fmt.Errorf("delete %s: %w", ch.ID, err)
			}
			a.Running.Delete(queueId)
			deleted++

		case ch.Paused:
			a.Logger.Info("pause calculated", fields...)
			if err := a.Provider.PauseChain(ctx, ch.ID); err != nil {
				a.Logger.Error("pause failed", append(fields, zap.Error(err))...)
				return fmt.Errorf("pause %s: %w", ch.ID, err)
			}
			a.Running.Store(queueId, ch)
			paused++

		default:
			// If it's already recorded as running, we may treat as unchanged after EnsureChain succeeds.
			_, previouslyRunning := a.Running.Load(queueId)
			a.Logger.Info("ensure calculated", fields...)
			if err := a.Provider.EnsureChain(ctx, ch); err != nil {
				a.Logger.Error("ensure failed", append(fields, zap.Error(err))...)
				return fmt.Errorf("ensure %s: %w", ch.ID, err)
			}
			a.Running.Store(queueId, ch)
			if previouslyRunning {
				unchanged++
			} else {
				ensured++
			}
		}
	}

	// If any previously Running chain disappeared from desired, delete it.
	var pruned int
	a.Running.Range(func(q string, prev *Chain) bool {
		chainID := q[len(queuePrefix):]
		if _, ok := desiredSet[chainID]; !ok {
			if err := a.Provider.DeleteChain(ctx, chainID); err != nil {
				a.Logger.Warn("prune delete failed", zap.String("chain_id", chainID), zap.String("queue_id", q), zap.Error(err))
			} else {
				a.Logger.Info("pruned missing chain", zap.String("chain_id", chainID), zap.String("queue_id", q))
			}
			a.Running.Delete(q)
			pruned++
		}
		return true
	})

	a.Logger.Info("reconcile finished",
		zap.Duration("elapsed", time.Since(runStart)),
		zap.Int("desired_total", len(des)),
		zap.Int("ensured", ensured),
		zap.Int("paused", paused),
		zap.Int("deleted", deleted),
		zap.Int("unchanged", unchanged),
		zap.Int("pruned", pruned),
	)

	return nil
}

// loadDesired queries chains FINAL with paused/deleted flags.
func (a *App) loadDesired(ctx context.Context) ([]Chain, error) {
	loadStart := time.Now()

	// Pull only the necessary columns; FINAL ensures we see the latest row per PK.
	var rows []admin.Chain
	err := a.IndexerDB.Db.
		NewSelect().
		Model(&rows).
		// NOTE: keep only the columns we need for the scheduler
		Column("chain_id", "paused", "deleted").
		Final().
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]Chain, 0, len(rows))
	for _, r := range rows {
		out = append(out, Chain{
			ID:      r.ChainID,
			Paused:  r.Paused != 0,
			Deleted: r.Deleted != 0,
		})
	}

	a.Logger.Debug("desired loaded from db",
		zap.Int("row_count", len(rows)),
		zap.Duration("elapsed", time.Since(loadStart)),
	)
	return out, nil
}

// ReconcileOnce is a convenience wrapper for Reconcile.
func (a *App) ReconcileOnce(ctx context.Context) {
	if err := a.Reconcile(ctx); err != nil {
		a.Logger.Error("reconcile once failed", zap.Error(err))
	}
}

// TODO: expose a health probe in the right way (check db connection, cron, etc)

// Ready indicates whether the application is ready to handle operations, returning true if ready.
func (a *App) Ready() bool { return true }

// Alive indicates whether the application is alive, returning true if alive.
func (a *App) Alive() bool { return true }

// Start starts the application.
func (a *App) Start(ctx context.Context) {
	if a.Server == nil {
		a.Logger.Warn("http server not configured; call SetupServer() before Start()")
	}
	addr := ""
	if a.Server != nil {
		addr = a.Server.Addr
	}

	go func() {
		a.Logger.Info("http server starting", zap.String("addr", addr))
		if err := a.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.Logger.Error("http server error", zap.Error(err))
		}
	}()

	a.Logger.Info("app started")
	<-ctx.Done()
	a.Logger.Info("shutdown initiated")

	if a.Server != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.Server.Shutdown(shutdownCtx); err != nil {
			a.Logger.Warn("http server shutdown error", zap.Error(err))
		} else {
			a.Logger.Info("http server stopped")
		}
	}

	a.StopCron()
	time.Sleep(200 * time.Millisecond)
	a.Logger.Info("goodbye")
}

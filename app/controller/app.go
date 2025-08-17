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

	indexerDb, _, basicDbsErr := db.NewBasicDbs(ctx, logger)
	if basicDbsErr != nil {
		logger.Fatal("Unable to initialize basic databases", zap.Error(basicDbsErr))
	}

	app := &App{
		IndexerDB: indexerDb,
		Cron:      nil,
		CronSpec:  "*/15 * * * * *", // TODO: allow this to be set via env var?
		Provider:  provider,
		Running:   xsync.NewMap[string, *Chain](),
		Logger:    logger,
	}

	scheduleErr := app.SetupScheduler(ctx, cron.DefaultLogger, app.CronSpec)
	if scheduleErr != nil {
		return nil, scheduleErr
	}

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
}

// SetupScheduler sets up the cron scheduler.
func (a *App) SetupScheduler(ctx context.Context, logger cron.Logger, cronSpec string) error {
	// Seconds field, optional
	a.Cron = cron.New(cron.WithSeconds(), cron.WithChain(cron.Recover(logger)))

	_, err := a.Cron.AddFunc(cronSpec, func() {
		// keep each run bounded
		rctx, cancel := context.WithTimeout(ctx, 25*time.Second)
		defer cancel()
		if err := a.Reconcile(rctx); err != nil {
			logger.Info("[controller] reconcile error", "error", err)
		}
	})
	if err != nil {
		return err
	}

	return nil
}

// StartCron starts the cron scheduler.
func (a *App) StartCron() {
	a.Cron.Start()
	a.Logger.Info("[controller] Cron started", zap.String("cronSpec", a.CronSpec))
}

// StopCron stops the cron scheduler.
func (a *App) StopCron() {
	if a.Cron != nil {
		<-a.Cron.Stop().Done()
	}
	_ = a.Provider.Close()
}

// Reconcile fetches the desired state from ClickHouse and applies it via Provider.
func (a *App) Reconcile(ctx context.Context) error {
	des, err := a.loadDesired(ctx)
	if err != nil {
		return err
	}

	// Compute and apply changes.
	desiredSet := make(map[string]*Chain, len(des))
	for idx := range des {
		ch := &des[idx]
		desiredSet[ch.ID] = ch
		queueId := fmt.Sprintf("%s%s", queuePrefix, ch.ID)
		// Deleted wins
		switch {
		case ch.Deleted:
			if err := a.Provider.DeleteChain(ctx, ch.ID); err != nil {
				return fmt.Errorf("delete %s: %w", ch.ID, err)
			}
			a.Running.Delete(queueId)
		case ch.Paused:
			if err := a.Provider.PauseChain(ctx, ch.ID); err != nil {
				return fmt.Errorf("pause %s: %w", ch.ID, err)
			}
			a.Running.Store(queueId, ch)
		default:
			if err := a.Provider.EnsureChain(ctx, ch); err != nil {
				return fmt.Errorf("ensure %s: %w", ch.ID, err)
			}
			a.Running.Store(queueId, ch)
		}
	}

	// If any previously Running chain disappeared from desired, delete it.
	a.Running.Range(func(q string, prev *Chain) bool {
		chainID := q[len(queuePrefix):]
		if _, ok := desiredSet[chainID]; !ok {
			_ = a.Provider.DeleteChain(ctx, chainID)
			a.Running.Delete(q)
		}
		return true
	})

	return nil
}

// loadDesired queries chains FINAL with paused/deleted flags.
func (a *App) loadDesired(ctx context.Context) ([]Chain, error) {
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
	return out, nil
}

// ReconcileOnce is a convenience wrapper for Reconcile.
func (a *App) ReconcileOnce(ctx context.Context) {
	_ = a.Reconcile(ctx)
}

// TODO: expose a health probe in the right way (check db connection, cron, etc)

// Ready indicates whether the application is ready to handle operations, returning true if ready.
func (a *App) Ready() bool { return true }

// Alive indicates whether the application is alive, returning true if alive.
func (a *App) Alive() bool { return true }

// Start starts the application.
func (a *App) Start(ctx context.Context) {
	go func() { _ = a.Server.ListenAndServe() }()
	<-ctx.Done()
	_ = a.Server.Close()
	a.Logger.Info("[controller] shutting down…")
	a.StopCron()
	time.Sleep(200 * time.Millisecond)
	a.Logger.Info("さようなら!")
}

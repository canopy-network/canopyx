package controller

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/admin"

	"github.com/canopy-network/canopyx/pkg/logging"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"github.com/canopy-network/canopyx/pkg/utils"
	"github.com/gorilla/mux"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"github.com/canopy-network/canopyx/pkg/db"
)

const (
	backlogLowWatermark  = int64(10)
	backlogHighWatermark = int64(1000)
	queueRequestTimeout  = 2 * time.Second
	queueStatsTTL        = 30 * time.Second // Increased from 10s to reduce Temporal API calls
	scaleCooldown        = 60 * time.Second
)

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

	Temporal *temporal.Client

	queueCache *xsync.Map[string, cachedQueueStats]

	// Logger is used to log messages, errors, and events during the application's lifecycle and operations.
	Logger *zap.Logger

	// Server is the HTTP server that serves the API.
	Server *http.Server
}

type cachedQueueStats struct {
	stats           QueueStats // Aggregated stats for scaling decisions
	liveStats       QueueStats // Live queue stats for visibility
	historicalStats QueueStats // Historical queue stats for visibility
	fetched         time.Time
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

	temporalClient, err := temporal.NewClient(ctx, logger)
	if err != nil {
		logger.Fatal("unable to initialize temporal client", zap.Error(err))
	}

	app := &App{
		IndexerDB:  indexerDb,
		Cron:       nil,
		CronSpec:   "*/15 * * * * *", // TODO: allow this to be set via env var?
		Provider:   provider,
		Running:    xsync.NewMap[string, *Chain](),
		Temporal:   temporalClient,
		queueCache: xsync.NewMap[string, cachedQueueStats](),
		Logger:     logger,
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
	if a.Temporal != nil && a.Temporal.TClient != nil {
		a.Temporal.TClient.Close()
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
		a.populateChain(ctx, ch)
		desiredSet[ch.ID] = ch

		queueId := ch.TaskQueue
		prevReplicas := int32(0)
		previouslyRunning := false
		if prev, ok := a.Running.Load(queueId); ok {
			prevReplicas = prev.Replicas
			previouslyRunning = true
		}
		fields := []zap.Field{
			zap.String("chain_id", ch.ID),
			zap.Bool("paused", ch.Paused),
			zap.Bool("deleted", ch.Deleted),
			zap.String("queue_id", queueId),
			zap.Int32("min_replicas", ch.MinReplicas),
			zap.Int32("max_replicas", ch.MaxReplicas),
			zap.Int32("desired_replicas", ch.Replicas),
			zap.Int32("previous_replicas", prevReplicas),
			zap.Int64("pending_workflow_tasks", ch.Queue.PendingWorkflowTasks),
			zap.Int64("pending_activity_tasks", ch.Queue.PendingActivityTasks),
			zap.Float64("backlog_age_seconds", ch.Queue.BacklogAgeSeconds),
			zap.Int("poller_count", ch.Queue.PollerCount),
		}

		switch {
		case ch.Deleted:
			a.Logger.Info("delete calculated", fields...)
			if err := a.Provider.DeleteChain(ctx, ch.ID); err != nil {
				a.Logger.Error("delete failed", append(fields, zap.Error(err))...)
				return fmt.Errorf("delete %s: %w", ch.ID, err)
			}
			a.Logger.Info("delete applied", fields...)
			a.Running.Delete(queueId)
			deleted++

		case ch.Paused:
			a.Logger.Info("pause calculated", fields...)
			if err := a.Provider.PauseChain(ctx, ch.ID); err != nil {
				a.Logger.Error("pause failed", append(fields, zap.Error(err))...)
				return fmt.Errorf("pause %s: %w", ch.ID, err)
			}
			a.Logger.Info("pause applied", fields...)
			a.Running.Store(queueId, ch)
			paused++

		default:
			a.Logger.Info("ensure calculated", fields...)
			if err := a.Provider.EnsureChain(ctx, ch); err != nil {
				a.Logger.Error("ensure failed", append(fields, zap.Error(err))...)
				return fmt.Errorf("ensure %s: %w", ch.ID, err)
			}
			a.Logger.Info("ensure applied", fields...)
			if ch.Replicas != prevReplicas {
				a.Logger.Info("replica change applied",
					zap.String("chain_id", ch.ID),
					zap.String("queue_id", queueId),
					zap.Int32("previous_replicas", prevReplicas),
					zap.Int32("new_replicas", ch.Replicas),
					zap.Int64("queue_backlog_total", ch.Queue.PendingWorkflowTasks+ch.Queue.PendingActivityTasks),
					zap.Float64("backlog_age_seconds", ch.Queue.BacklogAgeSeconds),
				)
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
		chainID := strings.Split(q, ":")[1]
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

	// Pull chains using AdminDB.ListChain (uses FINAL)
	rows, err := a.IndexerDB.ListChain(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]Chain, 0, len(rows))
	for _, r := range rows {
		minReplicas := int32(r.MinReplicas)
		if minReplicas <= 0 {
			minReplicas = 1
		}
		maxReplicas := int32(r.MaxReplicas)
		if maxReplicas < minReplicas {
			maxReplicas = minReplicas
		}
		out = append(out, Chain{
			ID:          r.ChainID,
			Image:       r.Image,
			Paused:      r.Paused != 0,
			Deleted:     r.Deleted != 0,
			MinReplicas: minReplicas,
			MaxReplicas: maxReplicas,
			Replicas:    minReplicas,
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

func (a *App) populateChain(ctx context.Context, ch *Chain) {
	// With dual-queue architecture, we have both live and historical queues
	// For backward compatibility with display/logging, we'll show both queue names
	ch.TaskQueue = fmt.Sprintf("%s,%s", a.Temporal.GetIndexerLiveQueue(ch.ID), a.Temporal.GetIndexerHistoricalQueue(ch.ID))

	if ch.Paused || ch.Deleted {
		ch.Replicas = 0
		return
	}

	// Fetch and update queue stats
	stats, err := a.fetchQueueStats(ctx, ch.ID)
	if err != nil {
		a.Logger.Warn("queue stats fetch failed", zap.String("chain_id", ch.ID), zap.Error(err))
		ch.Replicas = ch.MinReplicas
		// Update queue health to unknown on error
		a.updateQueueHealthStatus(ctx, ch.ID, "unknown", fmt.Sprintf("failed to fetch queue stats: %v", err))
		return
	}

	ch.Queue = stats
	a.Logger.Info("queue depth metrics",
		zap.String("chain_id", ch.ID),
		zap.String("task_queue", ch.TaskQueue),
		zap.Int64("pending_workflow_tasks", stats.PendingWorkflowTasks),
		zap.Int64("pending_activity_tasks", stats.PendingActivityTasks),
		zap.Int("pollers", stats.PollerCount),
		zap.Float64("backlog_age_seconds", stats.BacklogAgeSeconds),
		zap.Int64("queue_backlog_total", stats.PendingWorkflowTasks+stats.PendingActivityTasks),
	)
	ch.Replicas = a.desiredReplicas(ch, stats)

	// Update queue health status based on backlog
	a.updateQueueHealthStatus(ctx, ch.ID, computeQueueHealthStatus(stats), "")

	// Update deployment health status
	a.updateDeploymentHealthStatus(ctx, ch.ID)
}

// computeQueueHealthStatus determines queue health based on backlog size
func computeQueueHealthStatus(stats QueueStats) string {
	backlog := stats.PendingWorkflowTasks + stats.PendingActivityTasks

	switch {
	case backlog < backlogLowWatermark:
		return "healthy"
	case backlog >= backlogLowWatermark && backlog < backlogHighWatermark:
		return "warning"
	case backlog >= backlogHighWatermark:
		return "critical"
	default:
		return "unknown"
	}
}

// updateQueueHealthStatus updates queue health in the database
func (a *App) updateQueueHealthStatus(ctx context.Context, chainID, status, customMessage string) {
	totalBacklog := int64(0)
	liveBacklog := int64(0)
	liveBacklogAge := 0.0
	histBacklog := int64(0)
	histBacklogAge := 0.0

	if cached, ok := a.queueCache.Load(chainID); ok {
		totalBacklog = cached.stats.PendingWorkflowTasks + cached.stats.PendingActivityTasks
		liveBacklog = cached.liveStats.PendingWorkflowTasks + cached.liveStats.PendingActivityTasks
		liveBacklogAge = cached.liveStats.BacklogAgeSeconds
		histBacklog = cached.historicalStats.PendingWorkflowTasks + cached.historicalStats.PendingActivityTasks
		histBacklogAge = cached.historicalStats.BacklogAgeSeconds
	}

	message := customMessage
	if message == "" {
		// NEW: Report both queues separately for visibility
		switch status {
		case "healthy":
			message = fmt.Sprintf("Total backlog: %d tasks (below low watermark of %d) | Live queue: %d tasks (age: %.0fs) | Historical queue: %d tasks (age: %.0fs)",
				totalBacklog, backlogLowWatermark, liveBacklog, liveBacklogAge, histBacklog, histBacklogAge)
		case "warning":
			message = fmt.Sprintf("Total backlog: %d tasks (between %d and %d) | Live queue: %d tasks (age: %.0fs) | Historical queue: %d tasks (age: %.0fs)",
				totalBacklog, backlogLowWatermark, backlogHighWatermark, liveBacklog, liveBacklogAge, histBacklog, histBacklogAge)
		case "critical":
			message = fmt.Sprintf("Total backlog: %d tasks (above high watermark of %d) | Live queue: %d tasks (age: %.0fs) | Historical queue: %d tasks (age: %.0fs)",
				totalBacklog, backlogHighWatermark, liveBacklog, liveBacklogAge, histBacklog, histBacklogAge)
		default:
			message = "Queue status unknown"
		}
	}

	if err := admin.UpdateQueueHealth(ctx, a.IndexerDB.Db, chainID, status, message); err != nil {
		a.Logger.Warn("failed to update queue health status",
			zap.String("chain_id", chainID),
			zap.String("status", status),
			zap.Error(err),
		)
	} else {
		a.Logger.Debug("queue health updated",
			zap.String("chain_id", chainID),
			zap.String("status", status),
			zap.String("message", message),
		)
	}
}

// updateDeploymentHealthStatus updates deployment health in the database
func (a *App) updateDeploymentHealthStatus(ctx context.Context, chainID string) {
	status, message, err := a.Provider.GetDeploymentHealth(ctx, chainID)
	if err != nil {
		a.Logger.Warn("failed to get deployment health",
			zap.String("chain_id", chainID),
			zap.Error(err),
		)
		// Still update with unknown status
		status = "unknown"
		message = fmt.Sprintf("failed to check deployment: %v", err)
	}

	if err := admin.UpdateDeploymentHealth(ctx, a.IndexerDB.Db, chainID, status, message); err != nil {
		a.Logger.Warn("failed to update deployment health status",
			zap.String("chain_id", chainID),
			zap.String("status", status),
			zap.Error(err),
		)
	} else {
		a.Logger.Debug("deployment health updated",
			zap.String("chain_id", chainID),
			zap.String("status", status),
			zap.String("message", message),
		)
	}
}

func (a *App) fetchQueueStats(ctx context.Context, chainID string) (QueueStats, error) {
	if a.Temporal == nil {
		return QueueStats{}, fmt.Errorf("temporal client not initialized")
	}
	if cached, ok := a.queueCache.Load(chainID); ok {
		if time.Since(cached.fetched) < queueStatsTTL {
			return cached.stats, nil
		}
	}

	ctx, cancel := context.WithTimeout(ctx, queueRequestTimeout)
	defer cancel()

	// NEW: Get stats from BOTH queues (live and historical)
	liveQueue := a.Temporal.GetIndexerLiveQueue(chainID)
	historicalQueue := a.Temporal.GetIndexerHistoricalQueue(chainID)

	// Fetch live queue stats
	livePendingWF, livePendingAct, livePollers, liveBacklogAge, liveErr := a.Temporal.GetQueueStats(ctx, liveQueue)
	if liveErr != nil {
		a.Logger.Warn("failed to fetch live queue stats, continuing with historical",
			zap.String("chain_id", chainID),
			zap.String("queue", liveQueue),
			zap.Error(liveErr),
		)
		// Set to zero on error, continue with historical queue
		livePendingWF = 0
		livePendingAct = 0
		livePollers = 0
		liveBacklogAge = 0
	}

	// Fetch historical queue stats
	histPendingWF, histPendingAct, histPollers, histBacklogAge, histErr := a.Temporal.GetQueueStats(ctx, historicalQueue)
	if histErr != nil {
		a.Logger.Warn("failed to fetch historical queue stats",
			zap.String("chain_id", chainID),
			zap.String("queue", historicalQueue),
			zap.Error(histErr),
		)
		// Set to zero on error
		histPendingWF = 0
		histPendingAct = 0
		histPollers = 0
		histBacklogAge = 0
	}

	// Return error only if BOTH queues failed
	if liveErr != nil && histErr != nil {
		return QueueStats{}, fmt.Errorf("both queues failed: live=%v, historical=%v", liveErr, histErr)
	}

	// Aggregate stats for scaling decisions
	totalPendingWF := livePendingWF + histPendingWF
	totalPendingAct := livePendingAct + histPendingAct
	totalPollers := livePollers + histPollers

	// Use max backlog age (worst case)
	maxBacklogAge := liveBacklogAge
	if histBacklogAge > maxBacklogAge {
		maxBacklogAge = histBacklogAge
	}

	// Log both queues separately for visibility
	a.Logger.Debug("queue stats fetched",
		zap.String("chain_id", chainID),
		zap.String("live_queue", liveQueue),
		zap.Int64("live_queue_wf", livePendingWF),
		zap.Int64("live_queue_act", livePendingAct),
		zap.Int("live_pollers", livePollers),
		zap.Float64("live_backlog_age_seconds", liveBacklogAge),
		zap.String("historical_queue", historicalQueue),
		zap.Int64("historical_queue_wf", histPendingWF),
		zap.Int64("historical_queue_act", histPendingAct),
		zap.Int("historical_pollers", histPollers),
		zap.Float64("historical_backlog_age_seconds", histBacklogAge),
		zap.Int64("total_pending_wf", totalPendingWF),
		zap.Int64("total_pending_act", totalPendingAct),
		zap.Int("total_pollers", totalPollers),
		zap.Float64("max_backlog_age_seconds", maxBacklogAge),
	)

	// Use aggregated stats (backward compatible with scaling logic)
	stats := QueueStats{
		PendingWorkflowTasks: totalPendingWF,
		PendingActivityTasks: totalPendingAct,
		PollerCount:          totalPollers,
		BacklogAgeSeconds:    maxBacklogAge,
	}

	// Store individual queue stats for visibility
	liveStats := QueueStats{
		PendingWorkflowTasks: livePendingWF,
		PendingActivityTasks: livePendingAct,
		PollerCount:          livePollers,
		BacklogAgeSeconds:    liveBacklogAge,
	}

	historicalStats := QueueStats{
		PendingWorkflowTasks: histPendingWF,
		PendingActivityTasks: histPendingAct,
		PollerCount:          histPollers,
		BacklogAgeSeconds:    histBacklogAge,
	}

	a.queueCache.Store(chainID, cachedQueueStats{
		stats:           stats,
		liveStats:       liveStats,
		historicalStats: historicalStats,
		fetched:         time.Now(),
	})
	return stats, nil
}

func (a *App) desiredReplicas(ch *Chain, stats QueueStats) int32 {
	minReplicas := ch.MinReplicas
	maxReplicas := ch.MaxReplicas
	if maxReplicas < minReplicas {
		maxReplicas = minReplicas
	}

	if minReplicas <= 0 {
		minReplicas = 1
	}
	if maxReplicas <= 0 {
		maxReplicas = minReplicas
	}

	prevDecision := ch.Hysteresis.LastDecisionReplicas
	if maxReplicas == minReplicas {
		now := time.Now()
		if prevDecision == 0 {
			ch.Hysteresis.LastDecisionReplicas = minReplicas
			ch.Hysteresis.LastChangeTime = now
		}
		a.Logger.Debug("replica decision",
			zap.String("chain_id", ch.ID),
			zap.Int32("min_replicas", minReplicas),
			zap.Int32("max_replicas", maxReplicas),
			zap.Int32("previous_replicas", prevDecision),
			zap.Int32("calculated_replicas", minReplicas),
			zap.Int32("desired_replicas", minReplicas),
			zap.Int64("pending_workflow_tasks", stats.PendingWorkflowTasks),
			zap.Int64("pending_activity_tasks", stats.PendingActivityTasks),
			zap.Int64("queue_backlog_total", stats.PendingWorkflowTasks+stats.PendingActivityTasks),
			zap.Float64("backlog_ratio", 0),
			zap.Bool("cooldown_active", false),
		)
		return minReplicas
	}

	backlog := stats.PendingWorkflowTasks + stats.PendingActivityTasks
	var desired, calculated int32
	ratio := 0.0
	switch {
	case backlog >= backlogHighWatermark:
		desired = maxReplicas
	case backlog <= backlogLowWatermark:
		desired = minReplicas
	default:
		span := float64(maxReplicas - minReplicas)
		ratio = float64(backlog-backlogLowWatermark) / float64(backlogHighWatermark-backlogLowWatermark)
		if ratio < 0 {
			ratio = 0
		} else if ratio > 1 {
			ratio = 1
		}
		desired = minReplicas + int32(math.Ceil(ratio*span))
	}

	calculated = desired

	if desired < minReplicas {
		desired = minReplicas
	}
	if desired > maxReplicas {
		desired = maxReplicas
	}

	now := time.Now()
	cooldownActive := false
	if prevDecision == 0 {
		ch.Hysteresis.LastDecisionReplicas = desired
		ch.Hysteresis.LastChangeTime = now
	} else if desired != ch.Hysteresis.LastDecisionReplicas {
		if now.Sub(ch.Hysteresis.LastChangeTime) < scaleCooldown {
			desired = ch.Hysteresis.LastDecisionReplicas
			cooldownActive = true
		} else {
			ch.Hysteresis.LastDecisionReplicas = desired
			ch.Hysteresis.LastChangeTime = now
		}
	}

	a.Logger.Debug("replica decision",
		zap.String("chain_id", ch.ID),
		zap.Int32("min_replicas", minReplicas),
		zap.Int32("max_replicas", maxReplicas),
		zap.Int32("previous_replicas", prevDecision),
		zap.Int32("calculated_replicas", calculated),
		zap.Int32("desired_replicas", desired),
		zap.Int64("pending_workflow_tasks", stats.PendingWorkflowTasks),
		zap.Int64("pending_activity_tasks", stats.PendingActivityTasks),
		zap.Int64("queue_backlog_total", backlog),
		zap.Float64("backlog_ratio", ratio),
		zap.Bool("cooldown_active", cooldownActive),
	)

	return desired
}

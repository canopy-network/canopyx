package types

import (
	"context"
	"net/http"
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/puzpuzpuz/xsync/v4"
	"go.uber.org/zap"
)

type App struct {
	IndexerDB *db.AdminDB
	ReportDB  *db.ReportsDB
	ChainsDB  *xsync.Map[string, db.ChainStore]
	// Zap Logger
	Logger *zap.Logger
	// Server represents the HTTP server instance used to handle incoming client requests and manage HTTP routes.
	Server *http.Server
}

// LoadChainStore attempts to load a chain store, and if not found, refreshes the chains map
// from the database and tries again before failing.
func (a *App) LoadChainStore(ctx context.Context, chainID string) (db.ChainStore, bool) {
	// First attempt: check if already loaded
	store, ok := a.ChainsDB.Load(chainID)
	if ok {
		return store, true
	}

	// Chain not found - refresh from database in case it was recently added
	a.Logger.Debug("Chain not found in cache, refreshing from database", zap.String("chainID", chainID))

	freshChainsDB, err := a.IndexerDB.EnsureChainsDbs(ctx)
	if err != nil {
		a.Logger.Error("Failed to refresh chains database", zap.Error(err))
		return nil, false
	}

	// Replace the entire map with the fresh one
	a.ChainsDB = freshChainsDB

	// Second attempt: check if chain exists now
	store, ok = a.ChainsDB.Load(chainID)
	return store, ok
}

// Start starts the application.
func (a *App) Start(ctx context.Context) {
	go func() { _ = a.Server.ListenAndServe() }()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := a.ReportDB.Close()
	if err != nil {
		a.Logger.Error("Failed to close database connection", zap.Error(err))
	}

	err = a.IndexerDB.Close()
	if err != nil {
		a.Logger.Error("Failed to close database connection", zap.Error(err))
	}

	_ = a.Server.Shutdown(shutdownCtx)
	time.Sleep(200 * time.Millisecond)
	a.Logger.Info("さようなら!")
}

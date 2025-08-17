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
	ChainsDB  *xsync.Map[string, *db.ChainDB]
	// Zap Logger
	Logger *zap.Logger
	// Server represents the HTTP server instance used to handle incoming client requests and manage HTTP routes.
	Server *http.Server
}

// Start starts the application.
func (a *App) Start(ctx context.Context) {
	go func() { _ = a.Server.ListenAndServe() }()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := a.ReportDB.Db.Close()
	if err != nil {
		a.Logger.Error("Failed to close database connection", zap.Error(err))
	}

	err = a.IndexerDB.Db.Close()
	if err != nil {
		a.Logger.Error("Failed to close database connection", zap.Error(err))
	}

	_ = a.Server.Shutdown(shutdownCtx)
	time.Sleep(200 * time.Millisecond)
	a.Logger.Info("さようなら!")
}

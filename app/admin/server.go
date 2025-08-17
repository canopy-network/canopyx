package admin

import (
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/canopy-network/canopyx/app/admin/controller"
	"github.com/canopy-network/canopyx/app/admin/types"
	"github.com/canopy-network/canopyx/pkg/utils"
)

// NewServer creates and returns a new Server instance with the given http.Server and zap.Logger.
func NewServer(app *types.App) error {
	ctler := controller.NewController(app)
	router, err := ctler.NewRouter()
	if err != nil {
		return err
	}

	// use <ip>:<port> to bind to a specific interface or :<port> to bind to all interfaces
	addr := utils.Env("ADDR", ":3000")

	app.Server = &http.Server{
		Addr:              addr,
		Handler:           controller.WithCORS(router),
		ReadHeaderTimeout: 10 * time.Second,
	}
	app.Logger.Info("Starting server", zap.String("addr", addr))

	return nil
}

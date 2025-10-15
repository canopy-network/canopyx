package query

import (
    "go.uber.org/zap"
    "net/http"

    "github.com/canopy-network/canopyx/app/query/controller"
    "github.com/canopy-network/canopyx/app/query/types"
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
    addr := utils.Env("ADDR", ":3001")

    app.Server = &http.Server{Addr: addr, Handler: controller.WithCORS(router)}
    app.Logger.Info("Starting server", zap.String("addr", addr))

    return nil
}

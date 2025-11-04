package main

import (
	"context"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/canopy-network/canopyx/app/admin"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	defer cancel()

	app := admin.Initialize(ctx)

	// TODO: handle stop signals
	serverErr := admin.NewServer(app)
	if serverErr != nil {
		app.Logger.Fatal("Unable to initialize server", zap.Error(serverErr))
	}

	app.Start(ctx)
}

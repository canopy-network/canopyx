package main

import (
	"context"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/canopy-network/canopyx/app/query"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	defer cancel()

	app := query.Initialize(ctx)

	serverErr := query.NewServer(app)
	if serverErr != nil {
		app.Logger.Fatal("Unable to initialize server", zap.Error(serverErr))
	}

	app.Start(ctx)
}

package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/canopy-network/canopyx/app/controller"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Provider: start with a fake logger (builds with no extra deps).
	// Swap to a real k8s provider later: provider := controller.NewK8sProvider(...)
	provider := controller.NewFakeProvider()

	app, err := controller.Initialize(ctx, provider)
	if err != nil {
		panic(err)
	}

	// Immediate pass before cron
	app.ReconcileOnce(ctx)

	// Start cron scheduler
	app.StartCron()

	// Setup server
	app.SetupServer()

	// Start server
	app.Start(ctx)
}

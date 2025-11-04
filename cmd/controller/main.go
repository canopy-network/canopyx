package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/canopy-network/canopyx/pkg/logging"
	"github.com/canopy-network/canopyx/pkg/utils"
	"go.uber.org/zap"

	"github.com/canopy-network/canopyx/app/controller"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger, err := logging.New()
	if err != nil {
		panic(err)
	}

	// Provider: start with a fake logger (builds with no extra deps).
	// Swap to a real k8s provider later: provider := controller.NewK8sProvider(...)
	providerSelected := utils.Env("PROVIDER", "fake")

	var provider controller.Provider
	switch providerSelected {
	case "k8s":
		logger.Info("using k8s provider")
		p, pErr := controller.NewK8sProviderFromEnv(logger)
		if pErr != nil {
			logger.Fatal("Unable to initialize k8s provider", zap.Error(pErr))
		}
		provider = p
	case "fake":
		logger.Info("using fake provider")
		provider = controller.NewFakeProvider(logger)
	default:
		logger.Info("using fake provider")
		provider = controller.NewFakeProvider(logger)
	}

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

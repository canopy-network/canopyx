package main

import (
	"context"
	"os/signal"
	"syscall"

	workerreports "github.com/canopy-network/canopyx/app/reporter"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	defer cancel()

	app := workerreports.Initialize(ctx)

	app.Start(ctx)
}

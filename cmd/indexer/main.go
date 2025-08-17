package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/canopy-network/canopyx/app/indexer"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	defer cancel()

	app := indexer.Initialize(ctx)

	app.Start(ctx)
}

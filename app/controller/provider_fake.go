package controller

import (
	"context"
	"go.uber.org/zap"
	"log"
)

// FakeProvider is a fake provider.
type FakeProvider struct {
	Logger *zap.Logger
}

// NewFakeProvider creates a new fake provider.
func NewFakeProvider(logger *zap.Logger) *FakeProvider { return &FakeProvider{Logger: logger} }

// EnsureChain is a no-op.
func (p *FakeProvider) EnsureChain(_ context.Context, c *Chain) error {
	log.Printf("[controller/Provider=fake] ensure chain=%s paused=%v deleted=%v", c.ID, c.Paused, c.Deleted)
	return nil
}

// PauseChain is a no-op.
func (p *FakeProvider) PauseChain(_ context.Context, chainID string) error {
	log.Printf("[controller/Provider=fake] pause chain=%s", chainID)
	return nil
}

// DeleteChain is a no-op.
func (p *FakeProvider) DeleteChain(_ context.Context, chainID string) error {
	log.Printf("[controller/Provider=fake] delete chain=%s", chainID)
	return nil
}

// Close is a no-op.
func (p *FakeProvider) Close() error { return nil }

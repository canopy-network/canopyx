package controller

import (
	"context"
	"log"

	"go.uber.org/zap"
)

// FakeProvider is a fake provider.
type FakeProvider struct {
	Logger *zap.Logger
}

// NewFakeProvider creates a new fake provider.
func NewFakeProvider(logger *zap.Logger) *FakeProvider { return &FakeProvider{Logger: logger} }

// EnsureChain is a no-op.
func (p *FakeProvider) EnsureChain(_ context.Context, c *Chain) error {
	log.Printf("[controller/Provider=fake] ensure chain=%s paused=%v deleted=%v replicas=%d", c.ID, c.Paused, c.Deleted, c.Replicas)
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

// GetDeploymentHealth always returns "healthy" for testing purposes.
func (p *FakeProvider) GetDeploymentHealth(_ context.Context, chainID string) (status, message string, err error) {
	log.Printf("[controller/Provider=fake] get deployment health chain=%s", chainID)
	return "healthy", "fake provider deployment always healthy", nil
}

// EnsureReindexWorker is a no-op.
func (p *FakeProvider) EnsureReindexWorker(_ context.Context, c *Chain) error {
	log.Printf("[controller/Provider=fake] ensure reindex worker chain=%s replicas=%d", c.ID, c.Replicas)
	return nil
}

// DeleteReindexWorker is a no-op.
func (p *FakeProvider) DeleteReindexWorker(_ context.Context, chainID string) error {
	log.Printf("[controller/Provider=fake] delete reindex worker chain=%s", chainID)
	return nil
}

// Close is a no-op.
func (p *FakeProvider) Close() error { return nil }

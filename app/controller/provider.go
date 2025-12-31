package controller

import "context"

// Provider abstracts the platform where we place per-chain workers.
// Implementations may talk to Kubernetes, Nomad, etc.
type Provider interface {
	// EnsureChain makes sure a per-chain worker deployment exists (and is Running if not paused).
	EnsureChain(ctx context.Context, c *Chain) error
	// PauseChain puts the deployment into a paused state (e.g., scale to zero).
	PauseChain(ctx context.Context, chainID string) error
	// DeleteChain removes the deployment and related autoscalers/resources.
	DeleteChain(ctx context.Context, chainID string) error
	// GetDeploymentHealth checks the health status of a chain's deployment.
	// Returns status (healthy/degraded/failed/unknown), a human-readable message, and any error.
	GetDeploymentHealth(ctx context.Context, chainID string) (status, message string, err error)

	// EnsureReindexWorker makes sure a per-chain reindex worker deployment exists with specified replicas.
	EnsureReindexWorker(ctx context.Context, c *Chain) error
	// DeleteReindexWorker removes the reindex worker deployment.
	DeleteReindexWorker(ctx context.Context, chainID string) error

	// Close releases any Provider resources.
	Close() error
}

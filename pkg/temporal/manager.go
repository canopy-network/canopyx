package temporal

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/canopy-network/canopyx/pkg/utils"
	batchpb "go.temporal.io/api/batch/v1"
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"
)

// ClientManager manages multiple Temporal clients for admin operations.
// It provides lazy initialization and caching of clients for both the admin
// namespace (canopyx) and chain-specific namespaces (chain_<id>).
type ClientManager struct {
	mu           sync.RWMutex
	hostPort     string
	config       NamespaceConfig
	logger       *zap.Logger
	adminClient  *AdminClient
	chainClients map[uint64]*ChainClient
}

// NewClientManager creates a manager for multi-namespace admin operations.
func NewClientManager(ctx context.Context, logger *zap.Logger) (*ClientManager, error) {
	host := utils.Env("TEMPORAL_HOSTPORT", "localhost:7233")

	return &ClientManager{
		hostPort:     host,
		config:       DefaultNamespaceConfig(),
		logger:       logger,
		chainClients: make(map[uint64]*ChainClient),
	}, nil
}

// GetAdminClient returns the admin namespace client with lazy initialization.
func (m *ClientManager) GetAdminClient(ctx context.Context) (*AdminClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.adminClient != nil {
		return m.adminClient, nil
	}

	c, err := NewAdminClient(ctx, m.logger)
	if err != nil {
		return nil, err
	}

	m.adminClient = c
	return m.adminClient, nil
}

// GetChainClient returns a chain-specific client for a specific namespace.
// The namespace should be in format "{chain_id}-{namespace_uid}" (e.g., "5-a1b2c3").
// Caches clients by chainID - if the cached client has a different namespace, it's replaced.
func (m *ClientManager) GetChainClient(ctx context.Context, chainID uint64, namespace string) (*ChainClient, error) {
	// Read lock first for a fast path
	m.mu.RLock()
	if c, ok := m.chainClients[chainID]; ok {
		if c.Namespace == namespace {
			m.mu.RUnlock()
			return c, nil
		}
		// Namespace changed (chain was recreated), need to replace client
		m.mu.RUnlock()
	} else {
		m.mu.RUnlock()
	}

	// Write lock for creation or replacement
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if c, ok := m.chainClients[chainID]; ok {
		if c.Namespace == namespace {
			return c, nil
		}
		// Close old client before replacing
		c.Close()
		delete(m.chainClients, chainID)
	}

	c, err := NewChainClientWithNamespace(ctx, m.logger, chainID, namespace)
	if err != nil {
		return nil, err
	}

	m.chainClients[chainID] = c
	return m.chainClients[chainID], nil
}

// EnsureNamespace creates a chain namespace with a specific namespace name.
// Format should be "{chain_id}-{namespace_uid}" (e.g., "5-a1b2c3").
// After creation, it waits for the namespace to be fully propagated to all Temporal services.
func (m *ClientManager) EnsureNamespace(ctx context.Context, chainID uint64, namespace string, retention time.Duration) error {
	nsClient, err := client.NewNamespaceClient(client.Options{
		HostPort: m.hostPort,
		Logger:   NewZapAdapter(m.logger),
	})
	if err != nil {
		return fmt.Errorf("failed to create namespace client: %w", err)
	}
	defer nsClient.Close()

	// Try to describe the namespace first
	_, err = nsClient.Describe(ctx, namespace)

	// If namespace exists, we're done
	if err == nil {
		m.logger.Debug("Chain namespace already exists",
			zap.Uint64("chainID", chainID),
			zap.String("namespace", namespace))
		return nil
	}

	// Check if the error is "namespace not found"
	var notFound *serviceerror.NamespaceNotFound
	if !errors.As(err, &notFound) {
		return fmt.Errorf("failed to describe namespace: %w", err)
	}

	// Namespace doesn't exist, create it
	m.logger.Info("Creating chain namespace",
		zap.Uint64("chainID", chainID),
		zap.String("namespace", namespace))

	err = nsClient.Register(ctx, &workflowservice.RegisterNamespaceRequest{
		Namespace:                        namespace,
		WorkflowExecutionRetentionPeriod: durationpb.New(retention),
	})
	if err != nil {
		return fmt.Errorf("failed to register namespace: %w", err)
	}

	// Wait for namespace to be fully propagated to all Temporal services.
	// The namespace cache takes ~10-15 seconds to propagate after creation.
	// We use the "fake workflow" trick: poll until GetWorkflowExecutionHistory
	// returns INVALID_ARGUMENT (namespace ready) instead of NOT_FOUND (not ready).
	m.logger.Info("Waiting for namespace to be fully propagated",
		zap.Uint64("chainID", chainID),
		zap.String("namespace", namespace))

	if err := m.waitForNamespaceReady(ctx, namespace); err != nil {
		return fmt.Errorf("namespace propagation failed: %w", err)
	}

	m.logger.Info("Chain namespace created and ready",
		zap.Uint64("chainID", chainID),
		zap.String("namespace", namespace))

	return nil
}

// waitForNamespaceReady waits for a newly created namespace to be fully propagated
// to all Temporal services (history, matching, etc.).
// Uses the "fake workflow" trick: GetWorkflowExecutionHistory returns:
// - NOT_FOUND: namespace not yet propagated
// - INVALID_ARGUMENT: namespace is ready (workflow doesn't exist, but namespace does)
func (m *ClientManager) waitForNamespaceReady(ctx context.Context, namespace string) error {
	// Create a client for the new namespace
	c, err := client.Dial(client.Options{
		HostPort:  m.hostPort,
		Namespace: namespace,
	})
	if err != nil {
		return fmt.Errorf("failed to create client for namespace readiness check: %w", err)
	}
	defer c.Close()

	maxWait := 30 * time.Second
	pollInterval := 500 * time.Millisecond
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		// Try to describe a fake workflow - this will fail, but the error type tells us
		// whether the namespace is propagated or not
		_, err := c.DescribeWorkflowExecution(ctx, "namespace-readiness-check-fake-workflow", "")
		if err != nil {
			// NOT_FOUND with "namespace not found" means namespace not yet propagated
			var nsNotFound *serviceerror.NamespaceNotFound
			if errors.As(err, &nsNotFound) {
				m.logger.Debug("Namespace not yet propagated, waiting...",
					zap.String("namespace", namespace))
				time.Sleep(pollInterval)
				continue
			}

			// NOT_FOUND with "workflow not found" means namespace IS ready
			var notFound *serviceerror.NotFound
			if errors.As(err, &notFound) {
				m.logger.Debug("Namespace is ready (workflow not found as expected)",
					zap.String("namespace", namespace))
				return nil
			}

			// Any other error (like INVALID_ARGUMENT) also means namespace is ready
			m.logger.Debug("Namespace appears ready",
				zap.String("namespace", namespace),
				zap.String("error_type", fmt.Sprintf("%T", err)))
			return nil
		}

		// If no error (shouldn't happen with fake workflow), namespace is ready
		return nil
	}

	return fmt.Errorf("namespace propagation timed out after %v", maxWait)
}

// PauseChainSchedules pauses all schedules in a chain namespace.
// This is called during soft-delete of a chain.
func (m *ClientManager) PauseChainSchedules(ctx context.Context, chainID uint64, namespace string) error {
	chainClient, err := m.GetChainClient(ctx, chainID, namespace)
	if err != nil {
		return fmt.Errorf("failed to get chain client: %w", err)
	}

	scheduleIDs := []string{
		chainClient.HeadScheduleID,
		chainClient.GapScanScheduleID,
		chainClient.PollSnapshotScheduleID,
		chainClient.ProposalSnapshotScheduleID,
		chainClient.LPSnapshotScheduleID,
	}

	var errs []error
	for _, id := range scheduleIDs {
		handle := chainClient.TSClient.GetHandle(ctx, id)
		err := handle.Pause(ctx, client.SchedulePauseOptions{
			Note: fmt.Sprintf("Chain soft-deleted at %s", time.Now().Format(time.RFC3339)),
		})
		if err != nil {
			// Log but continue - schedule might not exist
			var notFound *serviceerror.NotFound
			if !errors.As(err, &notFound) {
				errs = append(errs, fmt.Errorf("failed to pause schedule %s: %w", id, err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors pausing schedules: %v", errs)
	}

	m.logger.Info("Paused chain schedules",
		zap.Uint64("chainID", chainID),
		zap.Int("schedulesCount", len(scheduleIDs)))

	return nil
}

// UnpauseChainSchedules unpauses all schedules in a chain namespace.
// This is called during restore of a soft-deleted chain.
func (m *ClientManager) UnpauseChainSchedules(ctx context.Context, chainID uint64, namespace string) error {
	chainClient, err := m.GetChainClient(ctx, chainID, namespace)
	if err != nil {
		return fmt.Errorf("failed to get chain client: %w", err)
	}

	scheduleIDs := []string{
		chainClient.HeadScheduleID,
		chainClient.GapScanScheduleID,
		chainClient.PollSnapshotScheduleID,
		chainClient.ProposalSnapshotScheduleID,
		chainClient.LPSnapshotScheduleID,
	}

	var errs []error
	for _, id := range scheduleIDs {
		handle := chainClient.TSClient.GetHandle(ctx, id)
		err := handle.Unpause(ctx, client.ScheduleUnpauseOptions{
			Note: fmt.Sprintf("Chain restored at %s", time.Now().Format(time.RFC3339)),
		})
		if err != nil {
			var notFound *serviceerror.NotFound
			if !errors.As(err, &notFound) {
				errs = append(errs, fmt.Errorf("failed to unpause schedule %s: %w", id, err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors unpausing schedules: %v", errs)
	}

	m.logger.Info("Unpaused chain schedules",
		zap.Uint64("chainID", chainID),
		zap.Int("schedulesCount", len(scheduleIDs)))

	return nil
}

// TerminateRunningWorkflows terminates all running workflows in a chain namespace using batch operation.
// This is called during soft-delete to stop any in-progress work.
func (m *ClientManager) TerminateRunningWorkflows(ctx context.Context, chainID uint64, namespace string, reason string) error {
	chainClient, err := m.GetChainClient(ctx, chainID, namespace)
	if err != nil {
		var notFound *serviceerror.NamespaceNotFound
		if errors.As(err, &notFound) {
			m.logger.Debug("Chain namespace doesn't exist, no workflows to terminate",
				zap.Uint64("chainID", chainID))
			return nil
		}
		return fmt.Errorf("failed to get chain client: %w", err)
	}

	// Use batch terminate for all running workflows
	jobID := fmt.Sprintf("terminate-chain-%d-%d", chainID, time.Now().Unix())
	_, err = chainClient.TClient.WorkflowService().StartBatchOperation(ctx, &workflowservice.StartBatchOperationRequest{
		Namespace:       chainClient.Namespace,
		JobId:           jobID,
		VisibilityQuery: "ExecutionStatus = 'Running'",
		Reason:          reason,
		Operation: &workflowservice.StartBatchOperationRequest_TerminationOperation{
			TerminationOperation: &batchpb.BatchOperationTermination{
				Identity: "chain-delete-manager",
			},
		},
	})
	if err != nil {
		// If no workflows match, that's fine
		m.logger.Warn("Batch terminate may have failed (possibly no running workflows)",
			zap.Uint64("chainID", chainID),
			zap.Error(err))
		return nil
	}

	m.logger.Info("Batch terminate started for chain workflows",
		zap.Uint64("chainID", chainID),
		zap.String("jobID", jobID),
		zap.String("reason", reason))

	return nil
}

// DeleteChainSchedules deletes all schedules in a chain namespace.
// This is called before deleting the namespace itself.
func (m *ClientManager) DeleteChainSchedules(ctx context.Context, chainID uint64, namespace string) error {
	chainClient, err := m.GetChainClient(ctx, chainID, namespace)
	if err != nil {
		return fmt.Errorf("failed to get chain client: %w", err)
	}

	scheduleIDs := []string{
		chainClient.HeadScheduleID,
		chainClient.GapScanScheduleID,
		chainClient.PollSnapshotScheduleID,
		chainClient.ProposalSnapshotScheduleID,
		chainClient.LPSnapshotScheduleID,
	}

	var errs []error
	for _, id := range scheduleIDs {
		handle := chainClient.TSClient.GetHandle(ctx, id)
		err := handle.Delete(ctx)
		if err != nil {
			var notFound *serviceerror.NotFound
			if !errors.As(err, &notFound) {
				errs = append(errs, fmt.Errorf("failed to delete schedule %s: %w", id, err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors deleting schedules: %v", errs)
	}

	m.logger.Info("Deleted chain schedules",
		zap.Uint64("chainID", chainID),
		zap.Int("schedulesCount", len(scheduleIDs)))

	return nil
}

// DeleteNamespace deletes a chain namespace using the OperatorService API.
// This is called during hard-delete of a chain (permanent deletion).
// The namespace deletion will also delete all workflows and schedules within it.
// The deletion is asynchronous - Temporal runs a system workflow to clean up all resources.
//
// Returns the renamed namespace (e.g., "5-a1b2c3-deleted-xyz789") that must be monitored
// for cleanup completion before the namespace can be safely recreated.
// Returns empty string if namespace didn't exist.
func (m *ClientManager) DeleteNamespace(ctx context.Context, chainID uint64, namespace string) (string, error) {
	// First, close and remove the cached client
	m.mu.Lock()
	if c, ok := m.chainClients[chainID]; ok {
		c.Close()
		delete(m.chainClients, chainID)
	}
	m.mu.Unlock()

	// Create a workflow client for OperatorService access
	c, err := client.Dial(client.Options{
		HostPort: m.hostPort,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	// Use OperatorService to fully delete the namespace
	// This triggers an async workflow that:
	// 1. Terminates all running workflows
	// 2. Deletes all workflow executions from persistence
	// 3. Removes all data from visibility store
	// 4. Finally removes the namespace entry itself
	resp, err := c.OperatorService().DeleteNamespace(ctx, &operatorservice.DeleteNamespaceRequest{
		Namespace:            namespace,
		NamespaceDeleteDelay: durationpb.New(0), // Delete immediately after cleanup
	})
	if err != nil {
		var notFound *serviceerror.NamespaceNotFound
		if errors.As(err, &notFound) {
			// Namespace doesn't exist, nothing to delete
			m.logger.Debug("Chain namespace doesn't exist, nothing to delete",
				zap.Uint64("chainID", chainID),
				zap.String("namespace", namespace))
			return "", nil
		}
		return "", fmt.Errorf("failed to delete namespace: %w", err)
	}

	// IMPORTANT: DeleteNamespace RENAMES the namespace (e.g., "chain_5" -> "chain_5-deleted-abc123")
	// and then runs async cleanup. The caller must wait for the RENAMED namespace to be fully
	// deleted before recreating the namespace with the same name.
	renamedNamespace := resp.DeletedNamespace

	m.logger.Info("Namespace deletion initiated",
		zap.Uint64("chainID", chainID),
		zap.String("originalNamespace", namespace),
		zap.String("renamedNamespace", renamedNamespace))

	return renamedNamespace, nil
}

// WaitForNamespaceCleanup waits for a renamed namespace to be fully deleted.
// This should be called after DeleteChainNamespace to ensure cleanup completes
// before recreating a namespace with the same name.
func (m *ClientManager) WaitForNamespaceCleanup(ctx context.Context, renamedNamespace string) error {
	if renamedNamespace == "" {
		return nil // Nothing to wait for
	}

	nsClient, err := client.NewNamespaceClient(client.Options{
		HostPort: m.hostPort,
		Logger:   NewZapAdapter(m.logger),
	})
	if err != nil {
		return fmt.Errorf("failed to create namespace client: %w", err)
	}
	defer nsClient.Close()

	maxWait := 5 * time.Minute // Cleanup can take a while with many workflows
	pollInterval := 3 * time.Second
	deadline := time.Now().Add(maxWait)

	m.logger.Info("Waiting for namespace cleanup to complete",
		zap.String("renamedNamespace", renamedNamespace))

	for time.Now().Before(deadline) {
		_, err := nsClient.Describe(ctx, renamedNamespace)
		if err != nil {
			var notFound *serviceerror.NamespaceNotFound
			if errors.As(err, &notFound) {
				// Renamed namespace is fully deleted - cleanup complete
				m.logger.Info("Namespace cleanup completed",
					zap.String("renamedNamespace", renamedNamespace))
				return nil
			}
			// Some other error - log and continue waiting
			m.logger.Debug("Error checking renamed namespace status",
				zap.String("renamedNamespace", renamedNamespace),
				zap.Error(err))
		} else {
			m.logger.Debug("Renamed namespace still exists, cleanup in progress",
				zap.String("renamedNamespace", renamedNamespace))
		}
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("namespace cleanup timed out after %v", maxWait)
}

// Close closes all managed clients.
func (m *ClientManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.adminClient != nil {
		m.adminClient.Close()
		m.adminClient = nil
	}

	for chainID, c := range m.chainClients {
		c.Close()
		delete(m.chainClients, chainID)
	}
}

// GetConfig returns the namespace configuration.
func (m *ClientManager) GetConfig() NamespaceConfig {
	return m.config
}

// GetHostPort returns the Temporal host port.
func (m *ClientManager) GetHostPort() string {
	return m.hostPort
}

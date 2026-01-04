package temporal

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/canopy-network/canopyx/pkg/utils"
	batchpb "go.temporal.io/api/batch/v1"
	enumspb "go.temporal.io/api/enums/v1"
	namespacepb "go.temporal.io/api/namespace/v1"
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

// GetChainClient returns a chain-specific client with lazy initialization and caching.
func (m *ClientManager) GetChainClient(ctx context.Context, chainID uint64) (*ChainClient, error) {
	// Read lock first for a fast path
	m.mu.RLock()
	if c, ok := m.chainClients[chainID]; ok {
		m.mu.RUnlock()
		return c, nil
	}
	m.mu.RUnlock()

	// Write lock for creation
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if c, ok := m.chainClients[chainID]; ok {
		return c, nil
	}

	c, err := NewChainClient(ctx, m.logger, chainID)
	if err != nil {
		return nil, err
	}

	m.chainClients[chainID] = c
	return m.chainClients[chainID], nil
}

// EnsureChainNamespace creates the chain namespace if it doesn't exist.
func (m *ClientManager) EnsureChainNamespace(ctx context.Context, chainID uint64, retention time.Duration) error {
	namespace := m.config.ChainNamespace(chainID)

	nsClient, err := client.NewNamespaceClient(client.Options{
		HostPort: m.hostPort,
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

	// Wait for namespace to be available
	time.Sleep(2 * time.Second)
	return m.EnsureChainNamespace(ctx, chainID, retention)
}

// PauseChainSchedules pauses all schedules in a chain namespace.
// This is called during soft-delete of a chain.
func (m *ClientManager) PauseChainSchedules(ctx context.Context, chainID uint64) error {
	chainClient, err := m.GetChainClient(ctx, chainID)
	if err != nil {
		return fmt.Errorf("failed to get chain client: %w", err)
	}

	scheduleIDs := []string{
		chainClient.HeadScheduleID,
		chainClient.GapScanScheduleID,
		chainClient.PollSnapshotScheduleID,
		chainClient.ProposalSnapshotScheduleID,
		chainClient.LPSnapshotScheduleID,
		chainClient.CleanupStagingScheduleID,
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
func (m *ClientManager) UnpauseChainSchedules(ctx context.Context, chainID uint64) error {
	chainClient, err := m.GetChainClient(ctx, chainID)
	if err != nil {
		return fmt.Errorf("failed to get chain client: %w", err)
	}

	scheduleIDs := []string{
		chainClient.HeadScheduleID,
		chainClient.GapScanScheduleID,
		chainClient.PollSnapshotScheduleID,
		chainClient.ProposalSnapshotScheduleID,
		chainClient.LPSnapshotScheduleID,
		chainClient.CleanupStagingScheduleID,
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
func (m *ClientManager) TerminateRunningWorkflows(ctx context.Context, chainID uint64, reason string) error {
	chainClient, err := m.GetChainClient(ctx, chainID)
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
func (m *ClientManager) DeleteChainSchedules(ctx context.Context, chainID uint64) error {
	chainClient, err := m.GetChainClient(ctx, chainID)
	if err != nil {
		return fmt.Errorf("failed to get chain client: %w", err)
	}

	scheduleIDs := []string{
		chainClient.HeadScheduleID,
		chainClient.GapScanScheduleID,
		chainClient.PollSnapshotScheduleID,
		chainClient.ProposalSnapshotScheduleID,
		chainClient.LPSnapshotScheduleID,
		chainClient.CleanupStagingScheduleID,
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

// DeleteChainNamespace deletes a chain namespace.
// This is called during hard-delete of a chain (permanent deletion).
// The namespace deletion will also delete all workflows and schedules within it.
func (m *ClientManager) DeleteChainNamespace(ctx context.Context, chainID uint64) error {
	namespace := m.config.ChainNamespace(chainID)

	// First, close and remove the cached client
	m.mu.Lock()
	if c, ok := m.chainClients[chainID]; ok {
		c.Close()
		delete(m.chainClients, chainID)
	}
	m.mu.Unlock()

	// Create a namespace client for deletion
	nsClient, err := client.NewNamespaceClient(client.Options{
		HostPort: m.hostPort,
	})
	if err != nil {
		return fmt.Errorf("failed to create namespace client: %w", err)
	}
	defer nsClient.Close()

	// Check if namespace exists
	_, err = nsClient.Describe(ctx, namespace)
	if err != nil {
		var notFound *serviceerror.NamespaceNotFound
		if errors.As(err, &notFound) {
			// Namespace doesn't exist, nothing to delete
			m.logger.Debug("Chain namespace doesn't exist, nothing to delete",
				zap.Uint64("chainID", chainID),
				zap.String("namespace", namespace))
			return nil
		}
		return fmt.Errorf("failed to describe namespace: %w", err)
	}

	// Update namespace state to DELETED
	err = nsClient.Update(ctx, &workflowservice.UpdateNamespaceRequest{
		Namespace: namespace,
		UpdateInfo: &namespacepb.UpdateNamespaceInfo{
			State: enumspb.NAMESPACE_STATE_DELETED,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to delete namespace: %w", err)
	}

	m.logger.Info("Deleted chain namespace",
		zap.Uint64("chainID", chainID),
		zap.String("namespace", namespace))

	return nil
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

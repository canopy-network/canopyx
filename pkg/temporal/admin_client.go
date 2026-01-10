package temporal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/pkg/retry"
	"github.com/canopy-network/canopyx/pkg/utils"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"
)

// AdminClient manages the admin namespace (canopyx) for cross-chain operations.
// This includes maintenance tasks, LP snapshots, and compaction schedules.
type AdminClient struct {
	TClient   client.Client
	TSClient  client.ScheduleClient
	Namespace string // "canopyx"
	HostPort  string
	Config    NamespaceConfig
	logger    *zap.Logger

	// Admin-specific queue
	MaintenanceQueue string // "maintenance"

	// Admin-specific schedule patterns
	GlobalCompactionScheduleID string // "global:compaction"
}

// NewAdminClient creates a client for the admin namespace (canopyx).
func NewAdminClient(ctx context.Context, logger *zap.Logger) (*AdminClient, error) {
	connCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	host := utils.Env("TEMPORAL_HOSTPORT", "localhost:7233")
	ns := utils.Env("TEMPORAL_ADMIN_NAMESPACE", DefaultAdminNamespace)

	loggerWrapper := NewZapAdapter(logger)
	retryConfig := retry.DefaultConfig()

	var tClient client.Client

	logger.Info("Connecting to Temporal admin namespace",
		zap.String("host", host),
		zap.String("namespace", ns))

	err := retry.WithBackoff(connCtx, retryConfig, logger, "temporal_admin_connection", func() error {
		var err error
		tClient, err = Dial(connCtx, host, ns, loggerWrapper)
		if err != nil {
			return err
		}

		if _, err = tClient.CheckHealth(connCtx, nil); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &AdminClient{
		TClient:   tClient,
		TSClient:  tClient.ScheduleClient(),
		Namespace: ns,
		HostPort:  host,
		Config:    DefaultNamespaceConfig(),
		logger:    logger,
		// Use constants from types.go
		MaintenanceQueue:           QueueMaintenance,
		GlobalCompactionScheduleID: ScheduleGlobalCompaction,
	}, nil
}

// EnsureNamespace ensures the admin Temporal namespace exists, creating it if necessary.
func (c *AdminClient) EnsureNamespace(ctx context.Context, retention time.Duration) error {
	nsClient, err := client.NewNamespaceClient(client.Options{
		HostPort: c.HostPort,
		Logger:   NewZapAdapter(c.logger),
	})
	if err != nil {
		return fmt.Errorf("failed to create namespace client: %w", err)
	}
	defer nsClient.Close()

	// Try to describe the namespace first
	_, err = nsClient.Describe(ctx, c.Namespace)

	// If namespace exists, we're done
	if err == nil {
		return nil
	}

	// Check if the error is "namespace not found"
	var notFound *serviceerror.NamespaceNotFound
	if !errors.As(err, &notFound) {
		return fmt.Errorf("failed to describe namespace: %w", err)
	}

	// Namespace doesn't exist, create it
	err = nsClient.Register(ctx, &workflowservice.RegisterNamespaceRequest{
		Namespace:                        c.Namespace,
		WorkflowExecutionRetentionPeriod: durationpb.New(retention),
	})
	if err != nil {
		return fmt.Errorf("failed to register namespace: %w", err)
	}

	// Wait for namespace to be available
	time.Sleep(2 * time.Second)
	return c.EnsureNamespace(ctx, retention)
}

// GetQueueStats fetches queue statistics using the DescribeTaskQueueEnhanced API.
func (c *AdminClient) GetQueueStats(ctx context.Context, queueName string) (pendingWorkflowTasks int64, pendingActivityTasks int64, pollerCount int, backlogAgeSeconds float64, err error) {
	desc, err := c.TClient.DescribeTaskQueueEnhanced(ctx, client.DescribeTaskQueueEnhancedOptions{
		TaskQueue: queueName,
		TaskQueueTypes: []client.TaskQueueType{
			client.TaskQueueTypeWorkflow,
			client.TaskQueueTypeActivity,
		},
		ReportPollers: true,
		ReportStats:   true,
	})
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("describe task queue enhanced failed: %w", err)
	}

	//nolint:staticcheck // TODO: migrate to VersioningInfo API in future
	for _, versionInfo := range desc.VersionsInfo {
		if wfInfo, ok := versionInfo.TypesInfo[client.TaskQueueTypeWorkflow]; ok {
			pollerCount += len(wfInfo.Pollers)
			if wfInfo.Stats != nil {
				pendingWorkflowTasks += wfInfo.Stats.ApproximateBacklogCount
				if wfInfo.Stats.ApproximateBacklogAge.Seconds() > backlogAgeSeconds {
					backlogAgeSeconds = wfInfo.Stats.ApproximateBacklogAge.Seconds()
				}
			}
		}

		if actInfo, ok := versionInfo.TypesInfo[client.TaskQueueTypeActivity]; ok {
			if actInfo.Stats != nil {
				pendingActivityTasks += actInfo.Stats.ApproximateBacklogCount
				if actInfo.Stats.ApproximateBacklogAge.Seconds() > backlogAgeSeconds {
					backlogAgeSeconds = actInfo.Stats.ApproximateBacklogAge.Seconds()
				}
			}
		}
	}

	return pendingWorkflowTasks, pendingActivityTasks, pollerCount, backlogAgeSeconds, nil
}

// GetDeleteChainWorkflowID returns the workflow ID for deleting a chain.
func (c *AdminClient) GetDeleteChainWorkflowID(chainID uint64) string {
	return fmt.Sprintf(WorkflowIDDeleteChain, chainID)
}

// StartDeleteChainWorkflow starts the hard delete workflow for a chain.
// renamedNamespace is the Temporal namespace after deletion (for cleanup monitoring).
// Returns the workflow ID and run ID for tracking.
func (c *AdminClient) StartDeleteChainWorkflow(ctx context.Context, chainID uint64, renamedNamespace string) (workflowID string, runID string, err error) {
	workflowID = c.GetDeleteChainWorkflowID(chainID)

	run, err := c.TClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: c.MaintenanceQueue,
	}, "DeleteChainWorkflow", DeleteChainWorkflowInput{
		ChainID:          chainID,
		RenamedNamespace: renamedNamespace,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to start delete chain workflow: %w", err)
	}

	return run.GetID(), run.GetRunID(), nil
}

// DeleteChainWorkflowInput is the input for the DeleteChainWorkflow.
type DeleteChainWorkflowInput struct {
	ChainID          uint64
	RenamedNamespace string
}

// Close closes the underlying Temporal client connection.
func (c *AdminClient) Close() {
	if c.TClient != nil {
		c.TClient.Close()
	}
}

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

// ChainClient manages a single chain namespace with simplified queue and schedule names.
// Each chain has its own namespace (e.g., "chain_1", "chain_21") which provides isolation.
type ChainClient struct {
	TClient   client.Client
	TSClient  client.ScheduleClient
	ChainID   uint64
	Namespace string // "chain_<id>"
	HostPort  string
	logger    *zap.Logger

	// Simplified queue names (no chain prefix needed - namespace provides isolation)
	LiveQueue        string // "live"
	HistoricalQueue  string // "historical"
	OpsQueue         string // "ops"
	ReindexQueue     string // "reindex"
	MaintenanceQueue string // "maintenance" (for cleanup workflows)

	// Simplified schedule IDs
	HeadScheduleID             string // "headscan"
	GapScanScheduleID          string // "gapscan"
	PollSnapshotScheduleID     string // "pollsnapshot"
	ProposalSnapshotScheduleID string // "proposalsnapshot"
	LPSnapshotScheduleID       string // "lpsnapshot"

	// Simplified workflow ID templates
	IndexBlockWorkflowIDPattern string // "index:%d"
	SchedulerWorkflowID         string // "scheduler"
}

// NewChainClient creates a client for a chain-specific namespace.
// The namespace is derived from the chainID (e.g., chainID=1 -> namespace "chain_1").
// This is consistent with how database names are derived from chain IDs.
func NewChainClient(ctx context.Context, logger *zap.Logger, chainID uint64) (*ChainClient, error) {
	connCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	host := utils.Env("TEMPORAL_HOSTPORT", "localhost:7233")
	config := DefaultNamespaceConfig()
	ns := config.ChainNamespace(chainID) // "chain_<id>"

	loggerWrapper := NewZapAdapter(logger)
	retryConfig := retry.DefaultConfig()

	var tClient client.Client

	logger.Info("Connecting to Temporal chain namespace",
		zap.String("host", host),
		zap.String("namespace", ns),
		zap.Uint64("chainID", chainID))

	err := retry.WithBackoff(connCtx, retryConfig, logger, "temporal_chain_connection", func() error {
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

	return &ChainClient{
		TClient:   tClient,
		TSClient:  tClient.ScheduleClient(),
		ChainID:   chainID,
		Namespace: ns,
		HostPort:  host,
		logger:    logger,
		// Simplified queue names (using constants from types.go)
		LiveQueue:        QueueLive,
		HistoricalQueue:  QueueHistorical,
		OpsQueue:         QueueOps,
		ReindexQueue:     QueueReindex,
		MaintenanceQueue: QueueMaintenance,
		// Simplified schedule IDs (using constants from types.go)
		HeadScheduleID:             ScheduleHeadScan,
		GapScanScheduleID:          ScheduleGapScan,
		PollSnapshotScheduleID:     SchedulePollSnapshot,
		ProposalSnapshotScheduleID: ScheduleProposalSnapshot,
		LPSnapshotScheduleID:       ScheduleLPSnapshot,
		// Simplified workflow ID templates (using constants from types.go)
		IndexBlockWorkflowIDPattern: WorkflowIDIndexBlock,
		SchedulerWorkflowID:         WorkflowIDScheduler,
	}, nil
}

// NewChainClientWithNamespace creates a client for a chain-specific namespace with explicit namespace.
// This is useful when the caller already knows the namespace name.
func NewChainClientWithNamespace(ctx context.Context, logger *zap.Logger, chainID uint64, namespace string) (*ChainClient, error) {
	connCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	host := utils.Env("TEMPORAL_HOSTPORT", "localhost:7233")
	loggerWrapper := NewZapAdapter(logger)
	retryConfig := retry.DefaultConfig()

	var tClient client.Client

	logger.Info("Connecting to Temporal chain namespace",
		zap.String("host", host),
		zap.String("namespace", namespace),
		zap.Uint64("chainID", chainID))

	err := retry.WithBackoff(connCtx, retryConfig, logger, "temporal_chain_connection", func() error {
		var err error
		tClient, err = Dial(connCtx, host, namespace, loggerWrapper)
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

	return &ChainClient{
		TClient:   tClient,
		TSClient:  tClient.ScheduleClient(),
		ChainID:   chainID,
		Namespace: namespace,
		HostPort:  host,
		logger:    logger,
		// Simplified queue names (using constants from types.go)
		LiveQueue:        QueueLive,
		HistoricalQueue:  QueueHistorical,
		OpsQueue:         QueueOps,
		ReindexQueue:     QueueReindex,
		MaintenanceQueue: QueueMaintenance,
		// Simplified schedule IDs (using constants from types.go)
		HeadScheduleID:             ScheduleHeadScan,
		GapScanScheduleID:          ScheduleGapScan,
		PollSnapshotScheduleID:     SchedulePollSnapshot,
		ProposalSnapshotScheduleID: ScheduleProposalSnapshot,
		LPSnapshotScheduleID:       ScheduleLPSnapshot,
		// Simplified workflow ID templates (using constants from types.go)
		IndexBlockWorkflowIDPattern: WorkflowIDIndexBlock,
		SchedulerWorkflowID:         WorkflowIDScheduler,
	}, nil
}

// GetIndexBlockWorkflowID returns the workflow ID for indexing a block.
// e.g., height=500 -> "index:500"
func (c *ChainClient) GetIndexBlockWorkflowID(height uint64) string {
	return fmt.Sprintf(c.IndexBlockWorkflowIDPattern, height)
}

// GetReindexBlockWorkflowID returns a deterministic workflow ID for reindex operations.
// Format: "reindex:{height}:{requestId}"
// Note: chainID is omitted since the namespace already provides chain isolation.
func (c *ChainClient) GetReindexBlockWorkflowID(height uint64, requestID string) string {
	return fmt.Sprintf(WorkflowIDReindexBlock, height, requestID)
}

// GapSchedulerWorkflowID returns a unique workflow ID for scheduling a specific gap.
// Format: "gap-scheduler:{from}-{to}"
func (c *ChainClient) GapSchedulerWorkflowID(from, to uint64) string {
	return fmt.Sprintf(WorkflowIDGapScheduler, from, to)
}

// EnsureNamespace ensures the Temporal namespace exists, creating it if necessary.
func (c *ChainClient) EnsureNamespace(ctx context.Context, retention time.Duration) error {
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
func (c *ChainClient) GetQueueStats(ctx context.Context, queueName string) (pendingWorkflowTasks int64, pendingActivityTasks int64, pollerCount int, backlogAgeSeconds float64, err error) {
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

// Close closes the underlying Temporal client connection.
func (c *ChainClient) Close() {
	if c.TClient != nil {
		c.TClient.Close()
	}
}

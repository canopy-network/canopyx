package temporal

import (
	"context"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/pkg/retry"
	"github.com/canopy-network/canopyx/pkg/utils"
	"go.uber.org/zap"

	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/log"
	"google.golang.org/protobuf/types/known/durationpb"
)

type Client struct {
	TClient   client.Client
	TSClient  client.ScheduleClient
	Namespace string

	// Task Queues
	IndexerLiveQueue       string // chain:<chainID>:index:live - per chain queue for live blocks (within threshold of head)
	IndexerHistoricalQueue string // chain:<chainID>:index:historical - per chain queue for historical blocks (beyond threshold)
	IndexerOpsQueue        string // chain:<chainID>:ops - per chain operations queue (headscan, gapscan, maintenance).
	AdminMaintenanceQueue  string // admin:maintenance - global admin maintenance queue (compaction, cleanup, monitoring)

	// Schedule IDs
	HeadScheduleID                 string
	GapScanScheduleID              string
	CrossChainCompactionScheduleID string

	// Workflow IDs
	IndexBlockWorkflowId     string
	SchedulerWorkflowID      string
	CleanupStagingWorkflowID string
}

type Health struct {
	ConnectionOK bool                      `json:"connection_ok"`
	ManagerQueue []*taskqueuepb.PollerInfo `json:"manager_queue"`
	ReportsQueue []*taskqueuepb.PollerInfo `json:"reports_queue"`
}

func NewClient(ctx context.Context, logger *zap.Logger) (*Client, error) {
	// Add timeout to context for initial connection
	connCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	host := utils.Env("TEMPORAL_HOSTPORT", "localhost:7233")
	ns := utils.Env("TEMPORAL_NAMESPACE", "canopyx")
	loggerWrapper := NewZapAdapter(logger)
	retryConfig := retry.DefaultConfig()

	var tClient client.Client

	logger.Info("Connecting to Temporal", zap.String("host", host), zap.String("namespace", ns))

	err := retry.WithBackoff(connCtx, retryConfig, logger, "temporal_connection", func() error {
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

	return &Client{
		TClient:   tClient,
		TSClient:  tClient.ScheduleClient(),
		Namespace: ns,
		// for now this is just hardcoded, could be configurable if we need it
		IndexerLiveQueue:       "chain:%d:index:live",
		IndexerHistoricalQueue: "chain:%d:index:historical",
		IndexerOpsQueue:        "chain:%d:ops",
		AdminMaintenanceQueue:  "admin:maintenance",
		// schedule IDs
		HeadScheduleID:                 "chain:%d:headscan",
		GapScanScheduleID:              "chain:%d:gapscan",
		CrossChainCompactionScheduleID: "crosschain:compaction",
		// workflow IDs
		IndexBlockWorkflowId:     "chain:%d:index:%d",
		SchedulerWorkflowID:      "chain:%d:scheduler",
		CleanupStagingWorkflowID: "chain:%d:cleanup:%d",
	}, nil
}

// Dial connects to Temporal using the provided hostPort and namespace.
func Dial(ctx context.Context, hostPort, namespace string, logger log.Logger) (client.Client, error) {
	return client.DialContext(
		ctx,
		client.Options{
			HostPort:  hostPort,
			Namespace: namespace,
			Logger:    logger,
		},
	)
}

// GetIndexerLiveQueue returns the live indexer queue for the given chain.
// This queue is for blocks within the LiveBlockThreshold of the chain head.
func (c *Client) GetIndexerLiveQueue(chainID uint64) string {
	return fmt.Sprintf(c.IndexerLiveQueue, chainID)
}

// GetIndexerHistoricalQueue returns the historical indexer queue for the given chain.
// This queue is for blocks beyond the LiveBlockThreshold from the chain head.
func (c *Client) GetIndexerHistoricalQueue(chainID uint64) string {
	return fmt.Sprintf(c.IndexerHistoricalQueue, chainID)
}

// GetIndexerOpsQueue returns the operations queue for the given chain.
func (c *Client) GetIndexerOpsQueue(chainID uint64) string {
	return fmt.Sprintf(c.IndexerOpsQueue, chainID)
}

// GetHeadScheduleID returns the schedule ID for the head scan for the given chain.
func (c *Client) GetHeadScheduleID(chainID uint64) string {
	return fmt.Sprintf(c.HeadScheduleID, chainID)
}

// GetGapScanScheduleID returns the schedule ID for the gap scan for the given chain.
func (c *Client) GetGapScanScheduleID(chainID uint64) string {
	return fmt.Sprintf(c.GapScanScheduleID, chainID)
}

// GetIndexBlockWorkflowId returns the workflow ID for the indexing block for the given chain and height.
func (c *Client) GetIndexBlockWorkflowId(chainID uint64, height uint64) string {
	return fmt.Sprintf(c.IndexBlockWorkflowId, chainID, height)
}

// GetIndexBlockWorkflowIdWithTime returns the workflow ID for the indexing block for the given chain and height with a timestamp.
func (c *Client) GetIndexBlockWorkflowIdWithTime(chainID uint64, height uint64) string {
	return fmt.Sprintf(c.IndexBlockWorkflowId+":%d", chainID, height, time.Now().UnixNano())
}

// GetSchedulerWorkflowID returns the deterministic workflow ID for the SchedulerWorkflow for a given chain.
func (c *Client) GetSchedulerWorkflowID(chainID uint64) string {
	return fmt.Sprintf(c.SchedulerWorkflowID, chainID)
}

// GetCleanupStagingWorkflowID returns the workflow ID for the cleanup staging workflow.
// This workflow is triggered after promotion to clean up staging tables asynchronously.
func (c *Client) GetCleanupStagingWorkflowID(chainID uint64, height uint64) string {
	return fmt.Sprintf(c.CleanupStagingWorkflowID, chainID, height)
}

// TwoSecondSpec returns a schedule spec for HeadScan workflow (5 seconds).
// With 20s block time = 4 checks per block, max 5s delay for new blocks.
func (c *Client) TwoSecondSpec() client.ScheduleSpec {
	return c.GetScheduleSpec(5 * time.Second)
}

// ThreeMinuteSpec returns a schedule spec for three minutes.
func (c *Client) ThreeMinuteSpec() client.ScheduleSpec {
	return c.GetScheduleSpec(3 * time.Minute)
}

// OneHourSpec returns a schedule spec for one hour.
func (c *Client) OneHourSpec() client.ScheduleSpec {
	return c.GetScheduleSpec(time.Hour)
}

// GetScheduleSpec returns a schedule spec for the given interval.
func (c *Client) GetScheduleSpec(interval time.Duration) client.ScheduleSpec {
	return client.ScheduleSpec{Intervals: []client.ScheduleIntervalSpec{{Every: interval}}}
}

// EnsureNamespace ensures the Temporal namespace exists, creating it if necessary.
// This is necessary because the Temporal Helm chart's defaultNamespaces setting doesn't
// actually create namespaces - they must be registered manually.
func (c *Client) EnsureNamespace(ctx context.Context, retention time.Duration) error {
	host := utils.Env("TEMPORAL_HOSTPORT", "localhost:7233")
	// Create a namespace client using the same connection options
	nsClient, err := client.NewNamespaceClient(client.Options{
		HostPort: host,
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

	// If error is not "namespace not found", return it
	// serviceerror.NamespaceNotFound is the expected error
	if err.Error() != fmt.Sprintf("Namespace %s is not found.", c.Namespace) {
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

	return nil
}

// GetQueueStats fetches queue statistics using the DescribeTaskQueueEnhanced API.
// This method aggregates stats across all Build IDs (versioned and unversioned workers).
// It returns pending workflow tasks, pending activity tasks, poller count, and the oldest backlog age.
//
// The Enhanced API provides statistics per Build ID, allowing us to track both
// versioned workers (with specific build IDs) and unversioned workers (empty string as build ID).
// This aggregation ensures we get the complete picture of queue depth regardless of worker versioning.
func (c *Client) GetQueueStats(ctx context.Context, queueName string) (pendingWorkflowTasks int64, pendingActivityTasks int64, pollerCount int, backlogAgeSeconds float64, err error) {
	// Use DescribeTaskQueueEnhanced API (replaces deprecated DescribeTaskQueue)
	// This returns stats per Build ID and per task queue type
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

	// Aggregate stats across all Build IDs (versioned + unversioned workers)
	// The VersionsInfo map has empty string "" as key for unversioned workers
	//nolint:staticcheck // TODO: migrate to VersioningInfo API in future
	for buildID, versionInfo := range desc.VersionsInfo {
		// Get workflow task stats
		if wfInfo, ok := versionInfo.TypesInfo[client.TaskQueueTypeWorkflow]; ok {
			pollerCount += len(wfInfo.Pollers)
			if wfInfo.Stats != nil {
				pendingWorkflowTasks += wfInfo.Stats.ApproximateBacklogCount
				// Use the oldest backlog age across all versions
				if wfInfo.Stats.ApproximateBacklogAge.Seconds() > backlogAgeSeconds {
					backlogAgeSeconds = wfInfo.Stats.ApproximateBacklogAge.Seconds()
				}
			}
		}

		// Get activity task stats
		if actInfo, ok := versionInfo.TypesInfo[client.TaskQueueTypeActivity]; ok {
			if actInfo.Stats != nil {
				pendingActivityTasks += actInfo.Stats.ApproximateBacklogCount
				// Update backlog age if the activity backlog is older
				if actInfo.Stats.ApproximateBacklogAge.Seconds() > backlogAgeSeconds {
					backlogAgeSeconds = actInfo.Stats.ApproximateBacklogAge.Seconds()
				}
			}
		}

		// Log debug information for transparency (optional - caller can add their own logging)
		_ = buildID // buildID is available here if needed for debugging
	}

	return pendingWorkflowTasks, pendingActivityTasks, pollerCount, backlogAgeSeconds, nil
}

// GetAdminMaintenanceQueue returns the admin maintenance task queue.
// This queue handles global admin maintenance operations like cross-chain table compaction.
func (c *Client) GetAdminMaintenanceQueue() string {
	return c.AdminMaintenanceQueue
}

// GetCrossChainCompactionScheduleID returns the schedule ID for cross-chain table compaction.
func (c *Client) GetCrossChainCompactionScheduleID() string {
	return c.CrossChainCompactionScheduleID
}

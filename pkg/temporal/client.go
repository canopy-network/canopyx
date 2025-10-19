package temporal

import (
	"context"
	"fmt"
	"time"

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
	ManagerQueue           string // manager - this is the queue for head-schedule and gap-scan or any other manager tasks in the future.
	ReportsQueue           string // reports - this is the queue for reports, global or by chain (to be defined)
	IndexerQueue           string // index:<chainID> - per chain queue for indexing blocks preventing a new out of data chain blocks an already up to date with a lot of missing heights. [DEPRECATED - keeping for backward compatibility]
	IndexerLiveQueue       string // index:<chainID>:live - per chain queue for live blocks (within threshold of head)
	IndexerHistoricalQueue string // index:<chainID>:historical - per chain queue for historical blocks (beyond threshold)
	IndexerOpsQueue        string // admin:<chainID> - per chain operations queue (headscan, gapscan, maintenance).

	// Schedule IDs
	HeadScheduleID          string
	GapScanScheduleID       string
	GlobalReportsScheduleID string

	// Workflow IDs
	IndexBlockWorkflowId         string
	SchedulerWorkflowID          string
	CleanupStagingWorkflowID     string
}

type Health struct {
	ConnectionOK bool                      `json:"connection_ok"`
	ManagerQueue []*taskqueuepb.PollerInfo `json:"manager_queue"`
	ReportsQueue []*taskqueuepb.PollerInfo `json:"reports_queue"`
}

func NewClient(ctx context.Context, logger *zap.Logger) (*Client, error) {
	host := utils.Env("TEMPORAL_HOSTPORT", "localhost:7233")
	ns := utils.Env("TEMPORAL_NAMESPACE", "canopyx")
	loggerWrapper := NewZapAdapter(logger)

	logger.Info("Connecting to Temporal", zap.String("host", host), zap.String("namespace", ns))
	tClient, err := Dial(ctx, host, ns, loggerWrapper)
	if err != nil {
		return nil, err
	}

	if _, err = tClient.CheckHealth(ctx, nil); err != nil {
		return nil, err
	}

	return &Client{
		TClient:   tClient,
		TSClient:  tClient.ScheduleClient(),
		Namespace: ns,
		// for now this is just hardcoded, could be configurable if we need it
		ManagerQueue:           "manager",
		ReportsQueue:           "reports",
		IndexerQueue:           "index:%s",           // Keep for backwards compat
		IndexerLiveQueue:       "index:%s:live",      // NEW: Live queue for recent blocks
		IndexerHistoricalQueue: "index:%s:historical", // NEW: Historical queue for old blocks
		IndexerOpsQueue:        "admin:%s",
		// schedule IDs
		HeadScheduleID:          "headscan:%s",
		GapScanScheduleID:       "gapscan:%s",
		GlobalReportsScheduleID: "reports:global",
		// workflow IDs
		IndexBlockWorkflowId:     "%s:index:%d",
		SchedulerWorkflowID:      "scheduler-%s",
		CleanupStagingWorkflowID: "cleanup-staging-%s-%d",
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

// GetManagerQueue returns the manager queue.
func (c *Client) GetManagerQueue() string { return c.ManagerQueue }

// GetGlobalReportsQueue returns the global reports queue.
func (c *Client) GetGlobalReportsQueue() string { return c.ReportsQueue }

// GetIndexerLiveQueue returns the live indexer queue for the given chain.
// This queue is for blocks within the LiveBlockThreshold of the chain head.
func (c *Client) GetIndexerLiveQueue(chainID string) string {
	return fmt.Sprintf(c.IndexerLiveQueue, chainID)
}

// GetIndexerHistoricalQueue returns the historical indexer queue for the given chain.
// This queue is for blocks beyond the LiveBlockThreshold from the chain head.
func (c *Client) GetIndexerHistoricalQueue(chainID string) string {
	return fmt.Sprintf(c.IndexerHistoricalQueue, chainID)
}

// GetIndexerOpsQueue returns the operations queue for the given chain.
func (c *Client) GetIndexerOpsQueue(chainID string) string {
	return fmt.Sprintf(c.IndexerOpsQueue, chainID)
}

// GetHeadScheduleID returns the schedule ID for the head scan for the given chain.
func (c *Client) GetHeadScheduleID(chainID string) string {
	return fmt.Sprintf(c.HeadScheduleID, chainID)
}

// GetGapScanScheduleID returns the schedule ID for the gap scan for the given chain.
func (c *Client) GetGapScanScheduleID(chainID string) string {
	return fmt.Sprintf(c.GapScanScheduleID, chainID)
}

// GetGlobalReportsScheduleID returns the schedule ID for the global reports.
func (c *Client) GetGlobalReportsScheduleID() string {
	return c.GlobalReportsScheduleID
}

// GetIndexBlockWorkflowId returns the workflow ID for the indexing block for the given chain and height.
func (c *Client) GetIndexBlockWorkflowId(chainID string, height uint64) string {
	return fmt.Sprintf(c.IndexBlockWorkflowId, chainID, height)
}

// GetIndexBlockWorkflowIdWithTime returns the workflow ID for the indexing block for the given chain and height with a timestamp.
func (c *Client) GetIndexBlockWorkflowIdWithTime(chainID string, height uint64) string {
	return fmt.Sprintf(c.IndexBlockWorkflowId+":%d", chainID, height, time.Now().UnixNano())
}

// GetSchedulerWorkflowID returns the deterministic workflow ID for the SchedulerWorkflow for a given chain.
func (c *Client) GetSchedulerWorkflowID(chainID string) string {
	return fmt.Sprintf(c.SchedulerWorkflowID, chainID)
}

// GetCleanupStagingWorkflowID returns the workflow ID for the cleanup staging workflow.
// This workflow is triggered after promotion to clean up staging tables asynchronously.
func (c *Client) GetCleanupStagingWorkflowID(chainID string, height uint64) string {
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
				// Update backlog age if activity backlog is older
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

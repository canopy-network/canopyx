package temporal

import (
	"context"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/pkg/utils"
	"go.uber.org/zap"

	"go.temporal.io/api/enums/v1"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	workflowservicepb "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/log"
)

type Client struct {
	TClient   client.Client
	TSClient  client.ScheduleClient
	Namespace string

	// Task Queues
	ManagerQueue string // manager - this is the queue for head-schedule and gap-scan or any other manager tasks in the future.
	ReportsQueue string // reports - this is the queue for reports, global or by chain (to be defined)
	IndexerQueue string // index:<chainID> - per chain queue for indexing blocks preventing a new out of data chain blocks an already up to date with a lot of missing heights.

	// Schedule IDs
	HeadScheduleID          string
	GapScanScheduleID       string
	GlobalReportsScheduleID string

	// Workflow IDs
	IndexBlockWorkflowId string
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
		ManagerQueue: "manager",
		ReportsQueue: "reports",
		IndexerQueue: "index:%s",
		// schedule IDs
		HeadScheduleID:          "headscan:%s",
		GapScanScheduleID:       "gapscan:%s",
		GlobalReportsScheduleID: "reports:global",
		// workflow IDs
		IndexBlockWorkflowId: "%s:index:%d",
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

// GetIndexerQueue returns the indexer queue for the given chain.
func (c *Client) GetIndexerQueue(chainID string) string {
	return fmt.Sprintf(c.IndexerQueue, chainID)
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

// TenSecondSpec returns a schedule spec for ten seconds.
func (c *Client) TenSecondSpec() client.ScheduleSpec {
	return c.GetScheduleSpec(10 * time.Second)
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

// Health returns the health of the Temporal client.
func (c *Client) Health(ctx context.Context) (Health, error) {
	h := Health{ConnectionOK: true}
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	svc := c.TClient.WorkflowService()
	if svc != nil {
		if rep, err := svc.DescribeTaskQueue(ctx, &workflowservicepb.DescribeTaskQueueRequest{
			Namespace:     c.Namespace,
			TaskQueue:     &taskqueuepb.TaskQueue{Name: c.ManagerQueue},
			TaskQueueType: enums.TASK_QUEUE_TYPE_WORKFLOW,
		}); err == nil {
			h.ManagerQueue = rep.GetPollers()
		}
		if rep, err := svc.DescribeTaskQueue(ctx, &workflowservicepb.DescribeTaskQueueRequest{
			Namespace:     c.Namespace,
			TaskQueue:     &taskqueuepb.TaskQueue{Name: c.ReportsQueue},
			TaskQueueType: enums.TASK_QUEUE_TYPE_WORKFLOW,
		}); err == nil {
			h.ReportsQueue = rep.GetPollers()
		}
	}
	return h, nil
}

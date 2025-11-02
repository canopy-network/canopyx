package indexer

// Workflow names for Temporal
const (
	IndexBlockWorkflowName     = "IndexBlockWorkflow"
	HeadScanWorkflowName       = "HeadScanWorkflow"
	GapScanWorkflowName        = "GapScanWorkflow"
	SchedulerWorkflowName      = "SchedulerWorkflow"
	CleanupStagingWorkflowName = "CleanupStagingWorkflow"
)

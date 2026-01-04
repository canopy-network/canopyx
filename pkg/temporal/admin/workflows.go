package admin

// Workflow names for admin maintenance operations
const (
	// CompactCrossChainTablesWorkflowName is the workflow that compacts all cross-chain global tables
	CompactCrossChainTablesWorkflowName = "CompactCrossChainTablesWorkflow"

	// DeleteChainWorkflowName is the workflow that permanently removes all data for a chain
	DeleteChainWorkflowName = "DeleteChainWorkflow"
)

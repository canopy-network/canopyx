package admin

// Workflow names for admin maintenance operations
const (
	// CompactGlobalTablesWorkflowName is the workflow that compacts all global tables
	CompactGlobalTablesWorkflowName = "CompactGlobalTablesWorkflow"

	// DeleteChainWorkflowName is the workflow that permanently removes all data for a chain
	DeleteChainWorkflowName = "DeleteChainWorkflow"
)

package types

import "time"

// --- Workflows

// WorkflowCompactCrossChainTablesWorkflowInput contains parameters for the compaction workflow.
type WorkflowCompactCrossChainTablesWorkflowInput struct {
	// No parameters needed currently - compacts all tables
}

// --- Activities

// ActivityCompactGlobalTableInput contains parameters for compacting a single global table.
type ActivityCompactGlobalTableInput struct {
	TableName string
}

// ActivityCompactGlobalTableOutput contains the result of compacting a table.
type ActivityCompactGlobalTableOutput struct {
	TableName string
	Success   bool
	Duration  time.Duration
	Error     string
}

// ActivityCompactAllGlobalTablesInput contains parameters for compacting all global tables.
type ActivityCompactAllGlobalTablesInput struct {
	// No parameters needed - compacts all tables from GetTableConfigs()
}

// ActivityCompactAllGlobalTablesOutput contains summary of compaction results.
type ActivityCompactAllGlobalTablesOutput struct {
	TotalTables   int
	SuccessCount  int
	FailureCount  int
	TotalDuration time.Duration
	TableResults  []ActivityCompactGlobalTableOutput
}

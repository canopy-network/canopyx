package types

// --- DeleteChainWorkflow types

// DeleteChainWorkflowInput contains parameters for the hard delete chain workflow.
type DeleteChainWorkflowInput struct {
	ChainID uint64
	// RenamedNamespace is the Temporal namespace after deletion (e.g., "chain_5-deleted-abc123").
	// The workflow must wait for this namespace to be fully cleaned up before completing.
	// Empty string means namespace didn't exist or was already deleted.
	RenamedNamespace string
}

// DeleteChainWorkflowOutput contains results of the hard delete chain workflow.
type DeleteChainWorkflowOutput struct {
	ChainID                  uint64
	ChainDataDeleted         bool
	AdminTablesCleaned       bool
	NamespaceCleanupComplete bool
	Errors                   []string
}

// --- Activities

// DeleteChainDataInput contains parameters for deleting chain data from global database.
type DeleteChainDataInput struct {
	ChainID uint64
}

// DeleteChainDataOutput contains results of deleting chain data.
type DeleteChainDataOutput struct {
	ChainID uint64
	Success bool
	Error   string
}

// CleanAdminTablesInput contains parameters for cleaning admin tables.
type CleanAdminTablesInput struct {
	ChainID uint64
}

// CleanAdminTablesOutput contains results of cleaning admin tables.
type CleanAdminTablesOutput struct {
	ChainID uint64
	Success bool
	Error   string
}

// WaitForNamespaceCleanupInput contains parameters for waiting on namespace cleanup.
type WaitForNamespaceCleanupInput struct {
	RenamedNamespace string
}

// WaitForNamespaceCleanupOutput contains results of namespace cleanup wait.
type WaitForNamespaceCleanupOutput struct {
	Success bool
	Error   string
}
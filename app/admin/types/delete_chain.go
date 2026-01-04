package types

// --- DeleteChainWorkflow types

// DeleteChainWorkflowInput contains parameters for the hard delete chain workflow.
type DeleteChainWorkflowInput struct {
	ChainID uint64
}

// DeleteChainWorkflowOutput contains results of the hard delete chain workflow.
type DeleteChainWorkflowOutput struct {
	ChainID            uint64
	CrossChainCleaned  bool
	DatabaseDropped    bool
	AdminTablesCleaned bool
	Errors             []string
}

// --- Activities

// CleanCrossChainDataInput contains parameters for cleaning cross-chain data.
type CleanCrossChainDataInput struct {
	ChainID uint64
}

// CleanCrossChainDataOutput contains results of cleaning cross-chain data.
type CleanCrossChainDataOutput struct {
	ChainID uint64
	Success bool
	Error   string
}

// DropChainDatabaseInput contains parameters for dropping the chain database.
type DropChainDatabaseInput struct {
	ChainID uint64
}

// DropChainDatabaseOutput contains results of dropping the chain database.
type DropChainDatabaseOutput struct {
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

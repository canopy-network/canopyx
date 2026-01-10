package global

import "github.com/canopy-network/canopyx/pkg/db/models/indexer"

// TableConfig defines the schema configuration for a global table.
// Each table has both production and staging variants with different ORDER BY strategies.
type TableConfig struct {
	TableName string // Base table name (e.g., "accounts")

	// ProductionOrderBy is the ORDER BY columns for production tables.
	// Pattern: (chain_id, entity_key, height) - optimized for entity lookups
	ProductionOrderBy []string

	// StagingOrderBy is the ORDER BY columns for staging tables.
	// Pattern: (height, chain_id, ...) - optimized for height-based cleanup
	StagingOrderBy []string

	// HasAddressColumn indicates whether to add bloom filter index on address column
	HasAddressColumn bool

	// UseCrossChainRename indicates whether this table has columns that need renaming
	// (e.g., events.chain_id -> event_chain_id, pools.chain_id -> pool_chain_id)
	UseCrossChainRename bool

	// Columns is the ColumnDef slice from indexer models
	Columns []indexer.ColumnDef

	// HasStaging indicates whether this table has a staging variant
	// (Most tables do, except proposal_snapshots and poll_snapshots)
	HasStaging bool

	// CustomSettings are additional table settings (e.g., index_granularity)
	CustomSettings string
}

// GetGlobalTableConfigs returns the configuration for all global tables.
// ORDER BY follows ClickHouse best practices:
// - Production: chain_id first (low cardinality), then entity key for lookups, then height
// - Staging: height first for efficient DELETE WHERE height IN (...) cleanup
func GetGlobalTableConfigs() []TableConfig {
	return []TableConfig{
		// --- Basic entities ---
		{
			TableName:         indexer.BlocksProductionTableName,
			ProductionOrderBy: []string{"chain_id", "height"},
			StagingOrderBy:    []string{"height", "chain_id"},
			HasAddressColumn:  false,
			Columns:           indexer.BlockColumns,
			HasStaging:        true,
		},
		{
			TableName:           indexer.TxsProductionTableName,
			ProductionOrderBy:   []string{"chain_id", "tx_hash", "height"},
			StagingOrderBy:      []string{"height", "chain_id", "tx_hash"},
			HasAddressColumn:    false,
			UseCrossChainRename: true, // tx_chain_id rename
			Columns:             indexer.TransactionColumns,
			HasStaging:          true,
		},
		{
			TableName:         indexer.AccountsProductionTableName,
			ProductionOrderBy: []string{"chain_id", "address", "height"},
			StagingOrderBy:    []string{"height", "chain_id", "address"},
			HasAddressColumn:  true,
			Columns:           indexer.AccountColumns,
			HasStaging:        true,
		},
		{
			TableName:           indexer.EventsProductionTableName,
			ProductionOrderBy:   []string{"chain_id", "event_type", "address", "reference", "height"},
			StagingOrderBy:      []string{"height", "chain_id", "event_type", "address", "reference"},
			HasAddressColumn:    true,
			UseCrossChainRename: true, // event_chain_id rename
			Columns:             indexer.EventColumns,
			HasStaging:          true,
		},
		{
			TableName:         indexer.BlockSummariesProductionTableName,
			ProductionOrderBy: []string{"chain_id", "height"},
			StagingOrderBy:    []string{"height", "chain_id"},
			HasAddressColumn:  false,
			Columns:           indexer.BlockSummaryColumns,
			HasStaging:        true,
		},
		{
			TableName:         indexer.ParamsProductionTableName,
			ProductionOrderBy: []string{"chain_id", "height"},
			StagingOrderBy:    []string{"height", "chain_id"},
			HasAddressColumn:  false,
			Columns:           indexer.ParamsColumns,
			HasStaging:        true,
		},
		{
			TableName:         indexer.SupplyProductionTableName,
			ProductionOrderBy: []string{"chain_id", "height"},
			StagingOrderBy:    []string{"height", "chain_id"},
			HasAddressColumn:  false,
			Columns:           indexer.SupplyColumns,
			HasStaging:        true,
			CustomSettings:    "SETTINGS index_granularity = 8192",
		},

		// --- Pool entities ---
		{
			TableName:           indexer.PoolsProductionTableName,
			ProductionOrderBy:   []string{"chain_id", "pool_id", "height"},
			StagingOrderBy:      []string{"height", "chain_id", "pool_id"},
			HasAddressColumn:    false,
			UseCrossChainRename: true, // pool_chain_id rename
			Columns:             indexer.PoolColumns,
			HasStaging:          true,
		},
		{
			TableName:         indexer.PoolPointsByHolderProductionTableName,
			ProductionOrderBy: []string{"chain_id", "pool_id", "address", "height"},
			StagingOrderBy:    []string{"height", "chain_id", "pool_id", "address"},
			HasAddressColumn:  true,
			Columns:           indexer.PoolPointsByHolderColumns,
			HasStaging:        true,
			CustomSettings:    "SETTINGS index_granularity = 8192",
		},

		// --- Order entities ---
		{
			TableName:         indexer.OrdersProductionTableName,
			ProductionOrderBy: []string{"chain_id", "order_id", "height"},
			StagingOrderBy:    []string{"height", "chain_id", "order_id"},
			HasAddressColumn:  false,
			Columns:           indexer.OrderColumns,
			HasStaging:        true,
		},

		// --- DEX entities ---
		{
			TableName:         indexer.DexPricesProductionTableName,
			ProductionOrderBy: []string{"chain_id", "local_chain_id", "remote_chain_id", "height"},
			StagingOrderBy:    []string{"height", "chain_id", "local_chain_id", "remote_chain_id"},
			HasAddressColumn:  false,
			Columns:           indexer.DexPriceColumns,
			HasStaging:        true,
		},
		{
			TableName:         indexer.DexOrdersProductionTableName,
			ProductionOrderBy: []string{"chain_id", "order_id", "height"},
			StagingOrderBy:    []string{"height", "chain_id", "order_id"},
			HasAddressColumn:  false,
			Columns:           indexer.DexOrderColumns,
			HasStaging:        true,
		},
		{
			TableName:         indexer.DexDepositsProductionTableName,
			ProductionOrderBy: []string{"chain_id", "order_id", "height"},
			StagingOrderBy:    []string{"height", "chain_id", "order_id"},
			HasAddressColumn:  false,
			Columns:           indexer.DexDepositColumns,
			HasStaging:        true,
		},
		{
			TableName:         indexer.DexWithdrawalsProductionTableName,
			ProductionOrderBy: []string{"chain_id", "order_id", "height"},
			StagingOrderBy:    []string{"height", "chain_id", "order_id"},
			HasAddressColumn:  false,
			Columns:           indexer.DexWithdrawalColumns,
			HasStaging:        true,
		},

		// --- Validator entities ---
		{
			TableName:         indexer.ValidatorsProductionTableName,
			ProductionOrderBy: []string{"chain_id", "address", "height"},
			StagingOrderBy:    []string{"height", "chain_id", "address"},
			HasAddressColumn:  true,
			Columns:           indexer.ValidatorColumns,
			HasStaging:        true,
			CustomSettings:    "SETTINGS index_granularity = 8192",
		},
		{
			TableName:         indexer.ValidatorNonSigningInfoProductionTableName,
			ProductionOrderBy: []string{"chain_id", "address", "height"},
			StagingOrderBy:    []string{"height", "chain_id", "address"},
			HasAddressColumn:  true,
			Columns:           indexer.ValidatorNonSigningInfoColumns,
			HasStaging:        true,
			CustomSettings:    "SETTINGS index_granularity = 8192",
		},
		{
			TableName:         indexer.ValidatorDoubleSigningInfoProductionTableName,
			ProductionOrderBy: []string{"chain_id", "address", "height"},
			StagingOrderBy:    []string{"height", "chain_id", "address"},
			HasAddressColumn:  true,
			Columns:           indexer.ValidatorDoubleSigningInfoColumns,
			HasStaging:        true,
			CustomSettings:    "SETTINGS index_granularity = 8192",
		},

		// --- Committee entities ---
		{
			TableName:           indexer.CommitteeProductionTableName,
			ProductionOrderBy:   []string{"chain_id", "committee_chain_id", "height"},
			StagingOrderBy:      []string{"height", "chain_id", "committee_chain_id"},
			HasAddressColumn:    false,
			UseCrossChainRename: true, // committee_chain_id rename
			Columns:             indexer.CommitteeColumns,
			HasStaging:          true,
		},
		{
			TableName:           indexer.CommitteeValidatorProductionTableName,
			ProductionOrderBy:   []string{"chain_id", "committee_id", "validator_address", "height"},
			StagingOrderBy:      []string{"height", "chain_id", "committee_id", "validator_address"},
			HasAddressColumn:    true,
			UseCrossChainRename: true,
			Columns:             indexer.CommitteeValidatorColumns,
			HasStaging:          true,
			CustomSettings:      "SETTINGS index_granularity = 8192",
		},
		{
			TableName:         indexer.CommitteePaymentsProductionTableName,
			ProductionOrderBy: []string{"chain_id", "committee_id", "address", "height"},
			StagingOrderBy:    []string{"height", "chain_id", "committee_id", "address"},
			HasAddressColumn:  true,
			Columns:           indexer.CommitteePaymentColumns,
			HasStaging:        true,
		},

		// --- Snapshot entities (NO staging tables) ---
		{
			TableName:         indexer.PollSnapshotsProductionTableName,
			ProductionOrderBy: []string{"chain_id", "proposal_hash", "snapshot_time"},
			StagingOrderBy:    nil, // No staging
			HasAddressColumn:  false,
			Columns:           indexer.PollSnapshotColumns,
			HasStaging:        false,
		},
		{
			TableName:         indexer.ProposalSnapshotsProductionTableName,
			ProductionOrderBy: []string{"chain_id", "proposal_hash", "snapshot_time"},
			StagingOrderBy:    nil, // No staging
			HasAddressColumn:  false,
			Columns:           indexer.ProposalSnapshotColumns,
			HasStaging:        false,
		},
	}
}

// GetColumnsForTable returns the column definitions for a given table name.
func GetColumnsForTable(tableName string) []indexer.ColumnDef {
	switch tableName {
	case indexer.AccountsProductionTableName:
		return indexer.AccountColumns
	case indexer.ValidatorsProductionTableName:
		return indexer.ValidatorColumns
	case indexer.ValidatorNonSigningInfoProductionTableName:
		return indexer.ValidatorNonSigningInfoColumns
	case indexer.ValidatorDoubleSigningInfoProductionTableName:
		return indexer.ValidatorDoubleSigningInfoColumns
	case indexer.PoolsProductionTableName:
		return indexer.PoolColumns
	case indexer.PoolPointsByHolderProductionTableName:
		return indexer.PoolPointsByHolderColumns
	case indexer.OrdersProductionTableName:
		return indexer.OrderColumns
	case indexer.DexOrdersProductionTableName:
		return indexer.DexOrderColumns
	case indexer.DexDepositsProductionTableName:
		return indexer.DexDepositColumns
	case indexer.DexWithdrawalsProductionTableName:
		return indexer.DexWithdrawalColumns
	case indexer.BlockSummariesProductionTableName:
		return indexer.BlockSummaryColumns
	case indexer.CommitteePaymentsProductionTableName:
		return indexer.CommitteePaymentColumns
	case indexer.EventsProductionTableName:
		return indexer.EventColumns
	case indexer.TxsProductionTableName:
		return indexer.TransactionColumns
	case indexer.CommitteeProductionTableName:
		return indexer.CommitteeColumns
	case indexer.CommitteeValidatorProductionTableName:
		return indexer.CommitteeValidatorColumns
	case indexer.DexPricesProductionTableName:
		return indexer.DexPriceColumns
	case indexer.ParamsProductionTableName:
		return indexer.ParamsColumns
	case indexer.SupplyProductionTableName:
		return indexer.SupplyColumns
	case indexer.BlocksProductionTableName:
		return indexer.BlockColumns
	case indexer.PollSnapshotsProductionTableName:
		return indexer.PollSnapshotColumns
	case indexer.ProposalSnapshotsProductionTableName:
		return indexer.ProposalSnapshotColumns
	default:
		return []indexer.ColumnDef{}
	}
}

// GetTableConfigByName returns the table config for a given table name, or nil if not found.
func GetTableConfigByName(tableName string) *TableConfig {
	for _, config := range GetGlobalTableConfigs() {
		if config.TableName == tableName {
			return &config
		}
	}
	return nil
}

// CompactableTable represents a table that needs compaction.
type CompactableTable struct {
	TableName string
}

// GetAllCompactableTables returns all tables that need OPTIMIZE TABLE FINAL compaction.
// This includes:
// - Production tables (from GetGlobalTableConfigs)
// - Created height aggregate tables (materialized view target tables)
// - LP position snapshot table
func GetAllCompactableTables() []CompactableTable {
	tables := make([]CompactableTable, 0, 40)

	// Add all production tables
	for _, config := range GetGlobalTableConfigs() {
		tables = append(tables, CompactableTable{TableName: config.TableName})
	}

	// Add created_height aggregate tables (AggregatingMergeTree tables for materialized views)
	createdHeightTables := []string{
		"account_created_height",
		"order_created_height",
		"dex_order_created_height",
		"dex_deposit_created_height",
		"dex_withdrawal_created_height",
		"pool_points_created_height",
		"pool_created_height",
		"validator_created_height",
		"committee_created_height",
	}
	for _, t := range createdHeightTables {
		tables = append(tables, CompactableTable{TableName: t})
	}

	// Add LP position snapshots table
	tables = append(tables, CompactableTable{TableName: indexer.LPPositionSnapshotsProductionTableName})

	return tables
}

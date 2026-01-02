package crosschain

import "github.com/canopy-network/canopyx/pkg/db/models/indexer"

// TableConfig defines the schema configuration for a global table.
type TableConfig struct {
	TableName        string   // Base table name (e.g., "accounts")
	PrimaryKey       []string // ORDER BY columns
	HasAddressColumn bool     // Whether to add bloom filter on address
	SchemaSQL        string   // Full CREATE TABLE schema (columns only, without ENGINE/ORDER BY)
	ColumnNames      []string // Explicit column names for INSERT/SELECT
}

// GetTableConfigs returns the configuration for the entities to sync.
// Each config is derived from the ColumnDef definitions in pkg/db/models/indexer/*.go
func GetTableConfigs() []TableConfig {
	return []TableConfig{
		{
			TableName:        indexer.AccountsProductionTableName,
			PrimaryKey:       []string{"chain_id", "address"},
			HasAddressColumn: true,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.AccountColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.AccountColumns),
		},
		{
			TableName:        indexer.ValidatorsProductionTableName,
			PrimaryKey:       []string{"chain_id", "address"},
			HasAddressColumn: true,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.ValidatorColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.ValidatorColumns),
		},
		{
			TableName:        indexer.ValidatorNonSigningInfoProductionTableName,
			PrimaryKey:       []string{"chain_id", "address"},
			HasAddressColumn: true,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.ValidatorNonSigningInfoColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.ValidatorNonSigningInfoColumns),
		},
		{
			TableName:        indexer.ValidatorDoubleSigningInfoProductionTableName,
			PrimaryKey:       []string{"chain_id", "address"},
			HasAddressColumn: true,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.ValidatorDoubleSigningInfoColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.ValidatorDoubleSigningInfoColumns),
		},
		{
			TableName:        indexer.PoolsProductionTableName,
			PrimaryKey:       []string{"chain_id", "pool_id"},
			HasAddressColumn: false,
			SchemaSQL:        indexer.ColumnsToCrossChainSchemaSQL(indexer.PoolColumns),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.PoolColumns),
		},
		{
			TableName:        indexer.PoolPointsByHolderProductionTableName,
			PrimaryKey:       []string{"chain_id", "address", "pool_id"},
			HasAddressColumn: true,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.PoolPointsByHolderColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.PoolPointsByHolderColumns),
		},
		{
			TableName:        indexer.OrdersProductionTableName,
			PrimaryKey:       []string{"chain_id", "order_id"},
			HasAddressColumn: false,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.OrderColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.OrderColumns),
		},
		{
			TableName:        indexer.DexOrdersProductionTableName,
			PrimaryKey:       []string{"chain_id", "order_id"},
			HasAddressColumn: false,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.DexOrderColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.DexOrderColumns),
		},
		{
			TableName:        indexer.DexDepositsProductionTableName,
			PrimaryKey:       []string{"chain_id", "order_id"},
			HasAddressColumn: false,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.DexDepositColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.DexDepositColumns),
		},
		{
			TableName:        indexer.DexWithdrawalsProductionTableName,
			PrimaryKey:       []string{"chain_id", "order_id"},
			HasAddressColumn: false,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.DexWithdrawalColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.DexWithdrawalColumns),
		},
		{
			TableName:        indexer.BlockSummariesProductionTableName,
			PrimaryKey:       []string{"chain_id", "height"},
			HasAddressColumn: false,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.BlockSummaryColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.BlockSummaryColumns),
		},
		{
			TableName:        indexer.CommitteePaymentsProductionTableName,
			PrimaryKey:       []string{"chain_id", "committee_id", "address", "height"},
			HasAddressColumn: true,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.CommitteePaymentColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.CommitteePaymentColumns),
		},
		{
			TableName:        indexer.EventsProductionTableName,
			PrimaryKey:       []string{"chain_id", "event_type", "address", "reference", "height"},
			HasAddressColumn: true,
			SchemaSQL:        indexer.ColumnsToCrossChainSchemaSQL(indexer.EventColumns),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.EventColumns),
		},
		{
			TableName:        indexer.TxsProductionTableName,
			PrimaryKey:       []string{"chain_id", "tx_hash", "height"},
			HasAddressColumn: false,
			SchemaSQL:        indexer.ColumnsToCrossChainSchemaSQL(indexer.TransactionColumns),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.TransactionColumns),
		},
		{
			TableName:        indexer.CommitteeProductionTableName,
			PrimaryKey:       []string{"chain_id", "committee_chain_id", "height"},
			HasAddressColumn: false,
			SchemaSQL:        indexer.ColumnsToCrossChainSchemaSQL(indexer.CommitteeColumns),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.CommitteeColumns),
		},
		{
			TableName:        indexer.CommitteeValidatorProductionTableName,
			PrimaryKey:       []string{"chain_id", "committee_id", "address", "height"},
			HasAddressColumn: true,
			SchemaSQL:        indexer.ColumnsToCrossChainSchemaSQL(indexer.CommitteeValidatorColumns),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.CommitteeValidatorColumns),
		},
		{
			TableName:        indexer.DexPricesProductionTableName,
			PrimaryKey:       []string{"chain_id", "local_chain_id", "remote_chain_id", "height"},
			HasAddressColumn: false,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.DexPriceColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.DexPriceColumns),
		},
		{
			TableName:        indexer.ParamsProductionTableName,
			PrimaryKey:       []string{"chain_id", "height"},
			HasAddressColumn: false,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.ParamsColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.ParamsColumns),
		},
		{
			TableName:        indexer.SupplyProductionTableName,
			PrimaryKey:       []string{"chain_id", "height"},
			HasAddressColumn: false,
			SchemaSQL:        indexer.ColumnsToSchemaSQL(indexer.FilterCrossChainColumns(indexer.SupplyColumns)),
			ColumnNames:      indexer.GetCrossChainColumnNames(indexer.SupplyColumns),
		},
	}
}

// GetColumnsForTable returns the column definitions for a given table name.
// This mapping connects table names to their ColumnDef definitions in pkg/db/models/indexer.
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
	default:
		// Return empty slice for unknown tables (should never happen if validateTableName is used)
		return []indexer.ColumnDef{}
	}
}

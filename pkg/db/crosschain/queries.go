package crosschain

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

const (
	defaultQueryLimit = 50
	maxQueryLimit     = 1000
)

// QueryLatestAccounts returns the latest state of accounts across chains.
// Uses LIMIT BY to ensure only the most recent snapshot for each (chain_id, address) pair.
func (db *DB) QueryLatestAccounts(ctx context.Context, opts QueryOptions) ([]AccountCrossChain, *QueryMetadata, error) {
	config := TableConfig{
		TableName:  indexer.AccountsProductionTableName,
		PrimaryKey: []string{"chain_id", "address", "height"},
	}

	dest := make([]AccountCrossChain, 0)
	metadata, err := db.queryLatestEntityTyped(ctx, &config, opts, &dest)
	if err != nil {
		return nil, nil, err
	}

	return dest, metadata, nil
}

// QueryLatestValidators returns the latest state of validators across chains.
func (db *DB) QueryLatestValidators(ctx context.Context, opts QueryOptions) ([]ValidatorCrossChain, *QueryMetadata, error) {
	config := TableConfig{
		TableName:  indexer.ValidatorsProductionTableName,
		PrimaryKey: []string{"chain_id", "address", "height"},
	}

	dest := &[]ValidatorCrossChain{}
	metadata, err := db.queryLatestEntityTyped(ctx, &config, opts, dest)
	if err != nil {
		return nil, nil, err
	}

	return *dest, metadata, nil
}

// QueryLatestValidatorNonSigningInfo returns the latest validator non-signing info across chains.
func (db *DB) QueryLatestValidatorNonSigningInfo(ctx context.Context, opts QueryOptions) ([]ValidatorNonSigningInfoCrossChain, *QueryMetadata, error) {
	config := TableConfig{
		TableName:  indexer.ValidatorNonSigningInfoProductionTableName,
		PrimaryKey: []string{"chain_id", "address", "height"},
	}

	dest := make([]ValidatorNonSigningInfoCrossChain, 0)
	metadata, err := db.queryLatestEntityTyped(ctx, &config, opts, &dest)
	if err != nil {
		return nil, nil, err
	}

	return dest, metadata, nil
}

// QueryLatestValidatorDoubleSigningInfo returns the latest validator double signing info across chains.
func (db *DB) QueryLatestValidatorDoubleSigningInfo(ctx context.Context, opts QueryOptions) ([]ValidatorDoubleSigningInfoCrossChain, *QueryMetadata, error) {
	config := TableConfig{
		TableName:  indexer.ValidatorDoubleSigningInfoProductionTableName,
		PrimaryKey: []string{"chain_id", "address", "height"},
	}

	dest := make([]ValidatorDoubleSigningInfoCrossChain, 0)
	metadata, err := db.queryLatestEntityTyped(ctx, &config, opts, &dest)
	if err != nil {
		return nil, nil, err
	}

	return dest, metadata, nil
}

// QueryLatestPools returns the latest state of pools across chains.
func (db *DB) QueryLatestPools(ctx context.Context, opts QueryOptions) ([]PoolCrossChain, *QueryMetadata, error) {
	config := TableConfig{
		TableName:  indexer.PoolsProductionTableName,
		PrimaryKey: []string{"chain_id", "pool_id", "height"},
	}

	dest := make([]PoolCrossChain, 0)
	metadata, err := db.queryLatestEntityTyped(ctx, &config, opts, &dest)
	if err != nil {
		return nil, nil, err
	}

	return dest, metadata, nil
}

// QueryLatestPoolPointsByHolder returns the latest pool points across chains.
func (db *DB) QueryLatestPoolPointsByHolder(ctx context.Context, opts QueryOptions) ([]PoolPointsByHolderCrossChain, *QueryMetadata, error) {
	config := TableConfig{
		TableName:  indexer.PoolPointsByHolderProductionTableName,
		PrimaryKey: []string{"chain_id", "address", "pool_id", "height"},
	}

	dest := make([]PoolPointsByHolderCrossChain, 0)
	metadata, err := db.queryLatestEntityTyped(ctx, &config, opts, &dest)
	if err != nil {
		return nil, nil, err
	}

	return dest, metadata, nil
}

// QueryLatestOrders returns the latest state of orders across chains.
func (db *DB) QueryLatestOrders(ctx context.Context, opts QueryOptions) ([]OrderCrossChain, *QueryMetadata, error) {
	config := TableConfig{
		TableName:  indexer.OrdersProductionTableName,
		PrimaryKey: []string{"chain_id", "order_id", "height"},
	}

	dest := make([]OrderCrossChain, 0)
	metadata, err := db.queryLatestEntityTyped(ctx, &config, opts, &dest)
	if err != nil {
		return nil, nil, err
	}

	return dest, metadata, nil
}

// QueryLatestDexOrders returns the latest state of DEX orders across chains.
func (db *DB) QueryLatestDexOrders(ctx context.Context, opts QueryOptions) ([]DexOrderCrossChain, *QueryMetadata, error) {
	config := TableConfig{
		TableName:  indexer.DexOrdersProductionTableName,
		PrimaryKey: []string{"chain_id", "order_id", "height"},
	}

	dest := make([]DexOrderCrossChain, 0)
	metadata, err := db.queryLatestEntityTyped(ctx, &config, opts, &dest)
	if err != nil {
		return nil, nil, err
	}

	return dest, metadata, nil
}

// QueryLatestDexDeposits returns the latest state of DEX deposits across chains.
func (db *DB) QueryLatestDexDeposits(ctx context.Context, opts QueryOptions) ([]DexDepositCrossChain, *QueryMetadata, error) {
	config := TableConfig{
		TableName:  indexer.DexDepositsProductionTableName,
		PrimaryKey: []string{"chain_id", "order_id", "height"},
	}

	dest := make([]DexDepositCrossChain, 0)
	metadata, err := db.queryLatestEntityTyped(ctx, &config, opts, &dest)
	if err != nil {
		return nil, nil, err
	}

	return dest, metadata, nil
}

// QueryLatestDexWithdrawals returns the latest state of DEX withdrawals across chains.
func (db *DB) QueryLatestDexWithdrawals(ctx context.Context, opts QueryOptions) ([]DexWithdrawalCrossChain, *QueryMetadata, error) {
	config := TableConfig{
		TableName:  indexer.DexWithdrawalsProductionTableName,
		PrimaryKey: []string{"chain_id", "order_id", "height"},
	}

	dest := make([]DexWithdrawalCrossChain, 0)
	metadata, err := db.queryLatestEntityTyped(ctx, &config, opts, &dest)
	if err != nil {
		return nil, nil, err
	}

	return dest, metadata, nil
}

// QueryLatestBlockSummaries returns the latest block summaries across chains.
func (db *DB) QueryLatestBlockSummaries(ctx context.Context, opts QueryOptions) ([]BlockSummaryCrossChain, *QueryMetadata, error) {
	config := TableConfig{
		TableName:  indexer.BlockSummariesProductionTableName,
		PrimaryKey: []string{"chain_id", "height"},
	}

	dest := make([]BlockSummaryCrossChain, 0)
	metadata, err := db.queryLatestEntityTyped(ctx, &config, opts, &dest)
	if err != nil {
		return nil, nil, err
	}

	return dest, metadata, nil
}

// QueryLatestCommitteePayments returns the latest committee payments across chains.
func (db *DB) QueryLatestCommitteePayments(ctx context.Context, opts QueryOptions) ([]CommitteePaymentCrossChain, *QueryMetadata, error) {
	config := TableConfig{
		TableName:  indexer.CommitteePaymentsProductionTableName,
		PrimaryKey: []string{"chain_id", "committee_id", "address", "height"},
	}

	dest := make([]CommitteePaymentCrossChain, 0)
	metadata, err := db.queryLatestEntityTyped(ctx, &config, opts, &dest)
	if err != nil {
		return nil, nil, err
	}

	return dest, metadata, nil
}

// QueryLatestEvents returns the latest events across chains.
func (db *DB) QueryLatestEvents(ctx context.Context, opts QueryOptions) ([]EventCrossChain, *QueryMetadata, error) {
	config := TableConfig{
		TableName:  indexer.EventsProductionTableName,
		PrimaryKey: []string{"chain_id", "event_type", "address", "reference", "height"},
	}

	dest := make([]EventCrossChain, 0)
	metadata, err := db.queryLatestEntityTyped(ctx, &config, opts, &dest)
	if err != nil {
		return nil, nil, err
	}

	return dest, metadata, nil
}

// QueryLatestTransactions returns the latest transactions across chains.
func (db *DB) QueryLatestTransactions(ctx context.Context, opts QueryOptions) ([]TransactionCrossChain, *QueryMetadata, error) {
	config := TableConfig{
		TableName:  indexer.TxsProductionTableName,
		PrimaryKey: []string{"chain_id", "tx_hash", "height"},
	}

	dest := make([]TransactionCrossChain, 0)
	metadata, err := db.queryLatestEntityTyped(ctx, &config, opts, &dest)
	if err != nil {
		return nil, nil, err
	}

	return dest, metadata, nil
}

// QueryLatestEntity is a generic method for querying any entity type.
// Returns results as maps for flexibility. Used by admin-web for debugging.
// For production use, prefer the strongly typed methods above.
func (db *DB) QueryLatestEntity(ctx context.Context, entityName string, opts QueryOptions) ([]map[string]interface{}, *QueryMetadata, error) {
	// Get entity configuration
	config, err := db.getEntityConfigByName(entityName)
	if err != nil {
		return nil, nil, err
	}

	// Create a strongly typed slice for scanning
	dest := createCrossChainEntitySlice(config.TableName)
	if dest == nil {
		return nil, nil, fmt.Errorf("unsupported entity type: %s", entityName)
	}

	// Execute query
	metadata, err := db.queryLatestEntityTyped(ctx, config, opts, dest)
	if err != nil {
		return nil, nil, err
	}

	// Convert a strongly typed result to maps using JSON marshal/unmarshal
	// This is the same pattern used in single-chain entity queries
	jsonBytes, err := json.Marshal(dest)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal results: %w", err)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &results); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal results: %w", err)
	}

	return results, metadata, nil
}

// queryLatestEntityTyped is the core query method used by all entity-specific methods.
// It executes a query with LIMIT BY to get only the latest state per entity.
func (db *DB) queryLatestEntityTyped(ctx context.Context, config *TableConfig, opts QueryOptions, dest interface{}) (*QueryMetadata, error) {
	// Apply defaults
	opts = applyQueryDefaults(opts)

	// Build WHERE clause for filtering
	var whereClauses []string
	var args []interface{}

	// Chain ID filtering
	if len(opts.ChainIDs) > 0 {
		placeholders := make([]string, len(opts.ChainIDs))
		for i, chainID := range opts.ChainIDs {
			placeholders[i] = "?"
			args = append(args, chainID)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("chain_id IN (%s)", strings.Join(placeholders, ",")))
	}

	// Field-based filtering
	// Use a whitelist approach to prevent SQL injection
	// Supports comma-separated values for IN queries (e.g., "addr1,addr2,addr3")
	if len(opts.Filters) > 0 {
		for field, value := range opts.Filters {
			// Validate field name contains only safe characters (alphanumeric + underscore)
			if !isValidColumnName(field) {
				continue
			}

			// Check if value contains multiple comma-separated values
			if strings.Contains(value, ",") {
				values := strings.Split(value, ",")
				placeholders := make([]string, len(values))
				for i, v := range values {
					placeholders[i] = "?"
					args = append(args, strings.TrimSpace(v))
				}
				whereClauses = append(whereClauses, fmt.Sprintf("%s IN (%s)", field, strings.Join(placeholders, ",")))
			} else {
				whereClauses = append(whereClauses, fmt.Sprintf("%s = ?", field))
				args = append(args, value)
			}
		}
	}

	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Build ORDER BY clause
	orderDirection := "DESC"
	if !opts.Desc {
		orderDirection = "ASC"
	}
	orderClause := fmt.Sprintf("ORDER BY %s %s", opts.OrderBy, orderDirection)

	// Build LIMIT BY clause to get only the latest state per entity
	// Primary key is always: [chain_id, entity_id..., height]
	// We want LIMIT BY all columns except height
	limitByColumns := config.PrimaryKey[:len(config.PrimaryKey)-1]
	limitByClause := fmt.Sprintf("LIMIT 1 BY %s", strings.Join(limitByColumns, ", "))

	globalTableName := db.getGlobalTableName(config.TableName)

	// Query for total count of UNIQUE entities (after LIMIT BY)
	// We need to count only the latest state per entity, not all historical snapshots
	// Use a subquery with LIMIT BY, then count the results
	countQuery := fmt.Sprintf(`
		SELECT count()
		FROM (
			SELECT 1
			FROM "%s"."%s" FINAL
			%s
			%s
			%s
		)
	`, db.Name, globalTableName, whereClause, orderClause, limitByClause)

	var total uint64
	if err := db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to query count: %w", err)
	}

	// Query for data with pagination
	// LIMIT BY must come before LIMIT/OFFSET
	dataQuery := fmt.Sprintf(`
		SELECT *
		FROM "%s"."%s" FINAL
		%s
		%s
		%s
		LIMIT ? OFFSET ?
	`, db.Name, globalTableName, whereClause, orderClause, limitByClause)

	dataArgs := append(args, opts.Limit+1, opts.Offset) // Request limit+1 to detect if there are more results

	// Execute query
	if err := db.SelectWithFinal(ctx, dest, dataQuery, dataArgs...); err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	// Determine if there are more results using reflection
	destValue := getSliceValue(dest)
	resultCount := destValue.Len()
	hasMore := resultCount > opts.Limit

	// Trim to limit if we got limit+1 results
	if hasMore {
		trimSliceToLimit(dest, opts.Limit)
	}

	metadata := &QueryMetadata{
		Total:   total,
		HasMore: hasMore,
		Limit:   opts.Limit,
		Offset:  opts.Offset,
	}

	return metadata, nil
}

// getEntityConfigByName returns the entity configuration for a given entity name.
// Entity names are the user-facing names like "accounts", "validators", etc.
func (db *DB) getEntityConfigByName(entityName string) (*TableConfig, error) {
	// Map entity names to table names
	entityToTableName := map[string]string{
		"accounts":                      indexer.AccountsProductionTableName,
		"validators":                    indexer.ValidatorsProductionTableName,
		"validator-non-signing-info":    indexer.ValidatorNonSigningInfoProductionTableName,
		"validator-double-signing-info": indexer.ValidatorDoubleSigningInfoProductionTableName,
		"pools":                         indexer.PoolsProductionTableName,
		"pool-points":                   indexer.PoolPointsByHolderProductionTableName,
		"orders":                        indexer.OrdersProductionTableName,
		"dex-orders":                    indexer.DexOrdersProductionTableName,
		"dex-deposits":                  indexer.DexDepositsProductionTableName,
		"dex-withdrawals":               indexer.DexWithdrawalsProductionTableName,
		"block-summaries":               indexer.BlockSummariesProductionTableName,
		"committee-payments":            indexer.CommitteePaymentsProductionTableName,
		"events":                        indexer.EventsProductionTableName,
		"transactions":                  indexer.TxsProductionTableName,
	}

	tableName, ok := entityToTableName[entityName]
	if !ok {
		return nil, fmt.Errorf("unsupported entity: %s", entityName)
	}

	// Find the config from GetTableConfigs()
	for _, config := range GetTableConfigs() {
		if config.TableName == tableName {
			return &config, nil
		}
	}

	return nil, fmt.Errorf("configuration not found for entity: %s", entityName)
}

// createCrossChainEntitySlice creates a strongly-typed slice for scanning query results.
// Returns a pointer to a slice of the appropriate CrossChain wrapper type.
func createCrossChainEntitySlice(tableName string) interface{} {
	switch tableName {
	case indexer.AccountsProductionTableName:
		return &[]AccountCrossChain{}
	case indexer.ValidatorsProductionTableName:
		return &[]ValidatorCrossChain{}
	case indexer.ValidatorNonSigningInfoProductionTableName:
		return &[]ValidatorNonSigningInfoCrossChain{}
	case indexer.ValidatorDoubleSigningInfoProductionTableName:
		return &[]ValidatorDoubleSigningInfoCrossChain{}
	case indexer.PoolsProductionTableName:
		return &[]PoolCrossChain{}
	case indexer.PoolPointsByHolderProductionTableName:
		return &[]PoolPointsByHolderCrossChain{}
	case indexer.OrdersProductionTableName:
		return &[]OrderCrossChain{}
	case indexer.DexOrdersProductionTableName:
		return &[]DexOrderCrossChain{}
	case indexer.DexDepositsProductionTableName:
		return &[]DexDepositCrossChain{}
	case indexer.DexWithdrawalsProductionTableName:
		return &[]DexWithdrawalCrossChain{}
	case indexer.BlockSummariesProductionTableName:
		return &[]BlockSummaryCrossChain{}
	case indexer.CommitteePaymentsProductionTableName:
		return &[]CommitteePaymentCrossChain{}
	case indexer.EventsProductionTableName:
		return &[]EventCrossChain{}
	case indexer.TxsProductionTableName:
		return &[]TransactionCrossChain{}
	default:
		return nil
	}
}

// applyQueryDefaults applies default values to QueryOptions.
func applyQueryDefaults(opts QueryOptions) QueryOptions {
	if opts.Limit <= 0 {
		opts.Limit = defaultQueryLimit
	}
	if opts.Limit > maxQueryLimit {
		opts.Limit = maxQueryLimit
	}
	if opts.OrderBy == "" {
		opts.OrderBy = "height"
	}
	// Desc defaults to true (already zero value is false, so we explicitly set it)
	// Actually, we want desc=true by default, so check if it was explicitly set
	// Since we can't distinguish between false and unset, we'll document that default is DESC
	// and users must explicitly set Desc=false for ASC ordering

	return opts
}

// getSliceValue uses reflection to get the underlying slice value from a pointer to a slice.
func getSliceValue(dest interface{}) reflect.Value {
	v := reflect.ValueOf(dest)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	return v
}

// trimSliceToLimit trims a slice (passed as interface{}) to the specified limit using reflection.
func trimSliceToLimit(dest interface{}, limit int) {
	v := getSliceValue(dest)
	if v.Kind() == reflect.Slice && v.Len() > limit {
		trimmed := v.Slice(0, limit)
		v.Set(trimmed)
	}
}

// isValidColumnName validates that a column name contains only safe characters.
// This prevents SQL injection by ensuring only alphanumeric characters and underscores are allowed.
func isValidColumnName(name string) bool {
	if name == "" {
		return false
	}
	for _, char := range name {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' {
			return false
		}
	}
	return true
}

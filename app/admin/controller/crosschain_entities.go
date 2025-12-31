package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/canopy-network/canopyx/pkg/db/crosschain"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// HandleCrossChainEntities handles dynamic cross-chain entity queries.
// GET /api/crosschain/entities/{entity}?chain_id=1,2&limit=50&offset=0&order_by=height&sort=desc
//
// Path Parameters:
//   - entity: Entity name (accounts, validators, pools, orders, etc.)
//
// Query Parameters:
//   - chain_id: Comma-separated list of chain IDs to filter (optional, default: all chains)
//   - limit: Number of records to return (default: 50, max: 1000)
//   - offset: Offset for pagination (default: 0)
//   - order_by: Column to sort by (default: height)
//   - sort: Sort order - "asc" or "desc" (default: "desc")
//
// Response:
//
//	{
//	  "data": [{...}],
//	  "total": 1000,
//	  "limit": 50,
//	  "offset": 0,
//	  "has_more": true
//	}
func (c *Controller) HandleCrossChainEntities(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	entityName := vars["entity"]

	// Parse query options from request
	opts, err := c.parseCrossChainQueryOptions(r)
	if err != nil {
		c.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request parameters: %v", err))
		return
	}

	// Execute the query using the crosschain store API
	results, metadata, err := c.App.CrossChainDB.QueryLatestEntity(ctx, entityName, opts)
	if err != nil {
		c.App.Logger.Error("Failed to query cross-chain entity data",
			zap.String("entity", entityName),
			zap.Error(err))
		c.writeError(w, http.StatusInternalServerError, "failed to query cross-chain entity data")
		return
	}

	// Build response
	response := map[string]interface{}{
		"data":     results,
		"total":    metadata.Total,
		"limit":    metadata.Limit,
		"offset":   metadata.Offset,
		"has_more": metadata.HasMore,
	}

	c.writeJSON(w, http.StatusOK, response)
}

// HandleCrossChainEntitySchema returns the schema for a cross-chain entity.
// GET /api/crosschain/entities/{entity}/schema
//
// Path Parameters:
//   - entity: Entity name (accounts, validators, pools, orders, etc.)
//
// Response:
//
//	{
//	  "entity": "accounts",
//	  "global_table": "accounts_global",
//	  "properties": {...},
//	  "primary_key": [...],
//	  "filterable_columns": [...],
//	  "sortable_columns": [...]
//	}
func (c *Controller) HandleCrossChainEntitySchema(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	entityName := vars["entity"]

	// Validate entity and get configuration
	config, err := c.getCrossChainEntityConfig(entityName)
	if err != nil {
		c.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid entity: %v", err))
		return
	}

	// Build schema response
	schema := map[string]interface{}{
		"entity":       entityName,
		"global_table": config.TableName + "_global",
		"primary_key":  config.PrimaryKey,
		// Note: For full schema introspection (properties, types), we'd need to add reflection
		// or maintain a mapping. For now, we provide basic info that's immediately available.
	}

	c.writeJSON(w, http.StatusOK, schema)
}

// HandleCrossChainEntitiesList returns list of available cross-chain entities.
// GET /api/crosschain/entities
//
// Response:
//
//	{
//	  "entities": [
//	    {"name": "accounts", "global_table": "accounts_global", "route": "accounts"},
//	    {"name": "validators", "global_table": "validators_global", "route": "validators"},
//	    ...
//	  ]
//	}
func (c *Controller) HandleCrossChainEntitiesList(w http.ResponseWriter, _ *http.Request) {
	// Get all table configs
	configs := crosschain.GetTableConfigs()

	// Entity names mapping (same as in crosschain package)
	entityNames := []string{
		"accounts",
		"validators",
		"validator-signing-info",
		"validator-double-signing-info",
		"pools",
		"pool-points",
		"orders",
		"dex-orders",
		"dex-deposits",
		"dex-withdrawals",
		"block-summaries",
	}

	entities := make([]map[string]interface{}, 0, len(configs))
	for i, config := range configs {
		if i < len(entityNames) {
			entities = append(entities, map[string]interface{}{
				"name":         entityNames[i],
				"table_name":   config.TableName,
				"global_table": config.TableName + "_global",
				"route":        entityNames[i],
			})
		}
	}

	c.writeJSON(w, http.StatusOK, map[string]interface{}{
		"entities": entities,
	})
}

// parseCrossChainQueryOptions parses HTTP query parameters into QueryOptions.
func (c *Controller) parseCrossChainQueryOptions(r *http.Request) (crosschain.QueryOptions, error) {
	opts := crosschain.QueryOptions{
		Desc: true, // Default to descending (newest first)
	}

	queryParams := r.URL.Query()

	// Parse chain_id parameter (comma-separated list)
	if chainIDStr := queryParams.Get("chain_id"); chainIDStr != "" {
		chainIDParts := strings.Split(chainIDStr, ",")
		for _, part := range chainIDParts {
			chainID, err := strconv.ParseUint(strings.TrimSpace(part), 10, 64)
			if err != nil {
				return opts, fmt.Errorf("invalid chain_id: %s", part)
			}
			opts.ChainIDs = append(opts.ChainIDs, chainID)
		}
	}

	// Parse limit
	if limitStr := queryParams.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return opts, fmt.Errorf("invalid limit: %w", err)
		}
		if limit <= 0 {
			return opts, fmt.Errorf("limit must be positive")
		}
		opts.Limit = limit
	}

	// Parse offset
	if offsetStr := queryParams.Get("offset"); offsetStr != "" {
		offset, err := strconv.ParseUint(offsetStr, 10, 64)
		if err != nil {
			return opts, fmt.Errorf("invalid offset: %w", err)
		}
		opts.Offset = offset
	}

	// Parse order_by
	if orderBy := queryParams.Get("order_by"); orderBy != "" {
		opts.OrderBy = orderBy
	}

	// Parse sort order
	if sortStr := queryParams.Get("sort"); sortStr != "" {
		sortLower := strings.ToLower(sortStr)
		switch sortLower {
		case "asc":
			opts.Desc = false
		case "desc":
			opts.Desc = true
		default:
			return opts, fmt.Errorf("invalid sort order: must be 'asc' or 'desc'")
		}
	}

	// Parse field-based filters
	// Any query parameter prefixed with "filter_" becomes a field filter
	// Example: ?filter_address=abc123&filter_status=active
	opts.Filters = make(map[string]string)
	for param, values := range queryParams {
		if strings.HasPrefix(param, "filter_") && len(values) > 0 {
			fieldName := strings.TrimPrefix(param, "filter_")
			if fieldName != "" && values[0] != "" {
				opts.Filters[fieldName] = values[0]
			}
		}
	}

	return opts, nil
}

// getCrossChainEntityConfig retrieves entity configuration from the crosschain store.
func (c *Controller) getCrossChainEntityConfig(entityName string) (*crosschain.TableConfig, error) {
	// Get all configs and find the matching one
	configs := crosschain.GetTableConfigs()

	// Map entity names to table names
	entityToTableName := map[string]string{
		"accounts":                      "accounts",
		"validators":                    "validators",
		"validator-signing-info":        "validator_signing_info",
		"validator-double-signing-info": "validator_double_signing_info",
		"pools":                         "pools",
		"pool-points":                   "pool_points_by_holder",
		"orders":                        "orders",
		"dex-orders":                    "dex_orders",
		"dex-deposits":                  "dex_deposits",
		"dex-withdrawals":               "dex_withdrawals",
		"block-summaries":               "block_summaries",
	}

	tableName, ok := entityToTableName[entityName]
	if !ok {
		return nil, fmt.Errorf("unsupported entity: %s", entityName)
	}

	// Find matching config
	for _, config := range configs {
		if config.TableName == tableName {
			return &config, nil
		}
	}

	return nil, fmt.Errorf("configuration not found for entity: %s", entityName)
}

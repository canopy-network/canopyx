package controller

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/canopy-network/canopyx/app/admin/controller/types"
	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/db/entities"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/go-jose/go-jose/v4/json"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

const (
	// defaultQueryLimit is the default number of records to return if no limit is specified
	defaultQueryLimit = 50
	// maxQueryLimit is the maximum number of records that can be requested in a single query
	maxQueryLimit = 1000
)

// HandleEntityQuery handles generic entity queries with pagination support.
// GET /api/chains/{id}/entity/{entity}?limit=50&cursor=123&sort=desc&use_staging=false
//
// Query Parameters:
//   - limit: Number of records to return (default: 50, max: 1000)
//   - cursor: Pagination cursor (height value to start from)
//   - sort: Sort order - "asc" or "desc" (default: "desc")
//   - use_staging: Query staging table instead of production (default: false)
//
// Response:
//
//	{
//	  "data": [{...}],
//	  "limit": 50,
//	  "next_cursor": 123
//	}
func (c *Controller) HandleEntityQuery(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	chainID := vars["id"]
	entityName := vars["entity"]

	// Parse and validate request parameters
	req, err := c.parseEntityQueryRequest(r)
	if err != nil {
		c.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request parameters: %v", err))
		return
	}

	// Validate and get entity
	entity, err := entities.FromString(entityName)
	if err != nil {
		c.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid entity: %v", err))
		return
	}

	// Load chain store
	store, ok := c.App.LoadChainStore(ctx, chainID)
	if !ok {
		c.writeError(w, http.StatusNotFound, "chain not found")
		return
	}

	// Determine which table to query (staging vs production)
	tableName := entity.TableName()
	if req.UseStaging {
		tableName = entity.StagingTableName()
	}

	// Execute the query
	data, nextCursor, err := c.queryEntityData(ctx, store, tableName, req)
	if err != nil {
		c.App.Logger.Error("Failed to query entity data",
			zap.String("chainID", chainID),
			zap.String("entity", entityName),
			zap.String("table", tableName),
			zap.Error(err),
		)
		c.writeError(w, http.StatusInternalServerError, "failed to query entity data")
		return
	}

	// Build response
	resp := types.EntityQueryResponse{
		Data:       data,
		Limit:      req.Limit,
		NextCursor: nextCursor,
	}

	c.writeJSON(w, http.StatusOK, resp)
}

// HandleEntityGet handles single entity lookups with explicit property/value query parameters.
// GET /api/chains/{id}/entity/{entity}?property=hash&value=73529c...&height=123&use_staging=false
//
// Path Parameters:
//   - id: Chain ID
//   - entity: Entity name (e.g., "blocks", "accounts", "txs")
//
// Query Parameters:
//   - property: Column name to query (hash, address, order_id, id, etc.) - REQUIRED
//   - value: Value to search for - REQUIRED
//   - height: Optional - if 0 or omitted, get latest (ORDER BY height DESC LIMIT 1); if specified, get at that exact height
//   - use_staging: Query staging table instead of production (default: false)
//
// Response: Single entity object or 404 if not found
func (c *Controller) HandleEntityGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	chainID := vars["id"]
	entityName := vars["entity"]

	// Parse request parameters (includes property and value)
	req, err := c.parseEntityGetRequest(r)
	if err != nil {
		c.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request parameters: %v", err))
		return
	}

	// Validate and get entity
	entity, err := entities.FromString(entityName)
	if err != nil {
		c.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid entity: %v", err))
		return
	}

	// Load chain store
	store, ok := c.App.LoadChainStore(ctx, chainID)
	if !ok {
		c.writeError(w, http.StatusNotFound, "chain not found")
		return
	}

	// Determine which table to query (staging vs production)
	tableName := entity.TableName()
	if req.UseStaging {
		tableName = entity.StagingTableName()
	}

	// Execute the query using the explicit property and value
	result, err := c.getEntityByID(ctx, store, tableName, req.Property, req.Value, req.Height)
	if err != nil {
		c.App.Logger.Error("Failed to get entity by ID",
			zap.String("chainID", chainID),
			zap.String("entity", entityName),
			zap.String("property", req.Property),
			zap.String("value", req.Value),
			zap.Error(err),
		)
		c.writeError(w, http.StatusInternalServerError, "failed to get entity")
		return
	}

	if result == nil {
		c.writeError(w, http.StatusNotFound, "entity not found")
		return
	}

	c.writeJSON(w, http.StatusOK, result)
}

// parseEntityQueryRequest parses and validates entity query request parameters
func (c *Controller) parseEntityQueryRequest(r *http.Request) (types.EntityQueryRequest, error) {
	req := types.EntityQueryRequest{
		Limit:      defaultQueryLimit,
		Cursor:     0,
		SortDesc:   true, // Default to descending (newest first)
		UseStaging: false,
	}

	// Parse limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return req, fmt.Errorf("invalid limit: %w", err)
		}
		if limit <= 0 {
			return req, fmt.Errorf("limit must be positive")
		}
		if limit > maxQueryLimit {
			limit = maxQueryLimit
		}
		req.Limit = limit
	}

	// Parse cursor
	if cursorStr := r.URL.Query().Get("cursor"); cursorStr != "" {
		cursor, err := strconv.ParseUint(cursorStr, 10, 64)
		if err != nil {
			return req, fmt.Errorf("invalid cursor: %w", err)
		}
		req.Cursor = cursor
	}

	// Parse sort order
	if sortStr := r.URL.Query().Get("sort"); sortStr != "" {
		sortLower := strings.ToLower(sortStr)
		switch sortLower {
		case "asc":
			req.SortDesc = false
		case "desc":
			req.SortDesc = true
		default:
			return req, fmt.Errorf("invalid sort order: must be 'asc' or 'desc'")
		}
	}

	// Parse use_staging
	if useStagingStr := r.URL.Query().Get("use_staging"); useStagingStr != "" {
		useStaging, err := strconv.ParseBool(useStagingStr)
		if err != nil {
			return req, fmt.Errorf("invalid use_staging: %w", err)
		}
		req.UseStaging = useStaging
	}

	return req, nil
}

// parseEntityGetRequest parses and validates entity get request parameters
func (c *Controller) parseEntityGetRequest(r *http.Request) (types.EntityGetRequest, error) {
	req := types.EntityGetRequest{
		Height:     nil,
		UseStaging: false,
	}

	// Parse property (required)
	req.Property = r.URL.Query().Get("property")
	if req.Property == "" {
		return req, fmt.Errorf("property parameter is required")
	}

	// Parse value (required)
	req.Value = r.URL.Query().Get("value")
	if req.Value == "" {
		return req, fmt.Errorf("value parameter is required")
	}

	// Parse height (optional)
	if heightStr := r.URL.Query().Get("height"); heightStr != "" {
		height, err := strconv.ParseUint(heightStr, 10, 64)
		if err != nil {
			return req, fmt.Errorf("invalid height: %w", err)
		}
		req.Height = &height
	}

	// Parse use_staging
	if useStagingStr := r.URL.Query().Get("use_staging"); useStagingStr != "" {
		useStaging, err := strconv.ParseBool(useStagingStr)
		if err != nil {
			return req, fmt.Errorf("invalid use_staging: %w", err)
		}
		req.UseStaging = useStaging
	}

	return req, nil
}

// createEntitySlice creates an empty slice of the appropriate type for the given entity
func createEntitySlice(entity entities.Entity) interface{} {
	switch entity {
	case entities.Blocks:
		return &[]indexermodels.Block{}
	case entities.Transactions:
		return &[]indexermodels.Transaction{}
	case entities.BlockSummaries:
		return &[]indexermodels.BlockSummary{}
	case entities.Accounts:
		return &[]indexermodels.Account{}
	case entities.Events:
		return &[]indexermodels.Event{}
	case entities.Orders:
		return &[]indexermodels.Order{}
	case entities.Pools:
		return &[]indexermodels.Pool{}
	case entities.DexPrices:
		return &[]indexermodels.DexPrice{}
	case entities.DexOrders:
		return &[]indexermodels.DexOrder{}
	case entities.DexDeposits:
		return &[]indexermodels.DexDeposit{}
	case entities.DexWithdrawals:
		return &[]indexermodels.DexWithdrawal{}
	case entities.PoolPointsByHolder:
		return &[]indexermodels.PoolPointsByHolder{}
	case entities.Params:
		return &[]indexermodels.Params{}
	case entities.Validators:
		return &[]indexermodels.Validator{}
	case entities.ValidatorSigningInfo:
		return &[]indexermodels.ValidatorSigningInfo{}
	case entities.ValidatorDoubleSigningInfo:
		return &[]indexermodels.ValidatorDoubleSigningInfo{}
	case entities.Committees:
		return &[]indexermodels.Committee{}
	case entities.CommitteeValidators:
		return &[]indexermodels.CommitteeValidator{}
	case entities.CommitteePayments:
		return &[]indexermodels.CommitteePayment{}
	case entities.PollSnapshots:
		return &[]indexermodels.PollSnapshot{}
	case entities.Supply:
		return &[]indexermodels.Supply{}
	default:
		return nil
	}
}

// queryEntityData executes a generic entity query with pagination
func (c *Controller) queryEntityData(
	ctx context.Context,
	store db.ChainStore,
	tableName string,
	req types.EntityQueryRequest,
) ([]map[string]interface{}, *uint64, error) {
	// Get the entity from table name
	entityName := strings.TrimSuffix(tableName, entities.StagingSuffix)
	entity, err := entities.FromString(entityName)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid entity: %w", err)
	}

	// Build query with cursor-based pagination
	// Request limit+1 to detect if there are more results
	queryLimit := req.Limit + 1

	var whereClause string
	var args []interface{}

	// Add cursor condition if provided
	if req.Cursor > 0 {
		if req.SortDesc {
			whereClause = "WHERE height < ?"
		} else {
			whereClause = "WHERE height > ?"
		}
		args = append(args, req.Cursor)
	}

	// Build sort clause
	orderClause := "ORDER BY height DESC"
	if !req.SortDesc {
		orderClause = "ORDER BY height ASC"
	}

	// Build final query
	// Note: Using FINAL for ReplacingMergeTree tables to get deduplicated results
	// Both production and staging tables use ReplacingMergeTree
	query := fmt.Sprintf(`
		SELECT *
		FROM "%s"."%s" FINAL
		%s
		%s
		LIMIT ?
	`, store.DatabaseName(), tableName, whereClause, orderClause)

	args = append(args, queryLimit)

	// Create the appropriate slice type for this entity
	dest := createEntitySlice(entity)
	if dest == nil {
		return nil, nil, fmt.Errorf("unsupported entity type: %s", entity)
	}

	// Execute query using Select() with the typed slice
	if err := store.Select(ctx, dest, query, args...); err != nil {
		return nil, nil, fmt.Errorf("query failed: %w", err)
	}

	// Convert the strongly-typed slice to []map[string]interface{}
	jsonBytes, err := json.Marshal(dest)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal results: %w", err)
	}

	var rawResults []map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &rawResults); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal results: %w", err)
	}

	// Determine if there are more results
	var nextCursor *uint64
	hasMore := len(rawResults) > req.Limit
	if hasMore {
		// Remove the extra record
		rawResults = rawResults[:req.Limit]

		// Extract the last height as the next cursor
		if len(rawResults) > 0 {
			if heightVal, ok := rawResults[len(rawResults)-1]["height"]; ok {
				// Try different numeric types that JSON might return
				switch v := heightVal.(type) {
				case float64:
					u := uint64(v)
					nextCursor = &u
				case uint64:
					nextCursor = &v
				case int64:
					u := uint64(v)
					nextCursor = &u
				}
			}
		}
	}

	return rawResults, nextCursor, nil
}

// getEntityByID retrieves a single entity by its primary key value using explicit property and value parameters
func (c *Controller) getEntityByID(
	ctx context.Context,
	store db.ChainStore,
	tableName string,
	property string,
	value string,
	height *uint64,
) (map[string]interface{}, error) {
	// Get the entity from table name
	entityName := strings.TrimSuffix(tableName, entities.StagingSuffix)
	entity, err := entities.FromString(entityName)
	if err != nil {
		return nil, fmt.Errorf("invalid entity: %w", err)
	}

	var query string
	var args []interface{}

	// Build query based on whether height is specified
	if height == nil || *height == 0 {
		// Get latest - ORDER BY height DESC LIMIT 1
		query = fmt.Sprintf(`
			SELECT *
			FROM "%s"."%s" FINAL
			WHERE %s = ?
			ORDER BY height DESC
			LIMIT 1
		`, store.DatabaseName(), tableName, property)
		args = []interface{}{value}
	} else {
		// Get at specific height - exact height match with the property
		query = fmt.Sprintf(`
			SELECT *
			FROM "%s"."%s" FINAL
			WHERE %s = ? AND height = ?
			LIMIT 1
		`, store.DatabaseName(), tableName, property)
		args = []interface{}{value, *height}
	}

	// Create a slice to hold the result (we expect 1 item)
	dest := createEntitySlice(entity)
	if dest == nil {
		return nil, fmt.Errorf("unsupported entity type: %s", entity)
	}

	// Execute query using Select() with the typed slice
	if err := store.Select(ctx, dest, query, args...); err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	// Convert the strongly-typed slice to []map[string]interface{}
	jsonBytes, err := json.Marshal(dest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &results); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	// Return the first result, or nil if no results
	if len(results) == 0 {
		return nil, nil
	}

	return results[0], nil
}

// writeJSON writes a JSON response
func (c *Controller) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

// HandleEntitySchema returns the schema (column names and types) for an entity.
// GET /api/chains/{id}/entity/{entity}/schema
//
// Path Parameters:
//   - id: Chain ID
//   - entity: Entity name (e.g., "blocks", "accounts", "txs")
//
// Response:
//
//	{
//	  "properties": {
//	    "hash": "string",
//	    "height": "uint64",
//	    "amount": "uint64",
//	    ...
//	  }
//	}
func (c *Controller) HandleEntitySchema(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	entityName := vars["entity"]

	// Validate and get entity
	entity, err := entities.FromString(entityName)
	if err != nil {
		c.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid entity: %v", err))
		return
	}

	// Extract schema from the indexer model using reflection
	schema := c.getEntitySchemaProperties(entity)
	if schema == nil {
		c.writeError(w, http.StatusInternalServerError, "failed to extract entity schema")
		return
	}

	c.writeJSON(w, http.StatusOK, map[string]interface{}{
		"properties": schema,
	})
}

// getEntitySchemaProperties extracts field names and types from the indexer model using reflection.
// It reads the `ch` struct tags which define the ClickHouse column names and field types.
func (c *Controller) getEntitySchemaProperties(entity entities.Entity) map[string]string {
	var model interface{}

	// Map entity to its corresponding indexer model
	switch entity {
	case entities.Blocks:
		model = indexermodels.Block{}
	case entities.Transactions:
		model = indexermodels.Transaction{}
	case entities.BlockSummaries:
		model = indexermodels.BlockSummary{}
	case entities.Accounts:
		model = indexermodels.Account{}
	case entities.Events:
		model = indexermodels.Event{}
	case entities.Orders:
		model = indexermodels.Order{}
	case entities.Pools:
		model = indexermodels.Pool{}
	case entities.DexPrices:
		model = indexermodels.DexPrice{}
	case entities.DexOrders:
		model = indexermodels.DexOrder{}
	case entities.DexDeposits:
		model = indexermodels.DexDeposit{}
	case entities.DexWithdrawals:
		model = indexermodels.DexWithdrawal{}
	case entities.PoolPointsByHolder:
		model = indexermodels.PoolPointsByHolder{}
	case entities.Params:
		model = indexermodels.Params{}
	case entities.Validators:
		model = indexermodels.Validator{}
	case entities.ValidatorSigningInfo:
		model = indexermodels.ValidatorSigningInfo{}
	case entities.ValidatorDoubleSigningInfo:
		model = indexermodels.ValidatorDoubleSigningInfo{}
	case entities.Committees:
		model = indexermodels.Committee{}
	case entities.CommitteeValidators:
		model = indexermodels.CommitteeValidator{}
	case entities.CommitteePayments:
		model = indexermodels.CommitteePayment{}
	case entities.PollSnapshots:
		model = indexermodels.PollSnapshot{}
	case entities.Supply:
		model = indexermodels.Supply{}
	default:
		return nil
	}

	return extractSchemaFromStruct(model)
}

// extractSchemaFromStruct uses reflection to extract field names and types from a struct.
// It reads the `ch` struct tag to get the ClickHouse column name and returns a map of column names to type strings.
func extractSchemaFromStruct(s interface{}) map[string]string {
	t := reflect.TypeOf(s)
	properties := make(map[string]string)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		chTag := field.Tag.Get("ch")

		// Skip fields without a `ch` tag
		if chTag == "" {
			continue
		}

		// Get the field type as a string
		fieldType := getFieldTypeString(field.Type)
		properties[chTag] = fieldType
	}

	return properties
}

// getFieldTypeString converts a reflect.Type to a user-friendly type string for the UI.
func getFieldTypeString(t reflect.Type) string {
	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int64"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "uint64"
	case reflect.Float32, reflect.Float64:
		return "float64"
	default:
		// For complex types like time.Time, return the type name
		if t.String() == "time.Time" {
			return "datetime"
		}
		return t.String()
	}
}

// writeError writes an error response
func (c *Controller) writeError(w http.ResponseWriter, statusCode int, message string) {
	c.writeJSON(w, statusCode, map[string]string{"error": message})
}

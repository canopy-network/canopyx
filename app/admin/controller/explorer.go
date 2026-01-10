package controller

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/db/entities"
	"github.com/go-jose/go-jose/v4/json"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// Table names allowed for explorer queries
const (
	ExplorerTableBlocks = "blocks"
	ExplorerTableTxs    = "txs"
)

// Default and maximum limits for data queries
const (
	DefaultExplorerLimit = 50
	MaxExplorerLimit     = 100
)

// ColumnInfo represents a column schema information
type ColumnInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// SchemaResponse represents the response for schema endpoint
type SchemaResponse struct {
	Columns []ColumnInfo `json:"columns"`
}

// DataResponse represents the response for data endpoint
type DataResponse struct {
	Rows    []map[string]interface{} `json:"rows"`
	Total   int64                    `json:"total"`
	HasMore bool                     `json:"hasMore"`
}

// ExplorerService encapsulates explorer-related business logic
type ExplorerService interface {
	GetTableSchema(ctx context.Context, chainID, tableName string) ([]ColumnInfo, error)
	GetTableData(ctx context.Context, chainID, tableName string, limit, offset int, fromHeight, toHeight *uint64) (*DataResponse, error)
}

// explorerService is the concrete implementation
type explorerService struct {
	controller *Controller
	logger     *zap.Logger
}

// NewExplorerService creates a new explorer service instance
func NewExplorerService(c *Controller) ExplorerService {
	return &explorerService{
		controller: c,
		logger:     c.App.Logger,
	}
}

// validateTableName ensures the table name is valid to prevent SQL injection
func (s *explorerService) validateTableName(tableName string) error {
	switch tableName {
	case ExplorerTableBlocks, ExplorerTableTxs:
		return nil
	default:
		return fmt.Errorf("invalid table name: %s", tableName)
	}
}

// GetTableSchema retrieves the schema for a given table
func (s *explorerService) GetTableSchema(ctx context.Context, chainID, tableName string) ([]ColumnInfo, error) {
	// Validate table name
	if err := s.validateTableName(tableName); err != nil {
		return nil, err
	}

	// Parse chain ID
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return nil, fmt.Errorf("invalid chain_id: %w", parseErr)
	}

	// Get GlobalDB configured for this chain
	chainDB := s.controller.App.GetGlobalDBForChain(chainIDUint)

	// Get table schema from chain store
	dbColumns, err := chainDB.GetTableSchema(ctx, tableName)
	if err != nil {
		return nil, fmt.Errorf("error querying schema: %w", err)
	}

	// Convert to ColumnInfo format
	var columns []ColumnInfo
	for _, col := range dbColumns {
		columns = append(columns, ColumnInfo{
			Name: col.Name,
			Type: col.Type,
		})
	}

	if len(columns) == 0 {
		return nil, fmt.Errorf("table %s not found or has no columns", tableName)
	}

	return columns, nil
}

// GetTableData retrieves paginated data from a given table
func (s *explorerService) GetTableData(ctx context.Context, chainID, tableName string, limit, offset int, fromHeight, toHeight *uint64) (*DataResponse, error) {
	// Validate table name
	if err := s.validateTableName(tableName); err != nil {
		return nil, err
	}

	// Validate and adjust limit
	if limit <= 0 {
		limit = DefaultExplorerLimit
	}
	if limit > MaxExplorerLimit {
		limit = MaxExplorerLimit
	}

	// Validate offset
	if offset < 0 {
		offset = 0
	}

	// Parse chain ID
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return nil, fmt.Errorf("invalid chain_id: %w", parseErr)
	}

	// Get GlobalDB configured for this chain
	chainDB := s.controller.App.GetGlobalDBForChain(chainIDUint)

	// Get table data from chain store
	results, total, hasMore, err := chainDB.GetTableDataPaginated(ctx, tableName, limit, offset, fromHeight, toHeight)
	if err != nil {
		return nil, fmt.Errorf("error querying data: %w", err)
	}

	return &DataResponse{
		Rows:    results,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

// HandleExplorerSchema handles GET /api/chains/{id}/explorer/schema
func (c *Controller) HandleExplorerSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	chainID := vars["id"]

	// Get table parameter
	tableName := r.URL.Query().Get("table")
	if tableName == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "table parameter is required"})
		return
	}

	// Create service instance
	service := NewExplorerService(c)

	// Get schema
	columns, err := service.GetTableSchema(ctx, chainID, tableName)
	if err != nil {
		// Determine appropriate status code
		statusCode := http.StatusInternalServerError
		if strings.Contains(err.Error(), "chain not found") {
			statusCode = http.StatusNotFound
		} else if strings.Contains(err.Error(), "invalid table") {
			statusCode = http.StatusBadRequest
		} else if strings.Contains(err.Error(), "table") && strings.Contains(err.Error(), "not found") {
			statusCode = http.StatusNotFound
		}

		c.App.Logger.Error("failed to get table schema",
			zap.String("chain_id", chainID),
			zap.String("table", tableName),
			zap.Error(err))

		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Return schema
	response := SchemaResponse{Columns: columns}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// HandleGlobalExplorerSchema handles GET /api/explorer/schema
// Returns schema for a table without requiring chain_id context.
// Query Parameters:
//   - table: Table name (required) - e.g., "blocks", "txs"
func (c *Controller) HandleGlobalExplorerSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get table parameter
	tableName := r.URL.Query().Get("table")
	if tableName == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "table parameter is required"})
		return
	}

	// Get GlobalDB (schema is the same for all chains, just need a DB instance)
	globalDB := c.App.GetGlobalDBForChain(0)

	// Get table schema
	dbColumns, err := globalDB.GetTableSchema(ctx, tableName)
	if err != nil {
		c.App.Logger.Error("failed to get table schema",
			zap.String("table", tableName),
			zap.Error(err))

		statusCode := http.StatusInternalServerError
		if strings.Contains(err.Error(), "table") && strings.Contains(err.Error(), "not found") {
			statusCode = http.StatusNotFound
		}
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Convert to ColumnInfo format
	var columns []ColumnInfo
	for _, col := range dbColumns {
		columns = append(columns, ColumnInfo{
			Name: col.Name,
			Type: col.Type,
		})
	}

	if len(columns) == 0 {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("table %s not found or has no columns", tableName)})
		return
	}

	// Return schema
	response := SchemaResponse{Columns: columns}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// HandleGlobalExplorerEntityQuery handles GET /api/explorer/entity/{entity}
// Returns entity data with optional chain_ids filter. When no chain_ids provided,
// queries all chains without PREWHERE filtering.
//
// Path Parameters:
//   - entity: Entity name (e.g., "blocks", "accounts", "txs")
//
// Query Parameters:
//   - chain_ids: Comma-separated list of chain IDs to filter (optional)
//   - limit: Number of records to return (default: 50, max: 1000)
//   - cursor: Pagination cursor (height value to start from)
//   - sort: Sort order - "asc" or "desc" (default: "desc")
func (c *Controller) HandleGlobalExplorerEntityQuery(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	entityName := vars["entity"]

	// Parse and validate entity
	entity, err := entities.FromString(entityName)
	if err != nil {
		c.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid entity: %v", err))
		return
	}

	// Parse chain_ids filter (optional)
	var chainIDs []uint64
	if chainIDsParam := r.URL.Query().Get("chain_ids"); chainIDsParam != "" {
		for _, idStr := range strings.Split(chainIDsParam, ",") {
			idStr = strings.TrimSpace(idStr)
			if idStr == "" {
				continue
			}
			id, err := strconv.ParseUint(idStr, 10, 64)
			if err != nil {
				c.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid chain_id: %s", idStr))
				return
			}
			chainIDs = append(chainIDs, id)
		}
	}

	// Parse pagination parameters
	limit := DefaultExplorerLimit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit <= 0 {
			c.writeError(w, http.StatusBadRequest, "invalid limit parameter")
			return
		}
		limit = parsedLimit
		if limit > MaxExplorerLimit {
			limit = MaxExplorerLimit
		}
	}

	var cursor uint64
	if cursorStr := r.URL.Query().Get("cursor"); cursorStr != "" {
		parsedCursor, err := strconv.ParseUint(cursorStr, 10, 64)
		if err != nil {
			c.writeError(w, http.StatusBadRequest, "invalid cursor parameter")
			return
		}
		cursor = parsedCursor
	}

	sortDesc := true
	if sortStr := r.URL.Query().Get("sort"); sortStr != "" {
		switch strings.ToLower(sortStr) {
		case "asc":
			sortDesc = false
		case "desc":
			sortDesc = true
		default:
			c.writeError(w, http.StatusBadRequest, "invalid sort order: must be 'asc' or 'desc'")
			return
		}
	}

	// Get GlobalDB
	globalDB := c.App.GetGlobalDBForChain(0)
	tableName := entity.TableName()

	// Execute query
	data, nextCursor, err := c.queryGlobalEntityData(ctx, globalDB, tableName, chainIDs, limit, cursor, sortDesc)
	if err != nil {
		c.App.Logger.Error("Failed to query global entity data",
			zap.String("entity", entityName),
			zap.Uint64s("chain_ids", chainIDs),
			zap.Error(err),
		)
		c.writeError(w, http.StatusInternalServerError, "failed to query entity data")
		return
	}

	// Build response
	resp := struct {
		Data       []map[string]interface{} `json:"data"`
		Limit      int                      `json:"limit"`
		NextCursor *uint64                  `json:"next_cursor"`
	}{
		Data:       data,
		Limit:      limit,
		NextCursor: nextCursor,
	}

	c.writeJSON(w, http.StatusOK, resp)
}

// queryGlobalEntityData executes a global entity query with optional chain_ids filter.
// When chainIDs is empty, queries all chains without PREWHERE.
// When chainIDs is provided, uses WHERE chain_id IN (...) filtering.
// Uses dynamic map-based scanning to handle all database columns regardless of model struct.
func (c *Controller) queryGlobalEntityData(
	ctx context.Context,
	store db.GlobalStore,
	tableName string,
	chainIDs []uint64,
	limit int,
	cursor uint64,
	sortDesc bool,
) ([]map[string]interface{}, *uint64, error) {
	// Request limit+1 to detect if there are more results
	queryLimit := limit + 1

	var whereClauses []string
	var args []interface{}

	// Add chain_id filter if provided (no PREWHERE, just WHERE)
	if len(chainIDs) > 0 {
		whereClauses = append(whereClauses, "chain_id IN ?")
		args = append(args, chainIDs)
	}

	// Add cursor condition if provided
	if cursor > 0 {
		if sortDesc {
			whereClauses = append(whereClauses, "height < ?")
		} else {
			whereClauses = append(whereClauses, "height > ?")
		}
		args = append(args, cursor)
	}

	// Build WHERE clause
	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Build sort clause
	orderClause := "ORDER BY height DESC"
	if !sortDesc {
		orderClause = "ORDER BY height ASC"
	}

	// First get column types from schema to create properly typed pointers
	// and determine the correct ordering column and chain ID column
	schema, schemaErr := store.GetTableSchema(ctx, tableName)
	if schemaErr != nil {
		return nil, nil, fmt.Errorf("get schema: %w", schemaErr)
	}
	typeMap := make(map[string]string)
	hasHeight := false
	hasChainID := false
	hasSourceChainID := false
	hasSnapshotDate := false
	for _, col := range schema {
		typeMap[col.Name] = col.Type
		switch col.Name {
		case "height":
			hasHeight = true
		case "chain_id":
			hasChainID = true
		case "source_chain_id":
			hasSourceChainID = true
		case "snapshot_date":
			hasSnapshotDate = true
		}
	}

	// Determine ordering column - priority: height > snapshot_date > chain_id > source_chain_id
	orderColumn := ""
	if hasHeight {
		orderColumn = "height"
	} else if hasSnapshotDate {
		orderColumn = "snapshot_date"
	} else if hasChainID {
		orderColumn = "chain_id"
	} else if hasSourceChainID {
		orderColumn = "source_chain_id"
	}

	// Determine chain ID column for filtering
	chainIDColumn := ""
	if hasChainID {
		chainIDColumn = "chain_id"
	} else if hasSourceChainID {
		chainIDColumn = "source_chain_id"
	}

	// Update ordering and cursor clauses to use the correct column
	if orderColumn != "" && orderColumn != "height" {
		if sortDesc {
			orderClause = fmt.Sprintf("ORDER BY %s DESC", orderColumn)
		} else {
			orderClause = fmt.Sprintf("ORDER BY %s ASC", orderColumn)
		}
		// Remove height-based cursor condition if table doesn't have height
		if cursor > 0 {
			// For non-height tables, we can't use height-based pagination
			// Just skip cursor-based pagination
			whereClauses = nil
			args = nil
			if len(chainIDs) > 0 && chainIDColumn != "" {
				whereClauses = append(whereClauses, fmt.Sprintf("%s IN ?", chainIDColumn))
				args = append(args, chainIDs)
			}
			whereClause = ""
			if len(whereClauses) > 0 {
				whereClause = "WHERE " + strings.Join(whereClauses, " AND ")
			}
		}
	}

	// Update chain ID filter to use the correct column if original whereClauses used chain_id
	if chainIDColumn != "chain_id" && chainIDColumn != "" && len(chainIDs) > 0 {
		// Rebuild whereClauses with correct chain ID column
		whereClauses = nil
		args = nil
		if cursor > 0 && hasHeight {
			if sortDesc {
				whereClauses = append(whereClauses, "height < ?")
			} else {
				whereClauses = append(whereClauses, "height > ?")
			}
			args = append(args, cursor)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("%s IN ?", chainIDColumn))
		args = append(args, chainIDs)
		whereClause = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Build final query (no PREWHERE for global queries)
	query := fmt.Sprintf(`
		SELECT *
		FROM "%s"."%s" FINAL
		%s
		%s
		LIMIT ?
	`, store.DatabaseName(), tableName, whereClause, orderClause)

	args = append(args, queryLimit)

	// Use dynamic map-based scanning via GetConnection().Query()
	rows, err := store.GetConnection().Query(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("query failed: %w", err)
	}

	defer func() { _ = rows.Close() }()

	// Get column names
	columnNames := rows.Columns()

	// Read all rows into maps
	var rawResults []map[string]interface{}
	for rows.Next() {
		// Create typed pointers based on column types
		valuePtrs := make([]interface{}, len(columnNames))
		for i, colName := range columnNames {
			colType := typeMap[colName]
			valuePtrs[i] = createTypedPointer(colType)
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, nil, fmt.Errorf("scan failed: %w", err)
		}

		// Build map with actual values
		rowMap := make(map[string]interface{})
		for i, colName := range columnNames {
			rowMap[colName] = derefPointer(valuePtrs[i])
		}
		rawResults = append(rawResults, rowMap)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("rows iteration error: %w", err)
	}

	// Determine if there are more results
	var nextCursor *uint64
	hasMore := len(rawResults) > limit
	if hasMore {
		rawResults = rawResults[:limit]

		if len(rawResults) > 0 {
			if heightVal, ok := rawResults[len(rawResults)-1]["height"]; ok {
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

// HandleExplorerData handles GET /api/chains/{id}/explorer/data
func (c *Controller) HandleExplorerData(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	chainID := vars["id"]

	// Parse query parameters
	query := r.URL.Query()

	// Get table parameter (required)
	tableName := query.Get("table")
	if tableName == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "table parameter is required"})
		return
	}

	// Parse limit parameter
	limit := DefaultExplorerLimit
	if limitStr := query.Get("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit < 0 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid limit parameter"})
			return
		}
		limit = parsedLimit
		if limit > MaxExplorerLimit {
			limit = MaxExplorerLimit
		}
	}

	// Parse offset parameter
	offset := 0
	if offsetStr := query.Get("offset"); offsetStr != "" {
		parsedOffset, err := strconv.Atoi(offsetStr)
		if err != nil || parsedOffset < 0 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid offset parameter"})
			return
		}
		offset = parsedOffset
	}

	// Parse from height parameter
	var fromHeight *uint64
	if fromStr := query.Get("from"); fromStr != "" {
		parsedFrom, err := strconv.ParseUint(fromStr, 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid from parameter"})
			return
		}
		fromHeight = &parsedFrom
	}

	// Parse to height parameter
	var toHeight *uint64
	if toStr := query.Get("to"); toStr != "" {
		parsedTo, err := strconv.ParseUint(toStr, 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid to parameter"})
			return
		}
		toHeight = &parsedTo
	}

	// Validate height range if both are provided
	if fromHeight != nil && toHeight != nil && *fromHeight > *toHeight {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "from height cannot be greater than to height"})
		return
	}

	// Create service instance
	service := NewExplorerService(c)

	// Get data
	response, err := service.GetTableData(ctx, chainID, tableName, limit, offset, fromHeight, toHeight)
	if err != nil {
		// Determine appropriate status code
		statusCode := http.StatusInternalServerError
		if strings.Contains(err.Error(), "chain not found") {
			statusCode = http.StatusNotFound
		} else if strings.Contains(err.Error(), "invalid table") {
			statusCode = http.StatusBadRequest
		}

		c.App.Logger.Error("failed to get table data",
			zap.String("chain_id", chainID),
			zap.String("table", tableName),
			zap.Int("limit", limit),
			zap.Int("offset", offset),
			zap.Error(err))

		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Return data
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// createTypedPointer creates a properly typed pointer for ClickHouse scanning based on column type
func createTypedPointer(colType string) interface{} {
	// Handle common ClickHouse types
	switch {
	case colType == "Bool":
		return new(bool)
	case colType == "UInt8":
		return new(uint8)
	case colType == "UInt16":
		return new(uint16)
	case colType == "UInt32":
		return new(uint32)
	case colType == "UInt64":
		return new(uint64)
	case colType == "Int8":
		return new(int8)
	case colType == "Int16":
		return new(int16)
	case colType == "Int32":
		return new(int32)
	case colType == "Int64":
		return new(int64)
	case colType == "Float32":
		return new(float32)
	case colType == "Float64":
		return new(float64)
	case colType == "String":
		return new(string)
	case colType == "Date":
		return new(time.Time) // Date types scan to time.Time
	case len(colType) >= 10 && colType[:10] == "DateTime64":
		return new(time.Time) // DateTime64 types scan to time.Time
	case len(colType) >= 8 && colType[:8] == "DateTime":
		return new(time.Time) // DateTime types scan to time.Time
	case len(colType) > 9 && colType[:9] == "Nullable(":
		// Handle nullable types - use pointer to base type
		innerType := colType[9 : len(colType)-1]
		return createTypedPointer(innerType)
	case len(colType) > 16 && colType[:16] == "LowCardinality(":
		// Handle LowCardinality - use underlying type
		innerType := colType[16 : len(colType)-1]
		return createTypedPointer(innerType)
	default:
		// Fallback to string for unknown types
		return new(string)
	}
}

// derefPointer dereferences a typed pointer to get the actual value
func derefPointer(ptr interface{}) interface{} {
	switch v := ptr.(type) {
	case *bool:
		return *v
	case *uint8:
		return *v
	case *uint16:
		return *v
	case *uint32:
		return *v
	case *uint64:
		return *v
	case *int8:
		return *v
	case *int16:
		return *v
	case *int32:
		return *v
	case *int64:
		return *v
	case *float32:
		return *v
	case *float64:
		return *v
	case *string:
		return *v
	case *time.Time:
		return *v
	case *interface{}:
		return *v
	default:
		return ptr
	}
}

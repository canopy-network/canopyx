package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-jose/go-jose/v4/json"
	"github.com/gorilla/mux"
	"github.com/uptrace/go-clickhouse/ch"
	"go.uber.org/zap"
)

// Table names allowed for explorer queries
const (
	ExplorerTableBlocks  = "blocks"
	ExplorerTableTxs     = "txs"
	ExplorerTableTxsRaw  = "txs_raw"
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
	case ExplorerTableBlocks, ExplorerTableTxs, ExplorerTableTxsRaw:
		return nil
	default:
		return fmt.Errorf("invalid table name: %s", tableName)
	}
}

// getChainDB retrieves the chain database for the given chainID
func (s *explorerService) getChainDB(ctx context.Context, chainID string) (*ch.DB, string, error) {
	// First check if the chain exists in admin DB
	_, err := s.controller.App.AdminDB.GetChain(ctx, chainID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", fmt.Errorf("chain not found: %s", chainID)
		}
		return nil, "", fmt.Errorf("error fetching chain: %w", err)
	}

	// Get or create the chain database connection
	chainDB, err := s.controller.App.NewChainDb(ctx, chainID)
	if err != nil {
		return nil, "", fmt.Errorf("error accessing chain database: %w", err)
	}

	// ChainDB implements ChainStore interface, not *ch.DB directly
	// We need to access the database name from the ChainStore interface
	dbName := chainDB.DatabaseName()

	// Use the AdminDB connection with the chain's database name
	// This is safe because all databases are on the same ClickHouse server
	return s.controller.App.AdminDB.Db, dbName, nil
}

// GetTableSchema retrieves the schema for a given table
func (s *explorerService) GetTableSchema(ctx context.Context, chainID, tableName string) ([]ColumnInfo, error) {
	// Validate table name
	if err := s.validateTableName(tableName); err != nil {
		return nil, err
	}

	// Get chain database
	chainDB, err := s.controller.App.NewChainDb(ctx, chainID)
	if err != nil {
		return nil, fmt.Errorf("error accessing chain database: %w", err)
	}

	// Get database name for the chain
	dbName := chainDB.DatabaseName()

	// Query to get column information from ClickHouse system tables
	query := `
		SELECT
			name,
			type
		FROM system.columns
		WHERE database = ? AND table = ?
		ORDER BY position
	`

	rows, err := s.controller.App.AdminDB.Db.QueryContext(ctx, query, dbName, tableName)
	if err != nil {
		return nil, fmt.Errorf("error querying schema: %w", err)
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var col ColumnInfo
		if err := rows.Scan(&col.Name, &col.Type); err != nil {
			return nil, fmt.Errorf("error scanning column info: %w", err)
		}
		columns = append(columns, col)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
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

	// Get chain database
	chainDB, err := s.controller.App.NewChainDb(ctx, chainID)
	if err != nil {
		return nil, fmt.Errorf("error accessing chain database: %w", err)
	}

	dbName := chainDB.DatabaseName()

	// Build WHERE conditions for height filtering
	conditions := []string{}
	args := []interface{}{}

	if fromHeight != nil {
		conditions = append(conditions, "height >= ?")
		args = append(args, *fromHeight)
	}
	if toHeight != nil {
		conditions = append(conditions, "height <= ?")
		args = append(args, *toHeight)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total rows (with filters applied)
	countQuery := fmt.Sprintf(`
		SELECT count(*)
		FROM "%s"."%s"
		%s
	`, dbName, tableName, whereClause)

	var total int64
	if err := s.controller.App.AdminDB.Db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		s.logger.Warn("failed to count rows", zap.String("chain_id", chainID), zap.String("table", tableName), zap.Error(err))
		// Continue without total count
		total = -1
	}

	// Query data with pagination
	dataQuery := fmt.Sprintf(`
		SELECT *
		FROM "%s"."%s"
		%s
		ORDER BY height DESC
		LIMIT ? OFFSET ?
	`, dbName, tableName, whereClause)

	args = append(args, limit, offset)

	rows, err := s.controller.App.AdminDB.Db.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("error querying data: %w", err)
	}
	defer rows.Close()

	// Get column names from the result set
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("error getting columns: %w", err)
	}

	// Prepare result slice
	results := make([]map[string]interface{}, 0, limit)

	// Create a slice of interface{} to hold pointers to column values
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))

	for rows.Next() {
		// Reset value pointers for each row
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}

		// Create a map for this row
		rowMap := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			// Handle NULL values
			if val == nil {
				rowMap[col] = nil
			} else {
				// Convert []byte to string for better JSON serialization
				if b, ok := val.([]byte); ok {
					rowMap[col] = string(b)
				} else {
					rowMap[col] = val
				}
			}
		}
		results = append(results, rowMap)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	// Determine if there are more rows
	hasMore := false
	if total > 0 {
		hasMore = int64(offset+len(results)) < total
	} else if len(results) == limit {
		// If we got exactly the limit, there might be more
		hasMore = true
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
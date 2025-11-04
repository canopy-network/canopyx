package controller

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

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

	// Get chain database
	chainDB, err := s.controller.App.NewChainDb(ctx, chainID)
	if err != nil {
		return nil, fmt.Errorf("error accessing chain database: %w", err)
	}

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

	// Get chain database
	chainDB, err := s.controller.App.NewChainDb(ctx, chainID)
	if err != nil {
		return nil, fmt.Errorf("error accessing chain database: %w", err)
	}

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

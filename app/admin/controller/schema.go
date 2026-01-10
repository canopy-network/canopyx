package controller

import (
    "context"
    "net/http"
    "strconv"
    "strings"

    "github.com/canopy-network/canopyx/pkg/db/global"
    "github.com/canopy-network/canopyx/pkg/db/entities"
    "github.com/go-jose/go-jose/v4/json"
    "github.com/gorilla/mux"
    "go.uber.org/zap"
)

// TableSchemaResponse represents the response structure for schema introspection
type TableSchemaResponse struct {
    Columns []global.Column `json:"columns"`
}

// HandleSchema returns the schema for the specified table
// GET /api/admin/chains/{id}/schema?table=blocks
func (c *Controller) HandleSchema(w http.ResponseWriter, r *http.Request) {
    chainID := mux.Vars(r)["id"]
    if chainID == "" {
        http.Error(w, "missing chain id", http.StatusBadRequest)
        return
    }

    tableName := r.URL.Query().Get("table")
    if tableName == "" {
        http.Error(w, "missing table parameter", http.StatusBadRequest)
        return
    }

    // Validate and normalize table name using entities package
    // Strip _staging suffix if present to get base entity name
    baseTableName := strings.TrimSuffix(tableName, entities.StagingSuffix)

    // Validate entity name
    _, err := entities.FromString(baseTableName)
    if err != nil {
        http.Error(w, "invalid table name: "+err.Error(), http.StatusBadRequest)
        return
    }

    // Use the actual table name (with _staging suffix if it was provided)
    actualTable := tableName

    // Check if this is a deleted chain query (for logging context)
    isDeleted := r.URL.Query().Get("deleted") == "true"
    _ = isDeleted // Unused but kept for logging context

    ctx := context.Background()

    // Parse chain ID
    chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
    if parseErr != nil {
        http.Error(w, "invalid chain_id", http.StatusBadRequest)
        return
    }

    // Get GlobalDB configured for this chain
    store := c.App.GetGlobalDBForChain(chainIDUint)

    // Get column information using DESCRIBE TABLE
    columns, err := store.DescribeTable(ctx, actualTable)
    if err != nil {
        // Check if error is due to table not existing
        if strings.Contains(err.Error(), "doesn't exist") {
            http.Error(w, "table not found", http.StatusNotFound)
            return
        }
        c.App.Logger.Error("failed to describe table", zap.Error(err))
        http.Error(w, "failed to describe table", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    _ = json.NewEncoder(w).Encode(TableSchemaResponse{
        Columns: columns,
    })
}

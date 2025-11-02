package controller

import (
	"context"
	"net/http"
	"strings"

	chainstore "github.com/canopy-network/canopyx/pkg/db/chain"
	"github.com/go-jose/go-jose/v4/json"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// TableSchemaResponse represents the response structure for schema introspection
type TableSchemaResponse struct {
	Columns []chainstore.Column `json:"columns"`
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

	// Validate table name to prevent SQL injection
	// Accept both underscore (database names) and dash (API route names)
	validTables := map[string]string{
		"blocks":          "blocks",
		"block_summaries": "block_summaries",
		"block-summaries": "block_summaries", // API route format
		"transactions":    "txs",
		"accounts":        "accounts",
		"events":          "events",
		"pools":           "pools",
		"orders":          "orders",
		"dex_prices":      "dex_prices",
		"dex-prices":      "dex_prices", // API route format
	}

	actualTable, ok := validTables[tableName]
	if !ok {
		http.Error(w, "invalid table name. Must be one of: blocks, block-summaries, transactions, accounts, events, pools, orders, dex-prices", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	store, ok := c.App.LoadChainStore(ctx, chainID)
	if !ok {
		http.Error(w, "chain not indexed", http.StatusNotFound)
		return
	}

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

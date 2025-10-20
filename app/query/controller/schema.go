package controller

import (
	"context"
	"net/http"
	"strings"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/gorilla/mux"
)

// SchemaResponse represents the response structure for schema introspection
type SchemaResponse struct {
	Columns []db.Column `json:"columns"`
}

// HandleSchema returns the schema for the specified table
func (c *Controller) HandleSchema(w http.ResponseWriter, r *http.Request) {
	chainID := mux.Vars(r)["id"]
	if chainID == "" {
		writeError(w, http.StatusBadRequest, "missing chain id")
		return
	}

	tableName := r.URL.Query().Get("table")
	if tableName == "" {
		writeError(w, http.StatusBadRequest, "missing table parameter")
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
		writeError(w, http.StatusBadRequest, "invalid table name. Must be one of: blocks, block-summaries, transactions, accounts, events, pools, orders, dex-prices")
		return
	}

	ctx := context.Background()

	store, ok := c.App.LoadChainStore(ctx, chainID)
	if !ok {
		writeError(w, http.StatusNotFound, "chain not indexed")
		return
	}

	// Get column information using DESCRIBE TABLE
	columns, err := store.DescribeTable(ctx, actualTable)
	if err != nil {
		// Check if error is due to table not existing
		if strings.Contains(err.Error(), "doesn't exist") {
			writeError(w, http.StatusNotFound, "table not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to describe table")
		return
	}

	writeJSON(w, http.StatusOK, SchemaResponse{
		Columns: columns,
	})
}

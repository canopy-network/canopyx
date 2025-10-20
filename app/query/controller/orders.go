package controller

import (
	"context"
	"net/http"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/gorilla/mux"
)

// HandleOrders returns order data with optional status filtering.
// Supports optional query parameter: status (e.g., ?status=open)
// Valid status values: "open", "filled", "cancelled", "expired"
// GET /chains/{id}/orders?status=<status>&cursor=<height>&limit=<n>&sort=<asc|desc>
func (c *Controller) HandleOrders(w http.ResponseWriter, r *http.Request) {
	chainID := mux.Vars(r)["id"]
	if chainID == "" {
		writeError(w, http.StatusBadRequest, "missing chain id")
		return
	}

	page, err := parsePageSpec(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := context.Background()

	store, ok := c.App.LoadChainStore(ctx, chainID)
	if !ok {
		writeError(w, http.StatusNotFound, "chain not indexed")
		return
	}

	// Convert SortOrder to bool (true = DESC, false = ASC)
	sortDesc := page.Sort == SortOrderDesc

	// Get optional status filter from query parameter
	status := r.URL.Query().Get("status")

	// Query with limit+1 to detect if there are more pages
	rows, err := store.QueryOrders(ctx, page.Cursor, page.Limit+1, sortDesc, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	nextCursor := (*uint64)(nil)
	if len(rows) > page.Limit {
		rows = rows[:page.Limit]
		cursor := rows[len(rows)-1].Height
		nextCursor = &cursor
	}

	writeJSON(w, http.StatusOK, pagedResponse[indexer.Order]{
		Data:       rows,
		Limit:      page.Limit,
		NextCursor: nextCursor,
	})
}

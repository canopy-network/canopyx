package controller

import (
	"context"
	"net/http"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/gorilla/mux"
)

// HandleEvents returns event data with optional event type filtering.
// Supports optional query parameter: type (e.g., ?type=transfer)
func (c *Controller) HandleEvents(w http.ResponseWriter, r *http.Request) {
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

	// Get optional event type filter from query parameter
	eventType := r.URL.Query().Get("type")

	// Query with limit+1 to detect if there are more pages
	rows, err := store.QueryEventsWithFilter(ctx, page.Cursor, page.Limit+1, sortDesc, eventType)
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

	writeJSON(w, http.StatusOK, pagedResponse[indexer.Event]{
		Data:       rows,
		Limit:      page.Limit,
		NextCursor: nextCursor,
	})
}

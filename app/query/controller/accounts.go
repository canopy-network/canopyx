package controller

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/gorilla/mux"
)

// HandleAccounts returns paginated account data ordered by height.
// Supports cursor-based pagination using the limit+1 pattern.
// Query parameters:
//   - cursor: height to start from (exclusive)
//   - limit: max number of results (default/max defined in parsePageSpec)
//   - sort: "asc" or "desc" (default "desc")
func (c *Controller) HandleAccounts(w http.ResponseWriter, r *http.Request) {
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

	// Query with limit+1 to detect if there are more pages
	rows, err := store.QueryAccounts(ctx, page.Cursor, page.Limit+1, sortDesc)
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

	writeJSON(w, http.StatusOK, pagedResponse[indexer.Account]{
		Data:       rows,
		Limit:      page.Limit,
		NextCursor: nextCursor,
	})
}

// HandleAccountByAddress returns a single account by address.
// Query parameters:
//   - height (optional): if specified, returns account state at or before that height
//
// Returns 404 if account not found, 400 for invalid parameters.
func (c *Controller) HandleAccountByAddress(w http.ResponseWriter, r *http.Request) {
	chainID := mux.Vars(r)["id"]
	address := mux.Vars(r)["address"]

	if chainID == "" || address == "" {
		writeError(w, http.StatusBadRequest, "missing chain id or address")
		return
	}

	// Parse optional height parameter
	var heightPtr *uint64
	if heightStr := r.URL.Query().Get("height"); heightStr != "" {
		height, err := strconv.ParseUint(heightStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid height parameter")
			return
		}
		heightPtr = &height
	}

	ctx := context.Background()

	store, ok := c.App.LoadChainStore(ctx, chainID)
	if !ok {
		writeError(w, http.StatusNotFound, "chain not indexed")
		return
	}

	account, err := store.GetAccountByAddress(ctx, address, heightPtr)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, "query failed")
		}
		return
	}

	writeJSON(w, http.StatusOK, account)
}
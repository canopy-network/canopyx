package controller

import (
	"context"
	"net/http"
	"strconv"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/gorilla/mux"
)

// HandleDexPrices returns DEX price data with optional chain pair filtering.
// Supports optional query parameters: local (local chain ID), remote (remote chain ID)
// Both local and remote must be provided together to filter by chain pair.
// GET /chains/{id}/dex-prices?local=<id>&remote=<id>&cursor=<height>&limit=<n>&sort=<asc|desc>
func (c *Controller) HandleDexPrices(w http.ResponseWriter, r *http.Request) {
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

	// Get optional chain pair filters from query parameters
	var localChainID, remoteChainID uint64
	if localStr := r.URL.Query().Get("local"); localStr != "" {
		local, err := strconv.ParseUint(localStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid local chain id")
			return
		}
		localChainID = local
	}

	if remoteStr := r.URL.Query().Get("remote"); remoteStr != "" {
		remote, err := strconv.ParseUint(remoteStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid remote chain id")
			return
		}
		remoteChainID = remote
	}

	// Query with limit+1 to detect if there are more pages
	rows, err := store.QueryDexPrices(ctx, page.Cursor, page.Limit+1, sortDesc, localChainID, remoteChainID)
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

	writeJSON(w, http.StatusOK, pagedResponse[indexer.DexPrice]{
		Data:       rows,
		Limit:      page.Limit,
		NextCursor: nextCursor,
	})
}

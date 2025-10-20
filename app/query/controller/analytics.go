package controller

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

// HandleDexVolume returns 24-hour DEX trading volume statistics.
// Endpoint: GET /chains/{id}/analytics/dex-volume
func (c *Controller) HandleDexVolume(w http.ResponseWriter, r *http.Request) {
	chainID := mux.Vars(r)["id"]
	if chainID == "" {
		writeError(w, http.StatusBadRequest, "missing chain id")
		return
	}

	ctx := context.Background()

	store, ok := c.App.LoadChainStore(ctx, chainID)
	if !ok {
		writeError(w, http.StatusNotFound, "chain not indexed")
		return
	}

	stats, err := store.GetDexVolume24h(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	// Return simple data array (no pagination for aggregated data)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": stats,
	})
}

// HandleOrderBookDepth returns order book depth aggregated by price levels.
// Endpoint: GET /chains/{id}/analytics/order-depth?committee=<id>&limit=<n>
func (c *Controller) HandleOrderBookDepth(w http.ResponseWriter, r *http.Request) {
	chainID := mux.Vars(r)["id"]
	if chainID == "" {
		writeError(w, http.StatusBadRequest, "missing chain id")
		return
	}

	// Parse committee (required)
	committeeStr := r.URL.Query().Get("committee")
	if committeeStr == "" {
		writeError(w, http.StatusBadRequest, "missing committee parameter")
		return
	}

	committee, err := strconv.ParseUint(committeeStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid committee value")
		return
	}

	// Parse limit (optional, default 20, max 100)
	limit := 20
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit <= 0 {
			writeError(w, http.StatusBadRequest, "invalid limit value")
			return
		}
		if parsedLimit > 100 {
			parsedLimit = 100
		}
		limit = parsedLimit
	}

	ctx := context.Background()

	store, ok := c.App.LoadChainStore(ctx, chainID)
	if !ok {
		writeError(w, http.StatusNotFound, "chain not indexed")
		return
	}

	levels, err := store.GetOrderBookDepth(ctx, committee, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": levels,
	})
}

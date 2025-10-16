package controller

import (
	"context"
	"net/http"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/gorilla/mux"
)

type pagedResponse[T any] struct {
	Data       []T     `json:"data"`
	Limit      int     `json:"limit"`
	NextCursor *uint64 `json:"next_cursor,omitempty"`
}

func (c *Controller) HandleBlocks(w http.ResponseWriter, r *http.Request) {
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

	// Query with limit+1 to detect if there are more pages
	rows, err := store.QueryBlocks(ctx, page.Cursor, page.Limit+1)
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

	writeJSON(w, http.StatusOK, pagedResponse[indexer.Block]{
		Data:       rows,
		Limit:      page.Limit,
		NextCursor: nextCursor,
	})
}

// HandleBlockSummaries returns block summary data
func (c *Controller) HandleBlockSummaries(w http.ResponseWriter, r *http.Request) {
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

	// Query with limit+1 to detect if there are more pages
	rows, err := store.QueryBlockSummaries(ctx, page.Cursor, page.Limit+1)
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

	writeJSON(w, http.StatusOK, pagedResponse[indexer.BlockSummary]{
		Data:       rows,
		Limit:      page.Limit,
		NextCursor: nextCursor,
	})
}

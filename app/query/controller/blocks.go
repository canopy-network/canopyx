package controller

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

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

	// Convert SortOrder to bool (true = DESC, false = ASC)
	sortDesc := page.Sort == SortOrderDesc

	// Query with limit+1 to detect if there are more pages
	rows, err := store.QueryBlocks(ctx, page.Cursor, page.Limit+1, sortDesc)
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

	// Convert SortOrder to bool (true = DESC, false = ASC)
	sortDesc := page.Sort == SortOrderDesc

	// Query with limit+1 to detect if there are more pages
	rows, err := store.QueryBlockSummaries(ctx, page.Cursor, page.Limit+1, sortDesc)
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

// HandleBlockByHeight returns a single block by height.
// Returns 404 if block not found, 400 for invalid parameters.
func (c *Controller) HandleBlockByHeight(w http.ResponseWriter, r *http.Request) {
	chainID := mux.Vars(r)["id"]
	heightStr := mux.Vars(r)["height"]

	if chainID == "" || heightStr == "" {
		writeError(w, http.StatusBadRequest, "missing chain id or height")
		return
	}

	height, err := strconv.ParseUint(heightStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid height")
		return
	}

	ctx := context.Background()

	store, ok := c.App.LoadChainStore(ctx, chainID)
	if !ok {
		writeError(w, http.StatusNotFound, "chain not indexed")
		return
	}

	block, err := store.GetBlock(ctx, height)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "block not found")
		} else {
			writeError(w, http.StatusInternalServerError, "query failed")
		}
		return
	}

	writeJSON(w, http.StatusOK, block)
}

// HandleBlockSummaryByHeight returns a single block summary by height.
// Returns 404 if block summary not found, 400 for invalid parameters.
func (c *Controller) HandleBlockSummaryByHeight(w http.ResponseWriter, r *http.Request) {
	chainID := mux.Vars(r)["id"]
	heightStr := mux.Vars(r)["height"]

	if chainID == "" || heightStr == "" {
		writeError(w, http.StatusBadRequest, "missing chain id or height")
		return
	}

	height, err := strconv.ParseUint(heightStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid height")
		return
	}

	ctx := context.Background()

	store, ok := c.App.LoadChainStore(ctx, chainID)
	if !ok {
		writeError(w, http.StatusNotFound, "chain not indexed")
		return
	}

	summary, err := store.GetBlockSummary(ctx, height)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "block summary not found")
		} else {
			writeError(w, http.StatusInternalServerError, "query failed")
		}
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

package controller

import (
    "context"
    "github.com/canopy-network/canopyx/pkg/db/models/indexer"
    "github.com/gorilla/mux"
    "go.uber.org/zap"
    "net/http"
)

// HandleTransactions returns transaction data
func (c *Controller) HandleTransactions(w http.ResponseWriter, r *http.Request) {
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
    rows, err := store.QueryTransactions(ctx, page.Cursor, page.Limit+1)
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

    writeJSON(w, http.StatusOK, pagedResponse[indexer.Transaction]{
        Data:       rows,
        Limit:      page.Limit,
        NextCursor: nextCursor,
    })
}

// HandleTransactionsRaw returns raw transaction data with all columns
func (c *Controller) HandleTransactionsRaw(w http.ResponseWriter, r *http.Request) {
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
    rows, err := store.QueryTransactionsRaw(ctx, page.Cursor, page.Limit+1)
    if err != nil {
        c.App.Logger.Error("QueryTransactionsRaw failed", zap.Error(err), zap.String("chainID", chainID))
        writeError(w, http.StatusInternalServerError, "query failed")
        return
    }

    nextCursor := (*uint64)(nil)
    if len(rows) > page.Limit {
        rows = rows[:page.Limit]
        cursor := rows[len(rows)-1]["height"].(uint64)
        nextCursor = &cursor
    }

    writeJSON(w, http.StatusOK, pagedResponse[map[string]interface{}]{
        Data:       rows,
        Limit:      page.Limit,
        NextCursor: nextCursor,
    })
}

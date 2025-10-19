package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// HandleTransactions returns transaction data with optional message type filtering.
// Supports optional query parameter: type (e.g., ?type=delegate)
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

	// Convert SortOrder to bool (true = DESC, false = ASC)
	sortDesc := page.Sort == SortOrderDesc

	// Get optional message type filter from query parameter
	messageType := r.URL.Query().Get("type")

	// Query with limit+1 to detect if there are more pages
	rows, err := store.QueryTransactionsWithFilter(ctx, page.Cursor, page.Limit+1, sortDesc, messageType)
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

	// Convert SortOrder to bool (true = DESC, false = ASC)
	sortDesc := page.Sort == SortOrderDesc

	// Query with limit+1 to detect if there are more pages
	rows, err := store.QueryTransactionsRaw(ctx, page.Cursor, page.Limit+1, sortDesc)
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

// TransactionDetailResponse wraps transaction data with a parsed message field
// for easier frontend consumption.
type TransactionDetailResponse struct {
	Transaction *indexer.Transaction   `json:"transaction"`
	Message     map[string]interface{} `json:"message,omitempty"`
}

// HandleTransactionByHash returns a single transaction by hash with parsed message data.
// Returns both the full transaction and the parsed msg JSON separately for frontend convenience.
// Returns 404 if transaction not found, 400 for invalid parameters.
func (c *Controller) HandleTransactionByHash(w http.ResponseWriter, r *http.Request) {
	chainID := mux.Vars(r)["id"]
	txHash := mux.Vars(r)["hash"]

	if chainID == "" || txHash == "" {
		writeError(w, http.StatusBadRequest, "missing chain id or transaction hash")
		return
	}

	ctx := context.Background()

	store, ok := c.App.LoadChainStore(ctx, chainID)
	if !ok {
		writeError(w, http.StatusNotFound, "chain not indexed")
		return
	}

	tx, err := store.GetTransactionByHash(ctx, txHash)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, "query failed")
		}
		return
	}

	// Parse msg JSON for frontend display
	var msgData map[string]interface{}
	if tx.Msg != "" {
		if err := json.Unmarshal([]byte(tx.Msg), &msgData); err != nil {
			// Log error but still return transaction
			c.App.Logger.Warn("Failed to parse transaction message",
				zap.Error(err),
				zap.String("txHash", txHash),
				zap.String("chainID", chainID))
		}
	}

	response := TransactionDetailResponse{
		Transaction: tx,
		Message:     msgData,
	}

	writeJSON(w, http.StatusOK, response)
}

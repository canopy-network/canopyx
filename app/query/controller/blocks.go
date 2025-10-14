package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

type blockItem struct {
	Height   uint64 `json:"height"`
	Hash     string `json:"hash"`
	Time     string `json:"time"`
	Proposer string `json:"proposer"`
	TxCount  uint32 `json:"tx_count"`
}

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

	store, ok := c.App.ChainsDB.Load(chainID)
	if !ok {
		writeError(w, http.StatusNotFound, "chain not indexed")
		return
	}

	ctx := context.Background()

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

	out := make([]blockItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, blockItem{
			Height:   row.Height,
			Hash:     row.Hash,
			Time:     row.Time.UTC().Format(time.RFC3339),
			Proposer: row.ProposerAddress,
			TxCount:  row.NumTxs,
		})
	}

	writeJSON(w, http.StatusOK, pagedResponse[blockItem]{
		Data:       out,
		Limit:      page.Limit,
		NextCursor: nextCursor,
	})
}

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

	store, ok := c.App.ChainsDB.Load(chainID)
	if !ok {
		writeError(w, http.StatusNotFound, "chain not indexed")
		return
	}

	ctx := context.Background()

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

	type txItem struct {
		Height       uint64  `json:"height"`
		Hash         string  `json:"hash"`
		Time         string  `json:"time"`
		MessageType  string  `json:"message_type"`
		Signer       string  `json:"signer"`
		Counterparty *string `json:"counterparty,omitempty"`
		Amount       *uint64 `json:"amount,omitempty"`
		Fee          uint64  `json:"fee"`
	}

	out := make([]txItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, txItem{
			Height:       row.Height,
			Hash:         row.TxHash,
			Time:         row.Time.UTC().Format(time.RFC3339),
			MessageType:  row.MessageType,
			Signer:       row.Signer,
			Counterparty: row.Counterparty,
			Amount:       row.Amount,
			Fee:          row.Fee,
		})
	}

	writeJSON(w, http.StatusOK, pagedResponse[txItem]{
		Data:       out,
		Limit:      page.Limit,
		NextCursor: nextCursor,
	})
}

package controller

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

type blockRow struct {
	Height          uint64    `ch:"height"`
	Hash            string    `ch:"hash"`
	Time            time.Time `ch:"time"`
	ProposerAddress string    `ch:"proposer_address"`
	NumTxs          uint32    `ch:"num_txs"`
}

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

	db := store.RawDB()
	ctx := context.Background()

	conds := make([]string, 0)
	args := make([]any, 0)
	if page.Cursor > 0 {
		conds = append(conds, "height < ?")
		args = append(args, page.Cursor)
	}

	query := fmt.Sprintf(`SELECT height, hash, time, proposer_address, num_txs FROM "%s"."blocks"`, store.DatabaseName())
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += " ORDER BY height DESC LIMIT ?"
	args = append(args, page.Limit+1)

	rows := make([]blockRow, 0, page.Limit+1)
	if err := db.NewRaw(query, args...).Scan(ctx, &rows); err != nil {
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

	db := store.RawDB()
	ctx := context.Background()

	type txRow struct {
		Height       uint64    `ch:"height"`
		TxHash       string    `ch:"tx_hash"`
		Time         time.Time `ch:"time"`
		MessageType  string    `ch:"message_type"`
		Counterparty *string   `ch:"counterparty"`
		Signer       string    `ch:"signer"`
		Amount       *uint64   `ch:"amount"`
		Fee          uint64    `ch:"fee"`
	}

	conds := make([]string, 0)
	args := make([]any, 0)
	if page.Cursor > 0 {
		conds = append(conds, "height < ?")
		args = append(args, page.Cursor)
	}

	query := fmt.Sprintf(`SELECT height, tx_hash, time, message_type, counterparty, signer, amount, fee FROM "%s"."txs"`, store.DatabaseName())
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += " ORDER BY height DESC LIMIT ?"
	args = append(args, page.Limit+1)

	rows := make([]txRow, 0, page.Limit+1)
	if err := db.NewRaw(query, args...).Scan(ctx, &rows); err != nil {
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

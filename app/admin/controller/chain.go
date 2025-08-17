package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/canopy-network/canopyx/app/admin/types"
	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"github.com/canopy-network/canopyx/pkg/utils"
	"github.com/go-jose/go-jose/v4/json"
	"github.com/gorilla/mux"
	"github.com/uptrace/go-clickhouse/ch"
	"go.uber.org/zap"
)

// HandleChainsList returns all registered chains
func (c *Controller) HandleChainsList(w http.ResponseWriter, r *http.Request) {
	cs, err := c.App.AdminDB.ListChain(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if cs == nil {
		cs = make([]admin.Chain, 0)
	}
	_ = json.NewEncoder(w).Encode(cs)
}

// HandleChainsUpsert creates or updates a chain
func (c *Controller) HandleChainsUpsert(w http.ResponseWriter, r *http.Request) {
	var chain admin.Chain
	if err := json.NewDecoder(r.Body).Decode(&chain); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad json"})
		return
	}
	if chain.ChainID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "chain_id required"})
		return
	}

	ctx := r.Context()

	if err := c.App.AdminDB.UpsertChain(ctx, &chain); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if _, err := c.App.NewChainDb(ctx, chain.ChainID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	err := c.App.EnsureChainSchedules(r.Context(), chain.ChainID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"ok": "1"})
}

// HandleChainDetail returns a chain by ID
func (c *Controller) HandleChainDetail(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	chain, err := c.App.AdminDB.GetChain(r.Context(), id)
	if err != nil {
		w.WriteHeader(404)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		return
	}
	_ = json.NewEncoder(w).Encode(chain)
}

// HandleProgress responds with the last indexed progress of a specific resource identified by its ID.
func (c *Controller) HandleProgress(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	h, _ := c.App.AdminDB.LastIndexed(r.Context(), id)
	_ = json.NewEncoder(w).Encode(map[string]any{"last": h})
}

// HandleGaps retrieves and sends back gaps identified for a specific resource based on the given ID from the request.
func (c *Controller) HandleGaps(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	gs, _ := c.App.AdminDB.FindGaps(r.Context(), id)
	_ = json.NewEncoder(w).Encode(gs)
}

// HandleChainStatus returns the last indexed height for all known chains.
func (c *Controller) HandleChainStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1) Get chains (for RPC fanout + to ensure we cover all known IDs)
	chains, err := c.App.AdminDB.ListChain(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	out := make(map[string]types.ChainStatus, len(chains))

	// 2) Bulk last-indexed via aggregate table
	// SELECT chain_id, maxMerge(max_height) FROM <db>.index_progress_agg GROUP BY chain_id
	{
		dbName := c.App.AdminDB.Name
		q := fmt.Sprintf(`
			SELECT chain_id, maxMerge(max_height) AS last_idx
			FROM "%s"."index_progress_agg"
			GROUP BY chain_id
		`, dbName)

		rows, err := c.App.AdminDB.Db.QueryContext(ctx, q)
		if err == nil {
			defer func(rows *ch.Rows) {
				err := rows.Close()
				if err != nil {
					c.App.Logger.Error("Failed to close rows", zap.Error(err))
				}
			}(rows)
			for rows.Next() {
				var id string
				var last uint64
				if err := rows.Scan(&id, &last); err != nil {
					continue
				}
				cs := out[id]
				cs.LastIndexed = last
				out[id] = cs
			}
			_ = rows.Err()
		}
	}

	// 3) Fallback for any chain missing in the aggregate: max(height) from base table
	{
		dbName := c.App.AdminDB.Name
		q := fmt.Sprintf(`
			SELECT chain_id, max(height) AS last_idx
			FROM "%s"."index_progress"
			GROUP BY chain_id
		`, dbName)

		rows, err := c.App.AdminDB.Db.QueryContext(ctx, q)
		if err == nil {
			defer func(rows *ch.Rows) {
				err := rows.Close()
				if err != nil {
					c.App.Logger.Error("Failed to close rows", zap.Error(err))
				}
			}(rows)
			for rows.Next() {
				var id string
				var last uint64
				if err := rows.Scan(&id, &last); err != nil {
					continue
				}
				cs := out[id]
				if cs.LastIndexed == 0 {
					cs.LastIndexed = last
					out[id] = cs
				}
			}
			_ = rows.Err()
		}
	}

	// 4) Heads via RPC (best-effort, bounded concurrency)
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, chn := range chains {
		chn := chn
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()

			cli := rpc.NewHTTPWithOpts(rpc.Opts{Endpoints: chn.RPCEndpoints})
			head, err := cli.ChainHead(ctx)
			if err != nil {
				return
			}
			cs := out[chn.ChainID]
			cs.Head = head
			out[chn.ChainID] = cs
		}()
	}
	wg.Wait()

	_ = json.NewEncoder(w).Encode(out)
}

// HandleChainPatch updates a chain by ID
func (c *Controller) HandleChainPatch(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	// 1) Load the current row (so we can do a partial update)
	cur, err := c.App.AdminDB.GetChain(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		return
	}

	// 2) Decode patch body
	var in admin.Chain
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad json"})
		return
	}

	// 3) Apply changes
	if in.ChainName != cur.ChainName {
		cur.ChainName = strings.TrimSpace(in.ChainName)
	}

	if in.Paused != cur.Paused {
		cur.Paused = in.Paused
	}
	if in.Deleted != cur.Deleted {
		cur.Deleted = in.Deleted
	}
	if in.RPCEndpoints != nil { // provided => replace
		cleaned := make([]string, 0, len(in.RPCEndpoints))
		for _, e := range in.RPCEndpoints {
			e = strings.TrimSpace(e)
			if e != "" {
				cleaned = append(cleaned, e)
			}
		}
		cur.RPCEndpoints = utils.Dedup(cleaned)
	}

	// 4) Persist (ReplacingMergeTree upsert)
	if upsertErr := c.App.AdminDB.UpsertChain(r.Context(), cur); upsertErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": upsertErr.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"ok": "1"})
}

// PATCH /admin/chains/status
func (c *Controller) HandlePatchChainsStatus(w http.ResponseWriter, r *http.Request) {
	var reqs []admin.Chain
	if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if len(reqs) == 0 {
		http.Error(w, "empty patch list", http.StatusBadRequest)
		return
	}

	// Convert to DB layer patches and execute
	patches := make([]admin.Chain, 0, len(reqs))
	for _, x := range reqs {
		if x.ChainID == "" {
			http.Error(w, "chain_id is required", http.StatusBadRequest)
			return
		}

		if x.RPCEndpoints != nil {
			x.RPCEndpoints = utils.Dedup(x.RPCEndpoints)
		}

		patches = append(patches, x)
	}

	if err := c.App.AdminDB.PatchChains(r.Context(), patches); err != nil {
		// Map a not-found to 404, everything else 500
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

package controller

import (
	"net/http"

	"github.com/canopy-network/canopyx/app/query/types"
	"github.com/go-jose/go-jose/v4/json"
	"github.com/gorilla/mux"
)

// StatsHour returns the last 500 hourly tx buckets for a chain.
func (c *Controller) StatsHour(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := mux.Vars(r)["id"]

	rows, err := c.App.ReportDB.GetChainTxHourly(ctx, id, 500)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	out := make([]types.StatsHour, 0, len(rows))
	for _, r := range rows {
		out = append(out, types.StatsHour{
			Hour:  r.Hour,
			Count: r.Count,
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

// StatsDay returns the last 90 daily tx buckets for a chain.
func (c *Controller) StatsDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := mux.Vars(r)["id"]

	rows, err := c.App.ReportDB.GetChainTxDaily(ctx, id, 90)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	out := make([]types.StatsDay, 0, len(rows))
	for _, r := range rows {
		out = append(out, types.StatsDay{
			Day:   r.Day, // r.Day is Date; your type uses time.Time—OK
			Count: r.Count,
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

// Stats24h returns the most recent 24h snapshot for a chain (latest row).
func (c *Controller) Stats24h(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := mux.Vars(r)["id"]

	rows, err := c.App.ReportDB.GetChainTx24h(ctx, id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	out := make([]types.Stats24h, 0, len(rows))
	for _, r := range rows {
		out = append(out, types.Stats24h{
			AsOf:  r.AsOf,
			Count: r.Count,
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

package controller

import (
	"net/http"

	"github.com/go-jose/go-jose/v4/json"
)

func (c *Controller) HandleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := c.App.IndexerDB.Db.Ping(ctx); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "errored", "error": "database connection error"})
		return
	}

	if err := c.App.ReportDB.Db.Ping(ctx); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "errored", "error": "database connection error"})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

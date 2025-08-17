package controller

import (
	"net/http"

	"github.com/go-jose/go-jose/v4/json"
)

func (c *Controller) HandleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	health, err := c.App.TemporalClient.Health(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !health.ConnectionOK {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "temporal connection error"})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(health)
}

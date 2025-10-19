package controller

import (
	"net/http"

	"github.com/canopy-network/canopyx/app/query/types"
	"github.com/gorilla/mux"
)

type Controller struct {
	App *types.App
}

// NewController returns a new controller.
func NewController(app *types.App) *Controller {
	return &Controller{
		App: app,
	}
}

// WithCORS is a middleware that adds CORS headers to the response.
func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Development: Echo back the origin to allow credentials with any origin
		// TODO: Restrict this in production to specific domains
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", http.MethodGet+", "+http.MethodPost+", "+http.MethodPut+", "+http.MethodPatch+", "+http.MethodDelete+", "+http.MethodOptions)

		// Fast-path the preflight
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// NewRouter returns a new router with all the routes defined in this file.
func (c *Controller) NewRouter() (*mux.Router, error) {
	r := mux.NewRouter()

	r.Handle("/health", http.HandlerFunc(c.HandleHealth)).Methods("GET")

	// WebSocket endpoint for real-time events
	r.HandleFunc("/ws", c.HandleWebSocket).Methods("GET")

	r.HandleFunc("/chains/{id}/stats/hour", c.StatsHour).Methods("GET")
	r.HandleFunc("/chains/{id}/stats/day", c.StatsDay).Methods("GET")
	r.HandleFunc("/chains/{id}/stats/24h", c.Stats24h).Methods("GET")
	r.HandleFunc("/chains/{id}/blocks", c.HandleBlocks).Methods("GET")
	r.HandleFunc("/chains/{id}/block_summaries", c.HandleBlockSummaries).Methods("GET")
	r.HandleFunc("/chains/{id}/transactions", c.HandleTransactions).Methods("GET")
	r.HandleFunc("/chains/{id}/schema", c.HandleSchema).Methods("GET")
	r.HandleFunc("/chains/{id}/transactions_raw", c.HandleTransactionsRaw).Methods("GET")

	return r, nil
}

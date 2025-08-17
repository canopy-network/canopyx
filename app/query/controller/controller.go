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

// NewRouter returns a new router with all the routes defined in this file.
func (c *Controller) NewRouter() (*mux.Router, error) {
	r := mux.NewRouter()

	r.Handle("/health", http.HandlerFunc(c.HandleHealth)).Methods("GET")

	r.HandleFunc("/chains/{id}/stats/hour", c.StatsHour).Methods("GET")
	r.HandleFunc("/chains/{id}/stats/day", c.StatsDay).Methods("GET")
	r.HandleFunc("/chains/{id}/stats/24h", c.Stats24h).Methods("GET")

	return r, nil
}

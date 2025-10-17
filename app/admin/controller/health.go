package controller

import (
    "net/http"

    "github.com/go-jose/go-jose/v4/json"
)

func (c *Controller) HandleHealth(w http.ResponseWriter, r *http.Request) {
    //ctx := r.Context()

    w.WriteHeader(http.StatusOK)
    // implement a real health check
    _ = json.NewEncoder(w).Encode("1")
}

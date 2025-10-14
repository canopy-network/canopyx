package controller

import (
	"net/http"

	"github.com/canopy-network/canopyx/app/admin/types"
	"github.com/canopy-network/canopyx/pkg/utils"
	"github.com/go-jose/go-jose/v4/json"
	"github.com/gorilla/mux"
)

type Controller struct {
	App        *types.App
	AdminToken string
	AuthUser   string
	Users      map[string]types.User
	AuthHash   []byte
	JWTSecret  []byte
}

// NewController returns a new controller.
func NewController(app *types.App) *Controller {
	adminToken := utils.Env("ADMIN_TOKEN", "devtoken")
	adminUser := utils.Env("ADMIN_USER", "admin")
	adminUsersJSON := utils.Env("ADMIN_USERS", "")
	adminPass := utils.Env("ADMIN_PASSWORD", "admin")
	jwtSecret := []byte(utils.Env("SESSION_SECRET", "change-me-please"))

	phash, _ := utils.HashOrRead(adminPass)
	users := map[string]types.User{}
	users[adminUser] = types.User{Username: adminUser, Hash: phash, Role: "admin"}
	if adminUsersJSON != "" {
		_ = json.Unmarshal([]byte(adminUsersJSON), &users)
	}

	return &Controller{
		App:        app,
		AdminToken: adminToken,
		AuthUser:   adminUser,
		Users:      users,
		AuthHash:   phash,
		JWTSecret:  jwtSecret,
	}
}

// WithCORS is a middleware that adds CORS headers to the response.
func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Allow your Next dev origin
		// TODO: make this configurable? or somehow calculated from ADDR
		if origin == "http://localhost:3003" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		}

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

	// basic it's ok, could even be a public endpoint
	r.Handle("/api/health", http.HandlerFunc(c.HandleHealth)).Methods(http.MethodGet)

	// API (crud?)
	r.Handle("/api/chains", c.RequireAuth(http.HandlerFunc(c.HandleChainsList))).Methods(http.MethodGet)
	r.Handle("/api/chains", c.RequireAuth(http.HandlerFunc(c.HandleChainsUpsert))).Methods(http.MethodPost)
	// GET/PATCH bulk status
	r.Handle("/api/chains/status", c.RequireAuth(http.HandlerFunc(c.HandleChainStatus))).Methods(http.MethodGet)
	r.Handle("/api/chains/status", c.RequireAuth(http.HandlerFunc(c.HandlePatchChainsStatus))).Methods(http.MethodPatch)
	// GET/PATCH individual status
	r.Handle("/api/chains/{id}", c.RequireAuth(http.HandlerFunc(c.HandleChainDetail))).Methods(http.MethodGet)
	r.Handle("/api/chains/{id}/progress", c.RequireAuth(http.HandlerFunc(c.HandleProgress))).Methods(http.MethodGet)
	r.Handle("/api/chains/{id}/gaps", c.RequireAuth(http.HandlerFunc(c.HandleGaps))).Methods(http.MethodGet)
	r.Handle("/api/chains/{id}", c.RequireAuth(http.HandlerFunc(c.HandleChainPatch))).Methods(http.MethodPatch)
	r.Handle("/api/chains/{id}/headscan", c.RequireAuth(http.HandlerFunc(c.HandleTriggerHeadScan))).Methods(http.MethodPost)
	r.Handle("/api/chains/{id}/gapscan", c.RequireAuth(http.HandlerFunc(c.HandleTriggerGapScan))).Methods(http.MethodPost)
	r.Handle("/api/chains/{id}/reindex", c.RequireAuth(http.HandlerFunc(c.HandleReindex))).Methods(http.MethodPost)

	if err := c.LoadAdminUI(r); err != nil {
		return nil, err
	}

	return r, nil
}

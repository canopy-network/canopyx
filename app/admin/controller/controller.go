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

	// basically it's ok, could even be a public endpoint
	r.Handle("/api/health", http.HandlerFunc(c.HandleHealth)).Methods(http.MethodGet)

	// Admin API - Login/Logout (normalized to /api prefix)
	r.HandleFunc("/api/auth/login", c.HandleAdminLogin).Methods(http.MethodPost)
	r.HandleFunc("/api/auth/logout", c.HandleAdminLogout).Methods(http.MethodPost)

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
	r.Handle("/api/chains/{id}/progress-history", c.RequireAuth(http.HandlerFunc(c.HandleIndexProgressHistory))).Methods(http.MethodGet)
	r.Handle("/api/chains/{id}", c.RequireAuth(http.HandlerFunc(c.HandleChainPatch))).Methods(http.MethodPatch)
	r.Handle("/api/chains/{id}", c.RequireAuth(http.HandlerFunc(c.HandleChainDelete))).Methods(http.MethodDelete)
	r.Handle("/api/chains/{id}/headscan", c.RequireAuth(http.HandlerFunc(c.HandleTriggerHeadScan))).Methods(http.MethodPost)
	r.Handle("/api/chains/{id}/gapscan", c.RequireAuth(http.HandlerFunc(c.HandleTriggerGapScan))).Methods(http.MethodPost)
	r.Handle("/api/chains/{id}/reindex", c.RequireAuth(http.HandlerFunc(c.HandleReindex))).Methods(http.MethodPost)

	// Schema introspection endpoints (migrated from query service)
	r.Handle("/api/entities", c.RequireAuth(http.HandlerFunc(c.HandleEntities))).Methods(http.MethodGet)
	r.Handle("/api/chains/{id}/schema", c.RequireAuth(http.HandlerFunc(c.HandleSchema))).Methods(http.MethodGet)

	// Generic entity query endpoints
	r.Handle("/api/chains/{id}/entity/{entity}", c.RequireAuth(http.HandlerFunc(c.HandleEntityQuery))).Methods(http.MethodGet)
	// Note: HandleEntityGet now uses query parameters (property, value, height) instead of path parameter id_value
	// This provides explicit column-based lookups without guessing the intent
	r.Handle("/api/chains/{id}/entity/{entity}/lookup", c.RequireAuth(http.HandlerFunc(c.HandleEntityGet))).Methods(http.MethodGet)
	// Entity schema endpoint - returns property names and types for dynamic form building
	r.Handle("/api/chains/{id}/entity/{entity}/schema", c.RequireAuth(http.HandlerFunc(c.HandleEntitySchema))).Methods(http.MethodGet)

	// WebSocket endpoint for real-time events (migrated from query service)
	r.HandleFunc("/api/ws", c.HandleWebSocket).Methods(http.MethodGet)

	// Cross-chain API endpoints
	r.Handle("/api/crosschain/health", c.RequireAuth(http.HandlerFunc(c.HandleCrossChainHealth))).Methods(http.MethodGet)
	r.Handle("/api/crosschain/resync/{chainID}/{table}", c.RequireAuth(http.HandlerFunc(c.HandleCrossChainResyncTable))).Methods(http.MethodPost)
	r.Handle("/api/crosschain/resync/{chainID}", c.RequireAuth(http.HandlerFunc(c.HandleCrossChainResyncChain))).Methods(http.MethodPost)
	r.Handle("/api/crosschain/sync-status/{chainID}/{table}", c.RequireAuth(http.HandlerFunc(c.HandleCrossChainSyncStatus))).Methods(http.MethodGet)

	// Cross-chain entity query endpoints (dynamic, mirror chain entity pattern)
	r.Handle("/api/crosschain/entities", c.RequireAuth(http.HandlerFunc(c.HandleCrossChainEntitiesList))).Methods(http.MethodGet)
	r.Handle("/api/crosschain/entities/{entity}", c.RequireAuth(http.HandlerFunc(c.HandleCrossChainEntities))).Methods(http.MethodGet)
	r.Handle("/api/crosschain/entities/{entity}/schema", c.RequireAuth(http.HandlerFunc(c.HandleCrossChainEntitySchema))).Methods(http.MethodGet)

	return r, nil
}

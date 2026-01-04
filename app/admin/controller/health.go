package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/go-jose/go-jose/v4/json"
	"go.uber.org/zap"
)

// HealthStatus represents the health check response
type HealthStatus struct {
	Status    string            `json:"status"` // "healthy", "degraded", or "unhealthy"
	Checks    map[string]string `json:"checks"` // Individual check statuses
	Timestamp time.Time         `json:"timestamp"`
}

// HandleHealth performs comprehensive health checks on critical system components.
// Checks: AdminDB connectivity, Temporal connection, Redis (if configured).
// GET /api/admin/health
func (c *Controller) HandleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	checks := make(map[string]string)
	overallHealthy := true

	// 1. Check AdminDB connectivity by trying a simple query
	if _, err := c.App.AdminDB.ListChain(ctx, false); err != nil {
		c.App.Logger.Warn("Health check: AdminDB query failed", zap.Error(err))
		checks["admin_db"] = "unhealthy: " + err.Error()
		overallHealthy = false
	} else {
		checks["admin_db"] = "healthy"
	}

	// 2. Check Temporal connection using CheckHealth method
	if c.App.TemporalManager != nil {
		adminClient, err := c.App.TemporalManager.GetAdminClient(ctx)
		if err != nil {
			c.App.Logger.Warn("Health check: Temporal admin client failed", zap.Error(err))
			checks["temporal"] = "unhealthy: " + err.Error()
			overallHealthy = false
		} else if _, err := adminClient.TClient.CheckHealth(ctx, nil); err != nil {
			c.App.Logger.Warn("Health check: Temporal connection failed", zap.Error(err))
			checks["temporal"] = "unhealthy: " + err.Error()
			overallHealthy = false
		} else {
			checks["temporal"] = "healthy"
		}
	} else {
		checks["temporal"] = "not configured"
	}

	// 3. Check Redis (if configured)
	if c.App.RedisClient != nil {
		if err := c.App.RedisClient.Health(ctx); err != nil {
			c.App.Logger.Warn("Health check: Redis ping failed", zap.Error(err))
			checks["redis"] = "unhealthy: " + err.Error()
			overallHealthy = false
		} else {
			checks["redis"] = "healthy"
		}
	} else {
		checks["redis"] = "not configured"
	}

	// Determine overall status
	status := "healthy"
	statusCode := http.StatusOK
	if !overallHealthy {
		status = "unhealthy"
		statusCode = http.StatusServiceUnavailable
	}

	response := HealthStatus{
		Status:    status,
		Checks:    checks,
		Timestamp: time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(response)
}

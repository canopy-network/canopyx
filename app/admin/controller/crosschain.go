package controller

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-jose/go-jose/v4/json"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// HandleCrossChainResyncTable handles POST /admin/crosschain/resync/:chainID/:table
// Resyncs a specific table for a specific chain (backfills existing data)
func (c *Controller) HandleCrossChainResyncTable(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	chainIDStr := vars["chainID"]
	tableName := vars["table"]

	if chainIDStr == "" || tableName == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "chain_id and table required"})
		return
	}

	chainID, parseErr := strconv.ParseUint(chainIDStr, 10, 64)
	if parseErr != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chain_id"})
		return
	}

	ctx := r.Context()

	// Check if cross-chain DB is available
	if c.App.CrossChainDB == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "cross-chain database not available"})
		return
	}

	// Type asserts to get the ResyncTable method
	ccStore, ok := c.App.CrossChainDB.(interface {
		ResyncTable(context.Context, uint64, string) error
	})
	if !ok {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "cross-chain store method not available"})
		return
	}

	user := c.currentUser(r)
	c.App.Logger.Info("Resyncing cross-chain table",
		zap.Uint64("chain_id", chainID),
		zap.String("table", tableName),
		zap.String("user", user))

	if err := ccStore.ResyncTable(ctx, chainID, tableName); err != nil {
		c.App.Logger.Error("Failed to resync cross-chain table",
			zap.Uint64("chain_id", chainID),
			zap.String("table", tableName),
			zap.String("user", user),
			zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	c.App.Logger.Info("Cross-chain table resync complete",
		zap.Uint64("chain_id", chainID),
		zap.String("table", tableName),
		zap.String("user", user))

	_ = json.NewEncoder(w).Encode(map[string]string{
		"ok":       "1",
		"chain_id": chainIDStr,
		"table":    tableName,
	})
}

// HandleCrossChainResyncChain handles POST /admin/crosschain/resync/:chainID
// Resyncs all tables for a specific chain (backfills existing data)
func (c *Controller) HandleCrossChainResyncChain(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	chainIDStr := vars["chainID"]

	if chainIDStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "chain_id required"})
		return
	}

	chainID, parseErr := strconv.ParseUint(chainIDStr, 10, 64)
	if parseErr != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chain_id"})
		return
	}

	ctx := r.Context()

	// Check if cross-chain DB is available
	if c.App.CrossChainDB == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "cross-chain database not available"})
		return
	}

	// Type assert to get the ResyncChain method
	ccStore, ok := c.App.CrossChainDB.(interface {
		ResyncChain(context.Context, uint64) error
	})
	if !ok {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "cross-chain store method not available"})
		return
	}

	user := c.currentUser(r)
	c.App.Logger.Info("Resyncing all cross-chain tables for chain",
		zap.Uint64("chain_id", chainID),
		zap.String("user", user))

	if err := ccStore.ResyncChain(ctx, chainID); err != nil {
		c.App.Logger.Error("Failed to resync cross-chain tables",
			zap.Uint64("chain_id", chainID),
			zap.String("user", user),
			zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	c.App.Logger.Info("Cross-chain resync complete for all tables",
		zap.Uint64("chain_id", chainID),
		zap.String("user", user))

	_ = json.NewEncoder(w).Encode(map[string]string{
		"ok":       "1",
		"chain_id": chainIDStr,
	})
}

// HandleCrossChainHealth handles GET /admin/crosschain/health
// Returns health status of the cross-chain sync system
func (c *Controller) HandleCrossChainHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check if cross-chain DB is available
	if c.App.CrossChainDB == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "cross-chain database not available"})
		return
	}

	health, err := c.App.CrossChainDB.GetHealthStatus(ctx)
	if err != nil {
		c.App.Logger.Error("Failed to get cross-chain health status", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(health)
}

// HandleCrossChainSyncStatus handles GET /admin/crosschain/sync-status/:chainID/:table
// Returns sync status for a specific chain and table
func (c *Controller) HandleCrossChainSyncStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	chainIDStr := vars["chainID"]
	tableName := vars["table"]

	if chainIDStr == "" || tableName == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "chain_id and table required"})
		return
	}

	chainID, parseErr := strconv.ParseUint(chainIDStr, 10, 64)
	if parseErr != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chain_id"})
		return
	}

	ctx := r.Context()

	// Check if cross-chain DB is available
	if c.App.CrossChainDB == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "cross-chain database not available"})
		return
	}

	status, err := c.App.CrossChainDB.GetSyncStatus(ctx, chainID, tableName)
	if err != nil {
		c.App.Logger.Error("Failed to get sync status",
			zap.Uint64("chain_id", chainID),
			zap.String("table", tableName),
			zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(status)
}

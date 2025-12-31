package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	chainstore "github.com/canopy-network/canopyx/pkg/db/chain"
	crosschainstore "github.com/canopy-network/canopyx/pkg/db/crosschain"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	indexerworkflow "github.com/canopy-network/canopyx/pkg/temporal/indexer"
	"github.com/go-jose/go-jose/v4/json"
	"github.com/gorilla/mux"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
)

// HandleCreateLPSnapshotSchedule creates an hourly schedule for LP position snapshots.
// POST /api/v1/admin/chains/:id/lp-schedule
// Optional body: {"backfill": {"start": "2024-01-01", "end": "2024-12-31"}}
// If backfill dates not provided, calculates from block 1 to current indexed height.
func (c *Controller) HandleCreateLPSnapshotSchedule(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	chainIDStr := vars["id"]

	chainIDUint, err := strconv.ParseUint(chainIDStr, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chain ID"})
		return
	}

	// Parse optional backfill parameters
	var req struct {
		Backfill *struct {
			Start string `json:"start"` // UTC date format: "2006-01-02"
			End   string `json:"end"`   // UTC date format: "2006-01-02"
		} `json:"backfill"`
	}
	if r.Body != http.NoBody {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Ignore decode errors for empty body
			_ = err
		}
	}

	// Create the schedule
	scheduleID := c.App.TemporalClient.GetLPSnapshotScheduleID(chainIDUint)
	handle := c.App.TemporalClient.TSClient.GetHandle(r.Context(), scheduleID)

	// Check if schedule already exists
	_, err = handle.Describe(r.Context())
	if err == nil {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("LP snapshot schedule already exists for chain %d", chainIDUint),
		})
		return
	}

	var notFound *serviceerror.NotFound
	if !errors.As(err, &notFound) {
		// Unexpected error
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Create hourly schedule
	c.App.Logger.Info("Creating LP snapshot schedule",
		zap.Uint64("chain_id", chainIDUint),
		zap.String("schedule_id", scheduleID))

	_, scheduleErr := c.App.TemporalClient.TSClient.Create(
		r.Context(),
		client.ScheduleOptions{
			ID:   scheduleID,
			Spec: c.App.TemporalClient.OneHourSpec(),
			Action: &client.ScheduleWorkflowAction{
				Workflow:  indexerworkflow.LPSnapshotWorkflowName,
				Args:      []interface{}{}, // No input args - workflow reads scheduled time from context
				TaskQueue: c.App.TemporalClient.GetIndexerOpsQueue(chainIDUint),
			},
		},
	)
	if scheduleErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": scheduleErr.Error()})
		return
	}

	c.App.Logger.Info("LP snapshot schedule created successfully",
		zap.Uint64("chain_id", chainIDUint))

	// Trigger backfill if requested
	if req.Backfill != nil {
		var startDate, endDate time.Time
		var backfillErr error

		if req.Backfill.Start != "" && req.Backfill.End != "" {
			// Use provided dates
			startDate, backfillErr = time.Parse("2006-01-02", req.Backfill.Start)
			if backfillErr != nil {
				c.App.Logger.Warn("Invalid backfill start date, skipping backfill",
					zap.String("start", req.Backfill.Start),
					zap.Error(backfillErr))
			} else {
				endDate, backfillErr = time.Parse("2006-01-02", req.Backfill.End)
				if backfillErr != nil {
					c.App.Logger.Warn("Invalid backfill end date, skipping backfill",
						zap.String("end", req.Backfill.End),
						zap.Error(backfillErr))
				}
			}
		} else {
			// Calculate from block heights
			startDate, endDate, backfillErr = c.calculateBackfillRange(r.Context(), chainIDUint)
		}

		if backfillErr == nil {
			if err := c.triggerLPSnapshotBackfill(r.Context(), chainIDUint, scheduleID, startDate, endDate); err != nil {
				c.App.Logger.Warn("Failed to trigger backfill after schedule creation",
					zap.Uint64("chain_id", chainIDUint),
					zap.Error(err))
				// Don't fail the request - schedule was created successfully
			}
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "created",
		"schedule_id": scheduleID,
		"chain_id":    chainIDUint,
	})
}

// HandlePauseLPSnapshotSchedule pauses the LP snapshot schedule.
// POST /api/v1/admin/chains/:id/lp-schedule/pause
func (c *Controller) HandlePauseLPSnapshotSchedule(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	chainIDStr := vars["id"]

	chainIDUint, err := strconv.ParseUint(chainIDStr, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chain ID"})
		return
	}

	scheduleID := c.App.TemporalClient.GetLPSnapshotScheduleID(chainIDUint)
	handle := c.App.TemporalClient.TSClient.GetHandle(r.Context(), scheduleID)

	if err := handle.Pause(r.Context(), client.SchedulePauseOptions{
		Note: "Paused via Admin API",
	}); err != nil {
		var notFound *serviceerror.NotFound
		if errors.As(err, &notFound) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "schedule not found"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	c.App.Logger.Info("LP snapshot schedule paused",
		zap.Uint64("chain_id", chainIDUint))

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "paused",
		"schedule_id": scheduleID,
		"chain_id":    chainIDUint,
	})
}

// HandleUnpauseLPSnapshotSchedule unpauses the LP snapshot schedule.
// POST /api/v1/admin/chains/:id/lp-schedule/unpause
// Optional body: {"backfill": {"start": "2024-01-01", "end": "2024-12-31"}}
func (c *Controller) HandleUnpauseLPSnapshotSchedule(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	chainIDStr := vars["id"]

	chainIDUint, err := strconv.ParseUint(chainIDStr, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chain ID"})
		return
	}

	// Parse optional backfill parameters
	var req struct {
		Backfill *struct {
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"backfill"`
	}
	if r.Body != http.NoBody {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Ignore decode errors for empty body
			_ = err
		}
	}

	scheduleID := c.App.TemporalClient.GetLPSnapshotScheduleID(chainIDUint)
	handle := c.App.TemporalClient.TSClient.GetHandle(r.Context(), scheduleID)

	if err := handle.Unpause(r.Context(), client.ScheduleUnpauseOptions{
		Note: "Unpaused via Admin API",
	}); err != nil {
		var notFound *serviceerror.NotFound
		if errors.As(err, &notFound) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "schedule not found"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	c.App.Logger.Info("LP snapshot schedule unpaused",
		zap.Uint64("chain_id", chainIDUint))

	// Trigger backfill if requested
	if req.Backfill != nil {
		var startDate, endDate time.Time
		var backfillErr error

		if req.Backfill.Start != "" && req.Backfill.End != "" {
			startDate, backfillErr = time.Parse("2006-01-02", req.Backfill.Start)
			if backfillErr != nil {
				c.App.Logger.Warn("Invalid backfill start date, skipping backfill",
					zap.String("start", req.Backfill.Start),
					zap.Error(backfillErr))
			} else {
				endDate, backfillErr = time.Parse("2006-01-02", req.Backfill.End)
				if backfillErr != nil {
					c.App.Logger.Warn("Invalid backfill end date, skipping backfill",
						zap.String("end", req.Backfill.End),
						zap.Error(backfillErr))
				}
			}
		} else {
			startDate, endDate, backfillErr = c.calculateBackfillRange(r.Context(), chainIDUint)
		}

		if backfillErr == nil {
			if err := c.triggerLPSnapshotBackfill(r.Context(), chainIDUint, scheduleID, startDate, endDate); err != nil {
				c.App.Logger.Warn("Failed to trigger backfill after unpause",
					zap.Uint64("chain_id", chainIDUint),
					zap.Error(err))
			}
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "unpaused",
		"schedule_id": scheduleID,
		"chain_id":    chainIDUint,
	})
}

// HandleDeleteLPSnapshotSchedule deletes the LP snapshot schedule.
// DELETE /api/v1/admin/chains/:id/lp-schedule
func (c *Controller) HandleDeleteLPSnapshotSchedule(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	chainIDStr := vars["id"]

	chainIDUint, err := strconv.ParseUint(chainIDStr, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chain ID"})
		return
	}

	scheduleID := c.App.TemporalClient.GetLPSnapshotScheduleID(chainIDUint)
	handle := c.App.TemporalClient.TSClient.GetHandle(r.Context(), scheduleID)

	if err := handle.Delete(r.Context()); err != nil {
		var notFound *serviceerror.NotFound
		if errors.As(err, &notFound) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "schedule not found"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	c.App.Logger.Info("LP snapshot schedule deleted",
		zap.Uint64("chain_id", chainIDUint))

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "deleted",
		"schedule_id": scheduleID,
		"chain_id":    chainIDUint,
	})
}

// HandleTriggerLPSnapshotBackfill triggers a backfill for LP position snapshots.
// POST /api/v1/admin/chains/:id/lp-snapshots/backfill
// Body: {"start": "2024-01-01", "end": "2024-12-31"}
// If dates not provided, calculates from block 1 to current indexed height.
func (c *Controller) HandleTriggerLPSnapshotBackfill(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	chainIDStr := vars["id"]

	chainIDUint, err := strconv.ParseUint(chainIDStr, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chain ID"})
		return
	}

	var req struct {
		Start string `json:"start"` // Optional: UTC date "2006-01-02"
		End   string `json:"end"`   // Optional: UTC date "2006-01-02"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Allow empty body
		_ = err
	}

	var startDate, endDate time.Time
	var backfillErr error

	if req.Start != "" && req.End != "" {
		// Use provided dates
		startDate, backfillErr = time.Parse("2006-01-02", req.Start)
		if backfillErr != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("invalid start date format: %v", backfillErr),
			})
			return
		}

		endDate, backfillErr = time.Parse("2006-01-02", req.End)
		if backfillErr != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("invalid end date format: %v", backfillErr),
			})
			return
		}
	} else {
		// Calculate from block heights
		startDate, endDate, backfillErr = c.calculateBackfillRange(r.Context(), chainIDUint)
		if backfillErr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("failed to calculate backfill range: %v", backfillErr),
			})
			return
		}
	}

	scheduleID := c.App.TemporalClient.GetLPSnapshotScheduleID(chainIDUint)

	if err := c.triggerLPSnapshotBackfill(r.Context(), chainIDUint, scheduleID, startDate, endDate); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "backfill_triggered",
		"schedule_id": scheduleID,
		"chain_id":    chainIDUint,
		"start_date":  startDate.Format("2006-01-02"),
		"end_date":    endDate.Format("2006-01-02"),
	})
}

// HandleQueryLPSnapshots queries LP position snapshots.
// GET /api/v1/admin/lp-snapshots?chain_id=1&address=0x...&pool_id=1&start_date=2024-01-01&end_date=2024-12-31&active_only=true&limit=100&offset=0
func (c *Controller) HandleQueryLPSnapshots(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	var params crosschainstore.LPPositionSnapshotQueryParams

	// Parse optional filters
	if chainIDStr := query.Get("chain_id"); chainIDStr != "" {
		chainID, err := strconv.ParseUint(chainIDStr, 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chain_id"})
			return
		}
		params.SourceChainID = &chainID
	}

	if address := query.Get("address"); address != "" {
		params.Address = &address
	}

	if poolIDStr := query.Get("pool_id"); poolIDStr != "" {
		poolID, err := strconv.ParseUint(poolIDStr, 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid pool_id"})
			return
		}
		params.PoolID = &poolID
	}

	if startDateStr := query.Get("start_date"); startDateStr != "" {
		startDate, err := time.Parse("2006-01-02", startDateStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid start_date format"})
			return
		}
		params.StartDate = &startDate
	}

	if endDateStr := query.Get("end_date"); endDateStr != "" {
		endDate, err := time.Parse("2006-01-02", endDateStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid end_date format"})
			return
		}
		params.EndDate = &endDate
	}

	if query.Get("active_only") == "true" {
		params.ActiveOnly = true
	}

	if limitStr := query.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 0 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid limit"})
			return
		}
		params.Limit = limit
	}

	if offsetStr := query.Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid offset"})
			return
		}
		params.Offset = offset
	}

	// Query cross-chain database
	if c.App.CrossChainDB == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "cross-chain database not available"})
		return
	}

	snapshots, err := c.App.CrossChainDB.QueryLPPositionSnapshots(r.Context(), params)
	if err != nil {
		c.App.Logger.Error("Failed to query LP snapshots", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if snapshots == nil {
		snapshots = make([]*indexermodels.LPPositionSnapshot, 0)
	}

	_ = json.NewEncoder(w).Encode(snapshots)
}

// calculateBackfillRange calculates the backfill date range from block 1 to current indexed height.
func (c *Controller) calculateBackfillRange(ctx context.Context, chainID uint64) (startDate, endDate time.Time, err error) {
	// Get chain database - reuse admin DB's connection pool
	chainDB := chainstore.NewWithSharedClient(c.App.AdminDB.Client, chainID)
	// Note: No Close() needed - shares admin DB's connection pool

	// Get block 1 for the start date
	block1, err := chainDB.GetBlock(ctx, 1)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("failed to get block 1: %w", err)
	}

	// Get last indexed height
	lastIndexedHeight, err := c.App.AdminDB.LastIndexed(ctx, chainID)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("failed to get last indexed height: %w", err)
	}

	if lastIndexedHeight == 0 {
		return time.Time{}, time.Time{}, fmt.Errorf("no blocks indexed yet for chain %d", chainID)
	}

	lastBlock, err := chainDB.GetBlock(ctx, lastIndexedHeight)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("failed to get last indexed block: %w", err)
	}

	// Convert to calendar dates (UTC)
	startDate = time.Date(block1.Time.Year(), block1.Time.Month(), block1.Time.Day(), 0, 0, 0, 0, time.UTC)
	endDate = time.Date(lastBlock.Time.Year(), lastBlock.Time.Month(), lastBlock.Time.Day(), 0, 0, 0, 0, time.UTC)

	c.App.Logger.Info("Calculated backfill range",
		zap.Uint64("chain_id", chainID),
		zap.Uint64("start_height", 1),
		zap.Uint64("end_height", lastIndexedHeight),
		zap.Time("start_date", startDate),
		zap.Time("end_date", endDate))

	return startDate, endDate, nil
}

// triggerLPSnapshotBackfill triggers a Temporal schedule backfill.
func (c *Controller) triggerLPSnapshotBackfill(ctx context.Context, chainID uint64, scheduleID string, startDate, endDate time.Time) error {
	handle := c.App.TemporalClient.TSClient.GetHandle(ctx, scheduleID)

	c.App.Logger.Info("Triggering LP snapshot backfill",
		zap.Uint64("chain_id", chainID),
		zap.String("schedule_id", scheduleID),
		zap.Time("start_date", startDate),
		zap.Time("end_date", endDate))

	// Use Temporal's built-in schedule backfill
	err := handle.Backfill(ctx, client.ScheduleBackfillOptions{
		Backfill: []client.ScheduleBackfill{
			{
				Start:   startDate,
				End:     endDate,
				Overlap: enums.SCHEDULE_OVERLAP_POLICY_ALLOW_ALL, // Allow parallel execution
			},
		},
	})

	if err != nil {
		c.App.Logger.Error("Failed to trigger backfill",
			zap.Uint64("chain_id", chainID),
			zap.Error(err))
		return fmt.Errorf("backfill trigger failed: %w", err)
	}

	c.App.Logger.Info("LP snapshot backfill triggered successfully",
		zap.Uint64("chain_id", chainID),
		zap.Time("start_date", startDate),
		zap.Time("end_date", endDate))

	return nil
}

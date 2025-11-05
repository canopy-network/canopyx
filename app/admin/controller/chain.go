package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/canopy-network/canopyx/pkg/temporal/indexer"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/canopy-network/canopyx/app/admin/controller/types"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/alitto/pond/v2"
	admintypes "github.com/canopy-network/canopyx/app/admin/types"
	adminstore "github.com/canopy-network/canopyx/pkg/db/admin"
	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"github.com/canopy-network/canopyx/pkg/utils"
	"github.com/go-jose/go-jose/v4/json"
	"github.com/gorilla/mux"
	"github.com/puzpuzpuz/xsync/v4"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	sdktemporal "go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// HandleChainsList returns all registered chains
func (c *Controller) HandleChainsList(w http.ResponseWriter, r *http.Request) {
	cs, err := c.App.AdminDB.ListChain(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if cs == nil {
		cs = make([]admin.Chain, 0)
	}
	_ = json.NewEncoder(w).Encode(cs)
}

// HandleChainsUpsert creates or updates a chain
func (c *Controller) HandleChainsUpsert(w http.ResponseWriter, r *http.Request) {
	var chain admin.Chain
	if err := json.NewDecoder(r.Body).Decode(&chain); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad json"})
		return
	}

	// If ChainID is not provided (0), we'll automatically detect it from RPC endpoints
	// If ChainID is provided, we'll validate it matches the RPC endpoints
	if chain.ChainID == 0 {
		// Validate RPC endpoints are provided for automatic detection
		if len(chain.RPCEndpoints) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "rpc_endpoints required for automatic chain ID detection"})
			return
		}

		// Automatically detect chain ID from RPC endpoints
		detectedChainID, err := rpc.ValidateAndExtractChainID(r.Context(), chain.RPCEndpoints, c.App.Logger)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("failed to validate RPC endpoints and detect chain ID: %v", err),
			})
			return
		}
		chain.ChainID = detectedChainID
		c.App.Logger.Info("Chain ID automatically detected from RPC endpoints",
			zap.Uint64("chain_id", detectedChainID),
		)
	} else {
		// Chain ID provided - validate it matches RPC endpoints if provided
		if len(chain.RPCEndpoints) > 0 {
			detectedChainID, err := rpc.ValidateAndExtractChainID(r.Context(), chain.RPCEndpoints, c.App.Logger)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": fmt.Sprintf("failed to validate RPC endpoints: %v", err),
				})
				return
			}
			if detectedChainID != chain.ChainID {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": fmt.Sprintf("chain ID mismatch: provided %d but RPC endpoints report %d", chain.ChainID, detectedChainID),
				})
				return
			}
			c.App.Logger.Info("Chain ID validated against RPC endpoints",
				zap.Uint64("chain_id", chain.ChainID),
			)
		}
	}

	chain.Image = strings.TrimSpace(chain.Image)
	chain.Notes = strings.TrimSpace(chain.Notes)

	if chain.MinReplicas == 0 {
		chain.MinReplicas = 1
	}
	if chain.MaxReplicas == 0 {
		chain.MaxReplicas = chain.MinReplicas
	}
	if chain.MaxReplicas < chain.MinReplicas {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "max_replicas must be greater than or equal to min_replicas"})
		return
	}
	if chain.Image == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "image required"})
		return
	}

	ctx := r.Context()

	if err := c.App.AdminDB.UpsertChain(ctx, &chain); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	chainIDStr := strconv.FormatUint(chain.ChainID, 10)
	if _, err := c.App.NewChainDb(ctx, chainIDStr); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	err := c.App.EnsureChainSchedules(r.Context(), chainIDStr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"ok": "1"})
}

// HandleChainDetail returns a chain by ID
func (c *Controller) HandleChainDetail(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	chainID, parseErr := strconv.ParseUint(id, 10, 64)
	if parseErr != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chain ID"})
		return
	}
	chain, err := c.App.AdminDB.GetChain(r.Context(), chainID)
	if err != nil {
		w.WriteHeader(404)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		return
	}
	_ = json.NewEncoder(w).Encode(chain)
}

// HandleProgress responds with the last indexed progress of a specific resource identified by its ID.
func (c *Controller) HandleProgress(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	chainID, parseErr := strconv.ParseUint(id, 10, 64)
	if parseErr != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chain ID"})
		return
	}
	h, _ := c.App.AdminDB.LastIndexed(r.Context(), chainID)
	_ = json.NewEncoder(w).Encode(map[string]any{"last": h})
}

// HandleGaps retrieves and sends back gaps identified for a specific resource based on the given ID from the request.
func (c *Controller) HandleGaps(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	chainID, parseErr := strconv.ParseUint(id, 10, 64)
	if parseErr != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chain ID"})
		return
	}
	gs, _ := c.App.AdminDB.FindGaps(r.Context(), chainID)
	_ = json.NewEncoder(w).Encode(gs)
}

// HandleIndexProgressHistory returns time-series indexing progress data for visualization
func (c *Controller) HandleIndexProgressHistory(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing chain id"})
		return
	}

	chainID, parseErr := strconv.ParseUint(id, 10, 64)
	if parseErr != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chain ID"})
		return
	}

	// Parse query parameters for a time window (default: last 24 hours, max 500 points)
	hoursParam := r.URL.Query().Get("hours")
	hours := 24 // default
	if hoursParam != "" {
		if parsed, err := time.ParseDuration(hoursParam + "h"); err == nil {
			hours = int(parsed.Hours())
		}
	}
	if hours > 168 { // max 7 days
		hours = 168
	}
	if hours < 1 {
		hours = 1
	}

	// Query aggregated data in time buckets for efficient charting
	// Use 5-minute buckets for <= 24h, 30-minute for <= 72h, 2-hour for > 72h
	var intervalMinutes int
	if hours <= 24 {
		intervalMinutes = 5
	} else if hours <= 72 {
		intervalMinutes = 30
	} else {
		intervalMinutes = 120
	}

	ctx := r.Context()

	points, pointsErr := c.App.AdminDB.IndexProgressHistory(ctx, chainID, hours, intervalMinutes)
	if pointsErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "query failed"})
		return
	}

	result := make([]types.ResponsePoint, 0, len(points))
	for i, p := range points {
		velocity := 0.0
		if i > 0 {
			timeDiff := p.TimeBucket.Sub(points[i-1].TimeBucket).Minutes()
			heightDiff := float64(p.MaxHeight - points[i-1].MaxHeight)
			if timeDiff > 0 {
				velocity = heightDiff / timeDiff
			}
		}

		result = append(result, types.ResponsePoint{
			Time:              p.TimeBucket.UTC().Format(time.RFC3339),
			Height:            p.MaxHeight,
			AvgLatency:        p.AvgLatency,
			AvgProcessingTime: p.AvgProcessingTime,
			BlocksIndexed:     p.BlocksIndexed,
			Velocity:          velocity,
		})
	}

	_ = json.NewEncoder(w).Encode(result)
}

// HandleChainStatus returns the last indexed height for all known chains.
func (c *Controller) HandleChainStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1) Get chains (for RPC fanout + to ensure we cover all known IDs)
	chains, err := c.App.AdminDB.ListChain(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	out := xsync.NewMap[string, admintypes.ChainStatus]()
	for _, chn := range chains {
		chainIDStr := strconv.FormatUint(chn.ChainID, 10)
		out.Store(chainIDStr, admintypes.ChainStatus{
			ChainID:     chn.ChainID,
			ChainName:   chn.ChainName,
			Image:       chn.Image,
			Notes:       chn.Notes,
			Paused:      chn.Paused != 0,
			Deleted:     chn.Deleted != 0,
			MinReplicas: chn.MinReplicas,
			MaxReplicas: chn.MaxReplicas,
			// Populate health fields from stored chain data
			Health: admintypes.HealthInfo{
				Status:    chn.OverallHealthStatus,
				Message:   "", // Overall health doesn't have a message
				UpdatedAt: chn.OverallHealthUpdatedAt,
			},
			RPCHealth: admintypes.HealthInfo{
				Status:    chn.RPCHealthStatus,
				Message:   chn.RPCHealthMessage,
				UpdatedAt: chn.RPCHealthUpdatedAt,
			},
			QueueHealth: admintypes.HealthInfo{
				Status:    chn.QueueHealthStatus,
				Message:   chn.QueueHealthMessage,
				UpdatedAt: chn.QueueHealthUpdatedAt,
			},
			DeploymentHealth: admintypes.HealthInfo{
				Status:    chn.DeploymentHealthStatus,
				Message:   chn.DeploymentHealthMessage,
				UpdatedAt: chn.DeploymentHealthUpdatedAt,
			},
		})
	}

	// 2) Bulk last-indexed via aggregate table
	// SELECT chain_id, maxMerge(max_height) FROM <db>.index_progress_agg GROUP BY chain_id
	{
		dbName := c.App.AdminDB.Name
		q := fmt.Sprintf(`
			SELECT chain_id, maxMerge(max_height) AS last_idx
			FROM "%s"."index_progress_agg"
			GROUP BY chain_id
		`, dbName)

		rows, err := c.App.AdminDB.Db.Query(ctx, q)
		if err == nil {
			defer func(rows driver.Rows) {
				err := rows.Close()
				if err != nil {
					c.App.Logger.Error("Failed to close rows", zap.Error(err))
				}
			}(rows)
			for rows.Next() {
				var chainID uint64
				var last uint64
				if err := rows.Scan(&chainID, &last); err != nil {
					c.App.Logger.Warn("Failed to scan index progress row", zap.Error(err))
					continue
				}
				id := fmt.Sprintf("%d", chainID)
				cs, ok := out.Load(id)
				if !ok {
					cs = admintypes.ChainStatus{ChainID: chainID}
				}
				cs.LastIndexed = last
				out.Store(id, cs)
			}
			_ = rows.Err()
		}
	}

	// 3) Fallback for any chain missing in the aggregate: max(height) from base table
	{
		dbName := c.App.AdminDB.Name
		q := fmt.Sprintf(`
			SELECT chain_id, max(height) AS last_idx
			FROM "%s"."index_progress"
			GROUP BY chain_id
		`, dbName)

		rows, err := c.App.AdminDB.Db.Query(ctx, q)
		if err == nil {
			defer func(rows driver.Rows) {
				err := rows.Close()
				if err != nil {
					c.App.Logger.Error("Failed to close rows", zap.Error(err))
				}
			}(rows)
			for rows.Next() {
				var chainID uint64
				var last uint64
				if err := rows.Scan(&chainID, &last); err != nil {
					c.App.Logger.Warn("Failed to scan fallback index progress row", zap.Error(err))
					continue
				}
				id := fmt.Sprintf("%d", chainID)
				cs, ok := out.Load(id)
				if !ok {
					cs = admintypes.ChainStatus{ChainID: chainID}
				}
				if cs.LastIndexed == 0 {
					cs.LastIndexed = last
					out.Store(id, cs)
				}
			}
			_ = rows.Err()
		}
	}

	// 4) Fetch heads via RPC and gaps in parallel using pond worker pool
	// Create worker pool for concurrent fetching (8 workers, queue size to accommodate all tasks)
	maxWorkers := 8
	totalTasks := len(chains) * 2 // head + gaps for each chain
	queueSize := totalTasks
	if queueSize < 16 {
		queueSize = 16 // minimum queue size
	}

	pool := pond.NewPool(maxWorkers, pond.WithQueueSize(queueSize))
	group := pool.NewGroupContext(ctx)
	groupCtx := group.Context()

	// Submit head fetching tasks
	for _, chn := range chains {
		chn := chn
		group.Submit(func() {
			if err := groupCtx.Err(); err != nil {
				return
			}

			fetchCtx, cancel := context.WithTimeout(groupCtx, 3*time.Second)
			defer cancel()

			cli := rpc.NewHTTPWithOpts(rpc.Opts{Endpoints: chn.RPCEndpoints})
			head, err := cli.ChainHead(fetchCtx)
			if err != nil {
				return
			}

			// Verify head block is actually available before using it
			// This prevents scheduling workflows for blocks that aren't ready yet
			// Same logic as GetLatestHead activity in pkg/indexer/activity/ops.go
			if head > 0 {
				_, err := cli.BlockByHeight(fetchCtx, head)
				if err != nil {
					// Head block not available yet, use head-1 instead
					chainIDStr := strconv.FormatUint(chn.ChainID, 10)
					c.App.Logger.Debug("Head block not yet available in HandleChainStatus, using head-1",
						zap.String("chain_id", chainIDStr),
						zap.Uint64("reported_head", head),
						zap.Uint64("adjusted_head", head-1),
						zap.Error(err),
					)
					head = head - 1
				}
			}

			chainIDStr := strconv.FormatUint(chn.ChainID, 10)
			cs, ok := out.Load(chainIDStr)
			if !ok {
				cs = admintypes.ChainStatus{ChainID: chn.ChainID}
			}
			cs.Head = head
			out.Store(chainIDStr, cs)
		})
	}

	// Submit gap fetching tasks
	for _, chn := range chains {
		chn := chn
		group.Submit(func() {
			if err := groupCtx.Err(); err != nil {
				return
			}

			// Fetch gaps from database
			gaps, err := c.App.AdminDB.FindGaps(groupCtx, chn.ChainID)
			chainIDStr := strconv.FormatUint(chn.ChainID, 10)
			if err != nil {
				c.App.Logger.Warn("failed to fetch gaps",
					zap.String("chain_id", chainIDStr),
					zap.Error(err))
				return
			}

			// Calculate gap summary statistics
			var missingCount uint64
			var largestGapStart, largestGapEnd uint64
			var largestGapSize uint64

			for _, gap := range gaps {
				gapSize := gap.To - gap.From + 1
				missingCount += gapSize

				// Track largest gap
				if gapSize > largestGapSize {
					largestGapSize = gapSize
					largestGapStart = gap.From
					largestGapEnd = gap.To
				}
			}

			// Update chain status with gap information
			cs, ok := out.Load(chainIDStr)
			if !ok {
				cs = admintypes.ChainStatus{ChainID: chn.ChainID}
			}
			cs.MissingBlocksCount = missingCount
			cs.GapRangesCount = len(gaps)
			cs.LargestGapStart = largestGapStart
			cs.LargestGapEnd = largestGapEnd
			out.Store(chainIDStr, cs)
		})
	}

	// Wait for all head and gap fetching tasks to complete
	if err := group.Wait(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, pond.ErrGroupStopped) {
		c.App.Logger.Warn("some chain status tasks failed", zap.Error(err))
	}

	// 5) Fetch queue metrics for both ops and indexer queues
	if c.App.TemporalClient != nil {
		for _, chn := range chains {
			chainIDStr := strconv.FormatUint(chn.ChainID, 10)
			opsStats, indexerStats, err := c.describeBothQueues(ctx, chainIDStr)
			if err != nil {
				c.App.Logger.Warn("describe queues failed", zap.String("chain_id", chainIDStr), zap.Error(err))
				continue
			}
			cs, _ := out.Load(chainIDStr)
			cs.OpsQueue = opsStats
			cs.IndexerQueue = indexerStats
			// For backward compatibility, populate deprecated Queue field with IndexerQueue
			cs.Queue = indexerStats

			// Populate dual-queue fields from cache
			// These will be 0 until dual-queue architecture is deployed
			if cached, ok := c.App.QueueStatsCache.Load(chainIDStr); ok {
				// Live queue depth = total pending tasks (workflows + activities)
				cs.LiveQueueDepth = cached.LiveQueue.PendingWorkflow + cached.LiveQueue.PendingActivity
				cs.LiveQueueBacklogAge = cached.LiveQueue.BacklogAgeSecs

				// Historical queue depth = total pending tasks (workflows + activities)
				cs.HistoricalQueueDepth = cached.HistoricalQueue.PendingWorkflow + cached.HistoricalQueue.PendingActivity
				cs.HistoricalQueueBacklogAge = cached.HistoricalQueue.BacklogAgeSecs
			}

			out.Store(chainIDStr, cs)
		}
	}

	for _, chn := range chains {
		chainIDStr := strconv.FormatUint(chn.ChainID, 10)
		history, err := c.App.AdminDB.ListReindexRequests(ctx, chn.ChainID, 10)
		if err != nil {
			c.App.Logger.Warn("list reindex history failed", zap.String("chain_id", chainIDStr), zap.Error(err))
			continue
		}
		records := make([]admintypes.ReindexEntry, 0, len(history))
		for _, h := range history {
			records = append(records, admintypes.ReindexEntry{
				Height:      h.Height,
				Status:      h.Status,
				RequestedBy: h.RequestedBy,
				RequestedAt: h.RequestedAt,
				WorkflowID:  h.WorkflowID,
				RunID:       h.RunID,
			})
		}
		cs, _ := out.Load(chainIDStr)
		cs.ReindexHistory = records
		out.Store(chainIDStr, cs)
	}

	// 6) Calculate IsLiveSync for each chain after all data has been loaded
	// IsLiveSync indicates whether the chain is caught up with the latest blocks (within 2 blocks of head)
	out.Range(func(key string, cs admintypes.ChainStatus) bool {
		if cs.Head > 0 && cs.LastIndexed > 0 {
			// Chain is considered "live synced" if last indexed is within 2 blocks of head
			cs.IsLiveSync = (cs.Head-cs.LastIndexed <= 2)
			out.Store(key, cs)
		}
		return true
	})

	// Convert xsync.Map to regular map for JSON encoding
	result := make(map[string]admintypes.ChainStatus)
	out.Range(func(key string, value admintypes.ChainStatus) bool {
		result[key] = value
		return true
	})

	_ = json.NewEncoder(w).Encode(result)
}

func (c *Controller) HandleTriggerHeadScan(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing chain id"})
		return
	}
	user := c.currentUser(r)
	if err := c.startOpsWorkflow(r.Context(), id, indexer.HeadScanWorkflowName); err != nil {
		c.App.Logger.Error("headscan trigger failed", zap.String("chain_id", id), zap.String("user", user), zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	c.App.Logger.Info("headscan triggered", zap.String("chain_id", id), zap.String("user", user))
	_ = json.NewEncoder(w).Encode(map[string]string{"ok": "1"})
}

func (c *Controller) HandleTriggerGapScan(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing chain id"})
		return
	}
	user := c.currentUser(r)
	if err := c.startOpsWorkflow(r.Context(), id, indexer.GapScanWorkflowName); err != nil {
		c.App.Logger.Error("gapscan trigger failed", zap.String("chain_id", id), zap.String("user", user), zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	c.App.Logger.Info("gapscan triggered", zap.String("chain_id", id), zap.String("user", user))
	_ = json.NewEncoder(w).Encode(map[string]string{"ok": "1"})
}

// HandleReindex processes a reindex request for the specified chain, validates input, and enqueues blocks for indexing.
func (c *Controller) HandleReindex(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing chain id"})
		return
	}

	var req types.ReindexRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid json"})
		return
	}

	heights := make([]uint64, 0)
	seen := make(map[uint64]struct{})
	addHeight := func(h uint64) {
		if _, ok := seen[h]; !ok {
			seen[h] = struct{}{}
			heights = append(heights, h)
		}
	}

	for _, h := range req.Heights {
		addHeight(h)
	}

	if req.From != nil && req.To != nil {
		from := *req.From
		to := *req.To
		if to < from {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid range"})
			return
		}
		if to-from > 500 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "range too large"})
			return
		}
		for h := from; h <= to; h++ {
			addHeight(h)
		}
	}

	if len(heights) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "no heights specified"})
		return
	}

	if len(heights) > 500 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "too many heights"})
		return
	}

	user := c.currentUser(r)

	// Enqueue workflows and collect execution info
	workflowInfos := make([]adminstore.ReindexWorkflowInfo, 0, len(heights))

	for _, h := range heights {
		workflowID, runID, err := c.enqueueIndexBlock(r.Context(), id, h, true)
		if err != nil {
			c.App.Logger.Error("reindex enqueue failed", zap.String("chain_id", id), zap.Uint64("height", h), zap.String("user", user), zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		workflowInfos = append(workflowInfos, adminstore.ReindexWorkflowInfo{
			Height:     h,
			WorkflowID: workflowID,
			RunID:      runID,
		})
	}

	// Record all reindex requests with workflow info
	chainIDUint, parseErr := strconv.ParseUint(id, 10, 64)
	if parseErr != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chain ID"})
		return
	}
	if err := c.App.AdminDB.RecordReindexRequestsWithWorkflow(r.Context(), chainIDUint, user, workflowInfos); err != nil {
		c.App.Logger.Warn("failed to record reindex history", zap.String("chain_id", id), zap.Error(err))
	}

	c.App.Logger.Info("reindex queued", zap.String("chain_id", id), zap.String("user", user), zap.Int("count", len(heights)), zap.Any("heights", heights))
	_ = json.NewEncoder(w).Encode(map[string]any{"queued": len(heights)})
}

// startOpsWorkflow starts a Temporal workflow to perform operations related to a specific chain and workflow name.
// It dynamically determines the input structure based on the workflow name and uses a Temporal client to execute it.
// Returns an error if a Temporal client is unavailable, chainID is invalid, or workflow execution fails.
func (c *Controller) startOpsWorkflow(ctx context.Context, chainID, workflowName string) error {
	tc := c.App.TemporalClient
	if tc == nil {
		return fmt.Errorf("temporal client unavailable")
	}

	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid chain ID format: %w", parseErr)
	}

	// Use the correct input type based on workflow name
	var input interface{}
	switch workflowName {
	case indexer.HeadScanWorkflowName:
		input = indexer.HeadScanInput{ChainID: chainIDUint}
	case indexer.GapScanWorkflowName:
		input = indexer.GapScanInput{ChainID: chainIDUint}
	default:
		// Fallback to old input type for unknown workflows
		input = indexer.ChainIdInput{ChainID: chainIDUint}
	}

	options := client.StartWorkflowOptions{
		ID:        fmt.Sprintf("%s:%s:%d", chainID, strings.ToLower(workflowName), time.Now().UnixNano()),
		TaskQueue: tc.GetIndexerOpsQueue(chainIDUint),
	}

	_, err := tc.TClient.ExecuteWorkflow(ctx, options, workflowName, input)
	return err
}

// enqueueIndexBlock enqueues a block for indexing by determining the proper queue based on its age and reindex flag.
// It communicates with the Temporal service to initiate the indexing process and determines queue routing dynamically.
// Returns the workflow ID, run ID, or an error if the Temporal client is unavailable or an issue occurs during execution.
func (c *Controller) enqueueIndexBlock(ctx context.Context, chainID string, height uint64, reindex bool) (string, string, error) {
	tc := c.App.TemporalClient
	if tc == nil {
		return "", "", fmt.Errorf("temporal client unavailable")
	}

	// Convert string chainID to uint64
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return "", "", fmt.Errorf("invalid chain ID format: %w", parseErr)
	}

	// Get latest head to determine queue routing (same logic as StartIndexWorkflow activity)
	var taskQueue string
	latest, err := c.App.AdminDB.LastIndexed(ctx, chainIDUint)
	if err != nil {
		// Failed to get latest, check if we can get it from RPC
		ch, err := c.App.AdminDB.GetChain(ctx, chainIDUint)
		if err == nil && len(ch.RPCEndpoints) > 0 {
			cli := rpc.NewHTTPWithOpts(rpc.Opts{Endpoints: ch.RPCEndpoints})
			if head, err := cli.ChainHead(ctx); err == nil {
				latest = head
			}
		}
		// If still no latest, default to historical queue for manual reindex
		if latest == 0 {
			c.App.Logger.Debug("Unable to determine latest head for queue routing, defaulting to historical queue",
				zap.String("chain_id", chainID),
				zap.Uint64("height", height),
			)
		}
	}

	// Determine target queue based on block age using the same logic as ops.go
	if indexer.IsLiveBlock(latest, height) {
		taskQueue = tc.GetIndexerLiveQueue(chainIDUint)
		c.App.Logger.Debug("Manual reindex routing to live queue",
			zap.String("chain_id", chainID),
			zap.String("task_queue", taskQueue),
			zap.Uint64("latest", latest),
			zap.Uint64("height", height),
		)
	} else {
		taskQueue = tc.GetIndexerHistoricalQueue(chainIDUint)
		c.App.Logger.Debug("Manual reindex routing to historical queue",
			zap.String("chain_id", chainID),
			zap.String("task_queue", taskQueue),
			zap.Uint64("latest", latest),
			zap.Uint64("height", height),
		)
	}

	input := indexer.IndexBlockInput{ChainID: chainIDUint, Height: height, Reindex: reindex, PriorityKey: "1"}
	options := client.StartWorkflowOptions{
		ID:        c.App.TemporalClient.GetIndexBlockWorkflowIdWithTime(chainIDUint, height),
		TaskQueue: taskQueue, // Use the dynamically determined queue
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 1.2,
			MaximumInterval:    5 * time.Second,
			MaximumAttempts:    0,
		},
	}

	run, err := tc.TClient.ExecuteWorkflow(ctx, options, "IndexBlockWorkflow", input)
	if err != nil {
		return "", "", err
	}
	return run.GetID(), run.GetRunID(), nil
}

// describeBothQueues fetches metrics for both the ops queue and indexer queue for a given chain.
// It uses caching with a 30s TTL to reduce load on Temporal API.
// Returns (opsQueue, indexerQueue, error).
func (c *Controller) describeBothQueues(ctx context.Context, chainID string) (admintypes.QueueStatus, admintypes.QueueStatus, error) {
	opsStats := admintypes.QueueStatus{}
	indexerStats := admintypes.QueueStatus{}

	temporalClient := c.App.TemporalClient
	if temporalClient == nil {
		c.App.Logger.Error("describeBothQueues: temporal client not initialized", zap.String("chain_id", chainID))
		return opsStats, indexerStats, fmt.Errorf("temporal temporalClient not initialized")
	}

	// Check cache first (30s TTL to reduce Temporal API load)
	if cached, ok := c.App.QueueStatsCache.Load(chainID); ok {
		cacheAge := time.Since(cached.Fetched)
		if cacheAge < 30*time.Second {
			c.App.Logger.Debug("using cached queue stats",
				zap.String("chain_id", chainID),
				zap.Duration("cache_age", cacheAge),
				zap.Int64("ops_pending_workflows", cached.OpsQueue.PendingWorkflow),
			)
			// Return aggregated indexer stats (live + historical combined)
			aggregatedIndexer := admintypes.QueueStatus{
				PendingWorkflow: cached.LiveQueue.PendingWorkflow + cached.HistoricalQueue.PendingWorkflow,
				PendingActivity: cached.LiveQueue.PendingActivity + cached.HistoricalQueue.PendingActivity,
				Pollers:         cached.LiveQueue.Pollers + cached.HistoricalQueue.Pollers,
				BacklogAgeSecs:  cached.LiveQueue.BacklogAgeSecs,
			}
			// Use max backlog age (worst case)
			if cached.HistoricalQueue.BacklogAgeSecs > aggregatedIndexer.BacklogAgeSecs {
				aggregatedIndexer.BacklogAgeSecs = cached.HistoricalQueue.BacklogAgeSecs
			}
			return cached.OpsQueue, aggregatedIndexer, nil
		}
	}

	reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	// Query both queues in parallel for better performance using the enhanced API
	type queueResult struct {
		stats           admintypes.QueueStatus
		liveStats       admintypes.QueueStatus // Individual live queue stats
		historicalStats admintypes.QueueStatus // Individual historical queue stats
		err             error
	}

	opsChannel := make(chan queueResult, 1)
	indexerChannel := make(chan queueResult, 1)

	// Convert string chainID to uint64 for temporal methods
	chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
	if parseErr != nil {
		return admintypes.QueueStatus{}, admintypes.QueueStatus{}, fmt.Errorf("invalid chain ID format: %w", parseErr)
	}

	// Query ops queue using the new common function
	go func() {
		queueName := temporalClient.GetIndexerOpsQueue(chainIDUint)
		c.App.Logger.Debug("querying ops queue", zap.String("chain_id", chainID), zap.String("queue_name", queueName))

		// Use the common GetQueueStats function with enhanced API
		pendingWorkflows, pendingActivities, pollerCount, backlogAge, err := temporalClient.GetQueueStats(reqCtx, queueName)
		if err != nil {
			opsChannel <- queueResult{
				stats:           opsStats,
				liveStats:       admintypes.QueueStatus{},
				historicalStats: admintypes.QueueStatus{},
				err:             err,
			}
			return
		}

		stats := admintypes.QueueStatus{
			PendingWorkflow: pendingWorkflows,
			PendingActivity: pendingActivities,
			Pollers:         pollerCount,
			BacklogAgeSecs:  backlogAge,
		}
		opsChannel <- queueResult{
			stats:           stats,
			liveStats:       admintypes.QueueStatus{},
			historicalStats: admintypes.QueueStatus{},
			err:             nil,
		}
	}()

	// Query BOTH live and historical queues and aggregate them for backward compatibility
	go func() {
		// Get stats from both live and historical queues
		liveQueue := temporalClient.GetIndexerLiveQueue(chainIDUint)
		historicalQueue := temporalClient.GetIndexerHistoricalQueue(chainIDUint)

		c.App.Logger.Debug("querying both indexer queues",
			zap.String("chain_id", chainID),
			zap.String("live_queue", liveQueue),
			zap.String("historical_queue", historicalQueue))

		// Query live queue
		livePendingWF, livePendingAct, livePollers, liveBacklogAge, liveErr := temporalClient.GetQueueStats(reqCtx, liveQueue)
		if liveErr != nil {
			c.App.Logger.Warn("failed to query live queue stats",
				zap.String("chain_id", chainID),
				zap.String("queue", liveQueue),
				zap.Error(liveErr))
			// Set to zero on error, continue with historical
			livePendingWF = 0
			livePendingAct = 0
			livePollers = 0
			liveBacklogAge = 0
		}

		// Query historical queue
		histPendingWF, histPendingAct, histPollers, histBacklogAge, histErr := temporalClient.GetQueueStats(reqCtx, historicalQueue)
		if histErr != nil {
			c.App.Logger.Warn("failed to query historical queue stats",
				zap.String("chain_id", chainID),
				zap.String("queue", historicalQueue),
				zap.Error(histErr))
			// Set to zero on error
			histPendingWF = 0
			histPendingAct = 0
			histPollers = 0
			histBacklogAge = 0
		}

		// Return error only if BOTH queues failed
		if liveErr != nil && histErr != nil {
			indexerChannel <- queueResult{
				stats:           indexerStats,
				liveStats:       admintypes.QueueStatus{},
				historicalStats: admintypes.QueueStatus{},
				err:             fmt.Errorf("both queues failed: live=%v, historical=%v", liveErr, histErr),
			}
			return
		}

		// Create individual queue stats
		liveQueueStats := admintypes.QueueStatus{
			PendingWorkflow: livePendingWF,
			PendingActivity: livePendingAct,
			Pollers:         livePollers,
			BacklogAgeSecs:  liveBacklogAge,
		}
		historicalQueueStats := admintypes.QueueStatus{
			PendingWorkflow: histPendingWF,
			PendingActivity: histPendingAct,
			Pollers:         histPollers,
			BacklogAgeSecs:  histBacklogAge,
		}

		// Aggregate stats from both queues
		totalPendingWF := livePendingWF + histPendingWF
		totalPendingAct := livePendingAct + histPendingAct
		totalPollers := livePollers + histPollers

		// Use max backlog age (worst case)
		maxBacklogAge := liveBacklogAge
		if histBacklogAge > maxBacklogAge {
			maxBacklogAge = histBacklogAge
		}

		// Return aggregated stats for backward compatibility, plus individual stats
		aggregatedStats := admintypes.QueueStatus{
			PendingWorkflow: totalPendingWF,
			PendingActivity: totalPendingAct,
			Pollers:         totalPollers,
			BacklogAgeSecs:  maxBacklogAge,
		}
		indexerChannel <- queueResult{
			stats:           aggregatedStats,
			liveStats:       liveQueueStats,
			historicalStats: historicalQueueStats,
			err:             nil,
		}
	}()

	// Collect results
	opsResult := <-opsChannel
	indexerResult := <-indexerChannel

	// If either query failed, log the detailed error and return
	if opsResult.err != nil {
		c.App.Logger.Error("failed to describe ops queue",
			zap.String("chain_id", chainID),
			zap.String("queue_name", temporalClient.GetIndexerOpsQueue(chainIDUint)),
			zap.Error(opsResult.err),
		)
		return opsStats, indexerStats, fmt.Errorf("failed to describe ops queue: %w", opsResult.err)
	}
	if indexerResult.err != nil {
		c.App.Logger.Error("failed to describe indexer queues",
			zap.String("chain_id", chainID),
			zap.String("live_queue", temporalClient.GetIndexerLiveQueue(chainIDUint)),
			zap.String("historical_queue", temporalClient.GetIndexerHistoricalQueue(chainIDUint)),
			zap.Error(indexerResult.err),
		)
		return opsStats, indexerStats, fmt.Errorf("failed to describe indexer queues: %w", indexerResult.err)
	}

	opsStats = opsResult.stats
	indexerStats = indexerResult.stats

	// Log final combined stats
	c.App.Logger.Info("queue stats fetched successfully",
		zap.String("chain_id", chainID),
		zap.Int64("ops_pending_workflows", opsStats.PendingWorkflow),
		zap.Int64("ops_pending_activities", opsStats.PendingActivity),
		zap.Int("ops_pollers", opsStats.Pollers),
		zap.Int64("indexer_pending_workflows", indexerStats.PendingWorkflow),
		zap.Int64("indexer_pending_activities", indexerStats.PendingActivity),
		zap.Int("indexer_pollers", indexerStats.Pollers),
	)

	// Store all queues in cache (ops + individual live + historical stats)
	c.App.QueueStatsCache.Store(chainID, admintypes.CachedQueueStats{
		OpsQueue:        opsStats,
		LiveQueue:       indexerResult.liveStats,
		HistoricalQueue: indexerResult.historicalStats,
		Fetched:         time.Now(),
	})

	return opsStats, indexerStats, nil
}

// HandleChainPatch updates a chain by ID
func (c *Controller) HandleChainPatch(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	// 1) Load the current row (so we can do a partial update)
	chainIDUint, parseErr := strconv.ParseUint(id, 10, 64)
	if parseErr != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chain ID"})
		return
	}
	cur, err := c.App.AdminDB.GetChain(r.Context(), chainIDUint)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		return
	}

	// 2) Decode patch body
	var in admin.Chain
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad json"})
		return
	}

	// 3) Apply changes
	if in.ChainName != cur.ChainName {
		cur.ChainName = strings.TrimSpace(in.ChainName)
	}

	if in.Paused != cur.Paused {
		cur.Paused = in.Paused
	}
	if in.Deleted != cur.Deleted {
		cur.Deleted = in.Deleted
	}
	if in.RPCEndpoints != nil { // provided => replace
		cleaned := make([]string, 0, len(in.RPCEndpoints))
		for _, e := range in.RPCEndpoints {
			e = strings.TrimSpace(e)
			if e != "" {
				cleaned = append(cleaned, e)
			}
		}
		cur.RPCEndpoints = utils.Dedup(cleaned)
	}

	// Update MinReplicas if provided (non-zero value)
	if in.MinReplicas > 0 {
		cur.MinReplicas = in.MinReplicas
	}

	// Update MaxReplicas if provided (non-zero value)
	if in.MaxReplicas > 0 {
		cur.MaxReplicas = in.MaxReplicas
	}

	// Validate that MaxReplicas >= MinReplicas after updates
	if cur.MaxReplicas < cur.MinReplicas {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "max_replicas must be greater than or equal to min_replicas"})
		return
	}

	// 4) Persist (ReplacingMergeTree upsert)
	if upsertErr := c.App.AdminDB.UpsertChain(r.Context(), cur); upsertErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": upsertErr.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"ok": "1"})
}

// HandlePatchChainsStatus PATCH /admin/chains/status
func (c *Controller) HandlePatchChainsStatus(w http.ResponseWriter, r *http.Request) {
	var reqs []admin.Chain
	if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if len(reqs) == 0 {
		http.Error(w, "empty patch list", http.StatusBadRequest)
		return
	}

	// Convert to DB layer patches and execute
	patches := make([]admin.Chain, 0, len(reqs))
	for _, x := range reqs {
		if x.ChainID == 0 {
			http.Error(w, "chain_id is required", http.StatusBadRequest)
			return
		}

		if x.RPCEndpoints != nil {
			x.RPCEndpoints = utils.Dedup(x.RPCEndpoints)
		}

		patches = append(patches, x)
	}

	if err := c.App.AdminDB.PatchChains(r.Context(), patches); err != nil {
		// Map a not-found to 404, everything else 500
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleChainDelete handles the deletion of a chain by marking it as deleted,
// removing Temporal schedules, and optionally dropping the chain database.
func (c *Controller) HandleChainDelete(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing chain id"})
		return
	}

	ctx := r.Context()

	// 1. Verify chain exists
	chainIDUint, parseErr := strconv.ParseUint(id, 10, 64)
	if parseErr != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chain ID"})
		return
	}
	chain, err := c.App.AdminDB.GetChain(ctx, chainIDUint)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "chain not found"})
		return
	}

	user := c.currentUser(r)
	c.App.Logger.Info("deleting chain", zap.String("chain_id", id), zap.String("user", user))

	// 2. Delete Temporal schedules for this chain
	if c.App.TemporalClient != nil {
		// Delete head scan schedule
		headScheduleID := c.App.TemporalClient.GetHeadScheduleID(chainIDUint)
		headHandle := c.App.TemporalClient.TSClient.GetHandle(ctx, headScheduleID)
		if err := headHandle.Delete(ctx); err != nil {
			var notFound *serviceerror.NotFound
			if !errors.As(err, &notFound) {
				c.App.Logger.Warn("failed to delete head scan schedule", zap.String("chain_id", id), zap.Error(err))
			}
		}

		// Delete gap scan schedule
		gapScheduleID := c.App.TemporalClient.GetGapScanScheduleID(chainIDUint)
		gapHandle := c.App.TemporalClient.TSClient.GetHandle(ctx, gapScheduleID)
		if err := gapHandle.Delete(ctx); err != nil {
			var notFound *serviceerror.NotFound
			if !errors.As(err, &notFound) {
				c.App.Logger.Warn("failed to delete gap scan schedule", zap.String("chain_id", id), zap.Error(err))
			}
		}
	}

	// 3. Mark chain as deleted in admin database
	chain.Deleted = 1
	if err := c.App.AdminDB.UpsertChain(ctx, chain); err != nil {
		c.App.Logger.Error("failed to mark chain as deleted", zap.String("chain_id", id), zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to delete chain"})
		return
	}

	// 4. Remove chain database from in-memory cache
	c.App.ChainsDB.Delete(id)

	// 5. Clear queue stats cache for this chain
	c.App.QueueStatsCache.Delete(id)

	c.App.Logger.Info("chain deleted successfully", zap.String("chain_id", id), zap.String("user", user))
	_ = json.NewEncoder(w).Encode(map[string]string{"ok": "1"})
}

package controller

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "net/http"
    "strings"
    "time"

    admintypes "github.com/canopy-network/canopyx/app/admin/types"
    "github.com/canopy-network/canopyx/pkg/db"
    "github.com/canopy-network/canopyx/pkg/db/models/admin"
    indexertypes "github.com/canopy-network/canopyx/pkg/indexer/types"
    indexerworkflow "github.com/canopy-network/canopyx/pkg/indexer/workflow"
    "github.com/canopy-network/canopyx/pkg/rpc"
    "github.com/canopy-network/canopyx/pkg/utils"
    "github.com/alitto/pond/v2"
    "github.com/go-jose/go-jose/v4/json"
    "github.com/gorilla/mux"
    "github.com/puzpuzpuz/xsync/v4"
    "github.com/uptrace/go-clickhouse/ch"
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
    if chain.ChainID == "" {
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "chain_id required"})
        return
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

    if _, err := c.App.NewChainDb(ctx, chain.ChainID); err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
        return
    }

    err := c.App.EnsureChainSchedules(r.Context(), chain.ChainID)
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
    chain, err := c.App.AdminDB.GetChain(r.Context(), id)
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
    h, _ := c.App.AdminDB.LastIndexed(r.Context(), id)
    _ = json.NewEncoder(w).Encode(map[string]any{"last": h})
}

// HandleGaps retrieves and sends back gaps identified for a specific resource based on the given ID from the request.
func (c *Controller) HandleGaps(w http.ResponseWriter, r *http.Request) {
    id := mux.Vars(r)["id"]
    gs, _ := c.App.AdminDB.FindGaps(r.Context(), id)
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

    // Parse query parameters for time window (default: last 24 hours, max 500 points)
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

    ctx := r.Context()
    dbName := c.App.AdminDB.Name

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

    query := fmt.Sprintf(`
        SELECT
            toStartOfInterval(indexed_at, INTERVAL %d MINUTE) AS time_bucket,
            max(height) AS max_height,
            avg(indexing_time) AS avg_latency,
            avg(indexing_time_ms) AS avg_processing_time,
            count() AS blocks_indexed
        FROM "%s"."index_progress"
        WHERE chain_id = ?
          AND indexed_at >= now() - INTERVAL %d HOUR
        GROUP BY time_bucket
        ORDER BY time_bucket ASC
    `, intervalMinutes, dbName, hours)

    type ProgressPoint struct {
        TimeBucket        time.Time `ch:"time_bucket"`
        MaxHeight         uint64    `ch:"max_height"`
        AvgLatency        float64   `ch:"avg_latency"`
        AvgProcessingTime float64   `ch:"avg_processing_time"`
        BlocksIndexed     uint64    `ch:"blocks_indexed"`
    }

    var points []ProgressPoint
    if err := c.App.AdminDB.Db.NewRaw(query, id).Scan(ctx, &points); err != nil {
        c.App.Logger.Error("query index progress history failed", zap.String("chain_id", id), zap.Error(err))
        w.WriteHeader(http.StatusInternalServerError)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "query failed"})
        return
    }

    // Format response
    type ResponsePoint struct {
        Time              string  `json:"time"`
        Height            uint64  `json:"height"`
        AvgLatency        float64 `json:"avg_latency"`         // seconds
        AvgProcessingTime float64 `json:"avg_processing_time"` // milliseconds
        BlocksIndexed     uint64  `json:"blocks_indexed"`
        Velocity          float64 `json:"velocity"` // blocks per minute
    }

    result := make([]ResponsePoint, 0, len(points))
    for i, p := range points {
        velocity := 0.0
        if i > 0 {
            timeDiff := p.TimeBucket.Sub(points[i-1].TimeBucket).Minutes()
            heightDiff := float64(p.MaxHeight - points[i-1].MaxHeight)
            if timeDiff > 0 {
                velocity = heightDiff / timeDiff
            }
        }

        result = append(result, ResponsePoint{
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
        out.Store(chn.ChainID, admintypes.ChainStatus{
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

        rows, err := c.App.AdminDB.Db.QueryContext(ctx, q)
        if err == nil {
            defer func(rows *ch.Rows) {
                err := rows.Close()
                if err != nil {
                    c.App.Logger.Error("Failed to close rows", zap.Error(err))
                }
            }(rows)
            for rows.Next() {
                var id string
                var last uint64
                if err := rows.Scan(&id, &last); err != nil {
                    continue
                }
                cs, ok := out.Load(id)
                if !ok {
                    cs = admintypes.ChainStatus{ChainID: id}
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

        rows, err := c.App.AdminDB.Db.QueryContext(ctx, q)
        if err == nil {
            defer func(rows *ch.Rows) {
                err := rows.Close()
                if err != nil {
                    c.App.Logger.Error("Failed to close rows", zap.Error(err))
                }
            }(rows)
            for rows.Next() {
                var id string
                var last uint64
                if err := rows.Scan(&id, &last); err != nil {
                    continue
                }
                cs, ok := out.Load(id)
                if !ok {
                    cs = admintypes.ChainStatus{ChainID: id}
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

            cs, ok := out.Load(chn.ChainID)
            if !ok {
                cs = admintypes.ChainStatus{ChainID: chn.ChainID}
            }
            cs.Head = head
            out.Store(chn.ChainID, cs)
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
            if err != nil {
                c.App.Logger.Warn("failed to fetch gaps",
                    zap.String("chain_id", chn.ChainID),
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
            cs, ok := out.Load(chn.ChainID)
            if !ok {
                cs = admintypes.ChainStatus{ChainID: chn.ChainID}
            }
            cs.MissingBlocksCount = missingCount
            cs.GapRangesCount = len(gaps)
            cs.LargestGapStart = largestGapStart
            cs.LargestGapEnd = largestGapEnd
            out.Store(chn.ChainID, cs)
        })
    }

    // Wait for all head and gap fetching tasks to complete
    if err := group.Wait(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, pond.ErrGroupStopped) {
        c.App.Logger.Warn("some chain status tasks failed", zap.Error(err))
    }

    // 5) Fetch queue metrics for both ops and indexer queues
    if c.App.TemporalClient != nil {
        for _, chn := range chains {
            opsStats, indexerStats, err := c.describeBothQueues(ctx, chn.ChainID)
            if err != nil {
                c.App.Logger.Warn("describe queues failed", zap.String("chain_id", chn.ChainID), zap.Error(err))
                continue
            }
            cs, _ := out.Load(chn.ChainID)
            cs.OpsQueue = opsStats
            cs.IndexerQueue = indexerStats
            // For backward compatibility, populate deprecated Queue field with IndexerQueue
            cs.Queue = indexerStats

            // Populate dual-queue fields from cache
            // These will be 0 until dual-queue architecture is deployed
            if cached, ok := c.App.QueueStatsCache.Load(chn.ChainID); ok {
                // Live queue depth = total pending tasks (workflows + activities)
                cs.LiveQueueDepth = cached.LiveQueue.PendingWorkflow + cached.LiveQueue.PendingActivity
                cs.LiveQueueBacklogAge = cached.LiveQueue.BacklogAgeSecs

                // Historical queue depth = total pending tasks (workflows + activities)
                cs.HistoricalQueueDepth = cached.HistoricalQueue.PendingWorkflow + cached.HistoricalQueue.PendingActivity
                cs.HistoricalQueueBacklogAge = cached.HistoricalQueue.BacklogAgeSecs
            }

            out.Store(chn.ChainID, cs)
        }
    }

    for _, chn := range chains {
        history, err := c.App.AdminDB.ListReindexRequests(ctx, chn.ChainID, 10)
        if err != nil {
            c.App.Logger.Warn("list reindex history failed", zap.String("chain_id", chn.ChainID), zap.Error(err))
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
        cs, _ := out.Load(chn.ChainID)
        cs.ReindexHistory = records
        out.Store(chn.ChainID, cs)
    }

    // 6) Calculate IsLiveSync for each chain after all data has been loaded
    // IsLiveSync indicates whether the chain is caught up with the latest blocks (within 2 blocks of head)
    out.Range(func(key string, cs admintypes.ChainStatus) bool {
        if cs.Head > 0 && cs.LastIndexed > 0 {
            // Chain is considered "live synced" if last indexed is within 2 blocks of head
            cs.IsLiveSync = (cs.Head - cs.LastIndexed <= 2)
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
    if err := c.startOpsWorkflow(r.Context(), id, indexerworkflow.HeadScanWorkflowName); err != nil {
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
    if err := c.startOpsWorkflow(r.Context(), id, indexerworkflow.GapScanWorkflowName); err != nil {
        c.App.Logger.Error("gapscan trigger failed", zap.String("chain_id", id), zap.String("user", user), zap.Error(err))
        w.WriteHeader(http.StatusInternalServerError)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
        return
    }
    c.App.Logger.Info("gapscan triggered", zap.String("chain_id", id), zap.String("user", user))
    _ = json.NewEncoder(w).Encode(map[string]string{"ok": "1"})
}

type reindexRequest struct {
    Heights []uint64 `json:"heights"`
    From    *uint64  `json:"from"`
    To      *uint64  `json:"to"`
}

func (c *Controller) HandleReindex(w http.ResponseWriter, r *http.Request) {
    id := mux.Vars(r)["id"]
    if id == "" {
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "missing chain id"})
        return
    }

    var req reindexRequest
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
    workflowInfos := make([]db.ReindexWorkflowInfo, 0, len(heights))

    for _, h := range heights {
        workflowID, runID, err := c.enqueueIndexBlock(r.Context(), id, h, true)
        if err != nil {
            c.App.Logger.Error("reindex enqueue failed", zap.String("chain_id", id), zap.Uint64("height", h), zap.String("user", user), zap.Error(err))
            w.WriteHeader(http.StatusInternalServerError)
            _ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
            return
        }
        workflowInfos = append(workflowInfos, db.ReindexWorkflowInfo{
            Height:     h,
            WorkflowID: workflowID,
            RunID:      runID,
        })
    }

    // Record all reindex requests with workflow info
    if err := c.App.AdminDB.RecordReindexRequestsWithWorkflow(r.Context(), id, user, workflowInfos); err != nil {
        c.App.Logger.Warn("failed to record reindex history", zap.String("chain_id", id), zap.Error(err))
    }

    c.App.Logger.Info("reindex queued", zap.String("chain_id", id), zap.String("user", user), zap.Int("count", len(heights)), zap.Any("heights", heights))
    _ = json.NewEncoder(w).Encode(map[string]any{"queued": len(heights)})
}

func (c *Controller) startOpsWorkflow(ctx context.Context, chainID, workflowName string) error {
    tc := c.App.TemporalClient
    if tc == nil {
        return fmt.Errorf("temporal client unavailable")
    }

    // Use the correct input type based on workflow name
    var input interface{}
    switch workflowName {
    case indexerworkflow.HeadScanWorkflowName:
        input = indexerworkflow.HeadScanInput{ChainID: chainID}
    case indexerworkflow.GapScanWorkflowName:
        input = indexerworkflow.GapScanInput{ChainID: chainID}
    default:
        // Fallback to old input type for unknown workflows
        input = indexertypes.ChainIdInput{ChainID: chainID}
    }

    options := client.StartWorkflowOptions{
        ID:        fmt.Sprintf("%s:%s:%d", chainID, strings.ToLower(workflowName), time.Now().UnixNano()),
        TaskQueue: tc.GetIndexerOpsQueue(chainID),
    }

    _, err := tc.TClient.ExecuteWorkflow(ctx, options, workflowName, input)
    return err
}

func (c *Controller) enqueueIndexBlock(ctx context.Context, chainID string, height uint64, reindex bool) (string, string, error) {
    tc := c.App.TemporalClient
    if tc == nil {
        return "", "", fmt.Errorf("temporal client unavailable")
    }

    input := indexertypes.IndexBlockInput{ChainID: chainID, Height: height, Reindex: reindex, PriorityKey: 1}
    options := client.StartWorkflowOptions{
        ID:        c.App.TemporalClient.GetIndexBlockWorkflowIdWithTime(chainID, height),
        TaskQueue: tc.GetIndexerQueue(chainID),
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
                zap.Int64("indexer_pending_workflows", cached.IndexerQueue.PendingWorkflow),
            )
            return cached.OpsQueue, cached.IndexerQueue, nil
        }
    }

    reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
    defer cancel()

    // Query both queues in parallel for better performance using the enhanced API
    type queueResult struct {
        stats admintypes.QueueStatus
        err   error
    }

    opsChannel := make(chan queueResult, 1)
    indexerChannel := make(chan queueResult, 1)

    // Query ops queue using the new common function
    go func() {
        queueName := temporalClient.GetIndexerOpsQueue(chainID)
        c.App.Logger.Debug("querying ops queue", zap.String("chain_id", chainID), zap.String("queue_name", queueName))

        // Use the common GetQueueStats function with enhanced API
        pendingWorkflows, pendingActivities, pollerCount, backlogAge, err := temporalClient.GetQueueStats(reqCtx, queueName)
        if err != nil {
            opsChannel <- queueResult{stats: opsStats, err: err}
            return
        }

        stats := admintypes.QueueStatus{
            PendingWorkflow: pendingWorkflows,
            PendingActivity: pendingActivities,
            Pollers:         pollerCount,
            BacklogAgeSecs:  backlogAge,
        }
        opsChannel <- queueResult{stats: stats, err: nil}
    }()

    // Query indexer queue using the new common function
    go func() {
        queueName := temporalClient.GetIndexerQueue(chainID)
        c.App.Logger.Debug("querying indexer queue", zap.String("chain_id", chainID), zap.String("queue_name", queueName))

        // Use the common GetQueueStats function with enhanced API
        pendingWorkflows, pendingActivities, pollerCount, backlogAge, err := temporalClient.GetQueueStats(reqCtx, queueName)
        if err != nil {
            indexerChannel <- queueResult{stats: indexerStats, err: err}
            return
        }

        stats := admintypes.QueueStatus{
            PendingWorkflow: pendingWorkflows,
            PendingActivity: pendingActivities,
            Pollers:         pollerCount,
            BacklogAgeSecs:  backlogAge,
        }
        indexerChannel <- queueResult{stats: stats, err: nil}
    }()

    // Collect results
    opsResult := <-opsChannel
    indexerResult := <-indexerChannel

    // If either query failed, log detailed error and return
    if opsResult.err != nil {
        c.App.Logger.Error("failed to describe ops queue",
            zap.String("chain_id", chainID),
            zap.String("queue_name", temporalClient.GetIndexerOpsQueue(chainID)),
            zap.Error(opsResult.err),
        )
        return opsStats, indexerStats, fmt.Errorf("failed to describe ops queue: %w", opsResult.err)
    }
    if indexerResult.err != nil {
        c.App.Logger.Error("failed to describe indexer queue",
            zap.String("chain_id", chainID),
            zap.String("queue_name", temporalClient.GetIndexerQueue(chainID)),
            zap.Error(indexerResult.err),
        )
        return opsStats, indexerStats, fmt.Errorf("failed to describe indexer queue: %w", indexerResult.err)
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

    // Store all queues in cache (including live and historical)
    c.App.QueueStatsCache.Store(chainID, admintypes.CachedQueueStats{
        OpsQueue:        opsStats,
        IndexerQueue:    indexerStats,
        LiveQueue:       admintypes.QueueStatus{}, // Will be populated when dual-queue is implemented
        HistoricalQueue: admintypes.QueueStatus{}, // Will be populated when dual-queue is implemented
        Fetched:         time.Now(),
    })

    return opsStats, indexerStats, nil
}

// HandleChainPatch updates a chain by ID
func (c *Controller) HandleChainPatch(w http.ResponseWriter, r *http.Request) {
    id := mux.Vars(r)["id"]

    // 1) Load the current row (so we can do a partial update)
    cur, err := c.App.AdminDB.GetChain(r.Context(), id)
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

// PATCH /admin/chains/status
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
        if x.ChainID == "" {
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
    chain, err := c.App.AdminDB.GetChain(ctx, id)
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
        headScheduleID := c.App.TemporalClient.GetHeadScheduleID(id)
        headHandle := c.App.TemporalClient.TSClient.GetHandle(ctx, headScheduleID)
        if err := headHandle.Delete(ctx); err != nil {
            var notFound *serviceerror.NotFound
            if !errors.As(err, &notFound) {
                c.App.Logger.Warn("failed to delete head scan schedule", zap.String("chain_id", id), zap.Error(err))
            }
        }

        // Delete gap scan schedule
        gapScheduleID := c.App.TemporalClient.GetGapScanScheduleID(id)
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

package controller

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "net/http"
    "sort"
    "strconv"
    "strings"
    "time"

    "github.com/canopy-network/canopyx/pkg/temporal/indexer"

    "github.com/canopy-network/canopyx/app/admin/controller/types"
    indexertypes "github.com/canopy-network/canopyx/app/indexer/types"

    "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
    "github.com/alitto/pond/v2"
    admintypes "github.com/canopy-network/canopyx/app/admin/types"
    "github.com/canopy-network/canopyx/pkg/db/models/admin"
    "github.com/canopy-network/canopyx/pkg/rpc"
    "github.com/canopy-network/canopyx/pkg/utils"
    "github.com/go-jose/go-jose/v4/json"
    "github.com/gorilla/mux"
    "github.com/puzpuzpuz/xsync/v4"
    "go.temporal.io/sdk/client"
    sdktemporal "go.temporal.io/sdk/temporal"
    "go.uber.org/zap"
)

// HandleChainsList returns all registered chains
// Query param: ?deleted=true to include soft-deleted chains
func (c *Controller) HandleChainsList(w http.ResponseWriter, r *http.Request) {
    // Parse query param to determine if we should include deleted chains
    includeDeleted := r.URL.Query().Get("deleted") == "true"

    cs, err := c.App.AdminDB.ListChain(r.Context(), includeDeleted)
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

    // Default chain_name if not provided
    chain.ChainName = strings.TrimSpace(chain.ChainName)
    if chain.ChainName == "" {
        chain.ChainName = fmt.Sprintf("Chain %d", chain.ChainID)
    }

    chain.Image = strings.TrimSpace(chain.Image)
    chain.Notes = strings.TrimSpace(chain.Notes)

    // Default image if not provided
    if chain.Image == "" {
        chain.Image = "canopynetwork/canopyx-indexer:latest"
    }

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

    // Default reindex settings if not provided
    if chain.ReindexMinReplicas == 0 {
        chain.ReindexMinReplicas = 1
    }
    if chain.ReindexMaxReplicas == 0 {
        chain.ReindexMaxReplicas = 3
    }
    if chain.ReindexScaleThreshold == 0 {
        chain.ReindexScaleThreshold = 100
    }
    if chain.ReindexMaxReplicas < chain.ReindexMinReplicas {
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "reindex_max_replicas must be greater than or equal to reindex_min_replicas"})
        return
    }

    ctx := r.Context()

    if err := c.App.AdminDB.UpsertChain(ctx, &chain); err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
        return
    }

    // With global single-DB architecture, we only need to ensure Temporal schedules exist.
    // The indexer will write to the global database with chain_id context.
    chainIDStr := strconv.FormatUint(chain.ChainID, 10)
    err := c.App.EnsureChainSchedules(r.Context(), chainIDStr)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
        return
    }

    // Return the full chain object including auto-detected chain_id
    // This is especially important for Tilt auto-registration to get the actual chain_id
    _ = json.NewEncoder(w).Encode(chain)
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
        w.WriteHeader(http.StatusNotFound)
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
    // Use fixed 60-second buckets for consistent granularity regardless of time window.
    // This ensures small data sets show full detail and larger windows still render efficiently.
    // Max points: 1h=60, 6h=360, 24h=1440, 3d=4320, 7d=10080 (all reasonable for charting)
    intervalSeconds := 60

    ctx := r.Context()

    points, pointsErr := c.App.AdminDB.IndexProgressHistory(ctx, chainID, hours, intervalSeconds)
    if pointsErr != nil {
        w.WriteHeader(http.StatusInternalServerError)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "query failed"})
        return
    }

    result := make([]types.ResponsePoint, 0, len(points))
    intervalMinutes := float64(intervalSeconds) / 60.0
    for i, p := range points {
        velocity := 0.0
        if i > 0 {
            timeDiff := p.TimeBucket.Sub(points[i-1].TimeBucket).Minutes()
            // Calculate velocity as blocks indexed per minute (indexer throughput)
            // NOT height difference (which measures chain growth rate)
            if timeDiff > 0 && p.BlocksIndexed > 0 {
                velocity = float64(p.BlocksIndexed) / timeDiff
            }
        } else if p.BlocksIndexed > 0 {
            // For the first data point (or single bucket), use the interval size
            velocity = float64(p.BlocksIndexed) / intervalMinutes
        }

        result = append(result, types.ResponsePoint{
            Time:           p.TimeBucket.UTC().Format(time.RFC3339),
            Height:         p.MaxHeight,
            AvgTotalTimeMs: p.AvgTotalTimeMs,
            BlocksIndexed:  p.BlocksIndexed,
            Velocity:       velocity,
            // Individual timing averages
            AvgFetchBlockMs:        p.AvgFetchBlockMs,
            AvgPrepareIndexMs:      p.AvgPrepareIndexMs,
            AvgIndexAccountsMs:     p.AvgIndexAccountsMs,
            AvgIndexCommitteesMs:   p.AvgIndexCommitteesMs,
            AvgIndexDexBatchMs:     p.AvgIndexDexBatchMs,
            AvgIndexDexPricesMs:    p.AvgIndexDexPricesMs,
            AvgIndexEventsMs:       p.AvgIndexEventsMs,
            AvgIndexOrdersMs:       p.AvgIndexOrdersMs,
            AvgIndexParamsMs:       p.AvgIndexParamsMs,
            AvgIndexPoolsMs:        p.AvgIndexPoolsMs,
            AvgIndexSupplyMs:       p.AvgIndexSupplyMs,
            AvgIndexTransactionsMs: p.AvgIndexTransactionsMs,
            AvgIndexValidatorsMs:   p.AvgIndexValidatorsMs,
            AvgSaveBlockMs:         p.AvgSaveBlockMs,
            AvgSaveBlockSummaryMs:  p.AvgSaveBlockSummaryMs,
        })
    }

    _ = json.NewEncoder(w).Encode(result)
}

// HandleChainStatus returns the last indexed height for all known chains.
func (c *Controller) HandleChainStatus(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // 1) Get chains (including deleted ones - UI will filter them)
    chains, err := c.App.AdminDB.ListChain(ctx, true)
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
        dbName := c.App.AdminDB.DatabaseName()
        q := fmt.Sprintf(`
			SELECT chain_id, maxMerge(max_height) AS last_idx
			FROM "%s"."index_progress_agg"
			GROUP BY chain_id
		`, dbName)

        rows, err := c.App.AdminDB.GetConnection().Query(ctx, q)
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
                // Use Compute for atomic update to prevent race condition
                out.Compute(id, func(oldValue admintypes.ChainStatus, loaded bool) (admintypes.ChainStatus, xsync.ComputeOp) {
                    if !loaded {
                        oldValue = admintypes.ChainStatus{ChainID: chainID}
                    }
                    oldValue.LastIndexed = last
                    return oldValue, xsync.UpdateOp
                })
            }
            _ = rows.Err()
        }
    }

    // 3) Fallback for any chain missing in the aggregate: max(height) from base table
    {
        dbName := c.App.AdminDB.DatabaseName()
        q := fmt.Sprintf(`
			SELECT chain_id, max(height) AS last_idx
			FROM "%s"."index_progress"
			GROUP BY chain_id
		`, dbName)

        rows, err := c.App.AdminDB.GetConnection().Query(ctx, q)
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
                // Use Compute for atomic update to prevent race condition
                out.Compute(id, func(oldValue admintypes.ChainStatus, loaded bool) (admintypes.ChainStatus, xsync.ComputeOp) {
                    if !loaded {
                        oldValue = admintypes.ChainStatus{ChainID: chainID}
                    }
                    // Only update if not already set
                    if oldValue.LastIndexed == 0 {
                        oldValue.LastIndexed = last
                    }
                    return oldValue, xsync.UpdateOp
                })
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
            // Use Compute for atomic update to prevent race condition
            out.Compute(chainIDStr, func(oldValue admintypes.ChainStatus, loaded bool) (admintypes.ChainStatus, xsync.ComputeOp) {
                if !loaded {
                    oldValue = admintypes.ChainStatus{ChainID: chn.ChainID}
                }
                oldValue.Head = head
                return oldValue, xsync.UpdateOp
            })
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

            // Update chain status with gap information (atomic to prevent race condition)
            out.Compute(chainIDStr, func(oldValue admintypes.ChainStatus, loaded bool) (admintypes.ChainStatus, xsync.ComputeOp) {
                if !loaded {
                    oldValue = admintypes.ChainStatus{ChainID: chn.ChainID}
                }
                oldValue.MissingBlocksCount = missingCount
                oldValue.GapRangesCount = len(gaps)
                oldValue.LargestGapStart = largestGapStart
                oldValue.LargestGapEnd = largestGapEnd
                return oldValue, xsync.UpdateOp
            })
        })
    }

    // Wait for all head and gap fetching tasks to complete
    if err := group.Wait(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, pond.ErrGroupStopped) {
        c.App.Logger.Warn("some chain status tasks failed", zap.Error(err))
    }

    // 5) Fetch queue metrics for both ops and indexer queues
    if c.App.TemporalManager != nil {
        for _, chn := range chains {
            // Skip deleted chains - their namespace has been deleted
            if chn.Deleted != 0 {
                continue
            }
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

    // 7) Fetch per-endpoint health data for each chain
    for _, chn := range chains {
        chainIDStr := strconv.FormatUint(chn.ChainID, 10)
        endpoints, err := c.App.AdminDB.GetEndpointsForChain(ctx, chn.ChainID)
        if err != nil {
            c.App.Logger.Warn("failed to fetch endpoint health",
                zap.String("chain_id", chainIDStr),
                zap.Error(err))
            continue
        }

        // Convert admin.RPCEndpoint to admintypes.EndpointHealth
        endpointHealth := make([]admintypes.EndpointHealth, 0, len(endpoints))
        for _, ep := range endpoints {
            endpointHealth = append(endpointHealth, admintypes.EndpointHealth{
                Endpoint:  ep.Endpoint,
                Status:    ep.Status,
                Height:    ep.Height,
                LatencyMs: ep.LatencyMs,
                Error:     ep.Error,
                UpdatedAt: ep.UpdatedAt,
            })
        }

        cs, _ := out.Load(chainIDStr)
        cs.Endpoints = endpointHealth
        out.Store(chainIDStr, cs)
    }

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

// validateReindexBlocks checks if blocks exist in the database (already indexed).
// Returns only blocks that have been previously indexed.
func (c *Controller) validateReindexBlocks(ctx context.Context, chainID uint64, heights []uint64) (alreadyIndexed []uint64, notIndexed []uint64, err error) {
    // Get GlobalDB configured for this chain
    store := c.App.GetGlobalDBForChain(chainID)

    for _, height := range heights {
        // Check if block exists in database
        exists, err := store.HasBlock(ctx, height)
        if err != nil {
            c.App.Logger.Warn("failed to check block existence",
                zap.Uint64("chain_id", chainID),
                zap.Uint64("height", height),
                zap.Error(err))
            continue
        }

        if exists {
            alreadyIndexed = append(alreadyIndexed, height)
        } else {
            notIndexed = append(notIndexed, height)
        }
    }

    return alreadyIndexed, notIndexed, nil
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
        // No block limit - allow unlimited ranges for full history reindex
        for h := from; h <= to; h++ {
            addHeight(h)
        }
    }

    if len(heights) == 0 {
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "no heights specified"})
        return
    }

    // No block limit - allow unlimited ranges for full history reindex

    // Parse chain ID for validation
    chainIDUint, parseErr := strconv.ParseUint(id, 10, 64)
    if parseErr != nil {
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chain ID"})
        return
    }

    // Validate that blocks exist in database (already indexed)
    alreadyIndexed, notIndexed, err := c.validateReindexBlocks(r.Context(), chainIDUint, heights)
    if err != nil {
        c.App.Logger.Error("failed to validate reindex blocks",
            zap.Uint64("chain_id", chainIDUint),
            zap.Error(err))
        w.WriteHeader(http.StatusInternalServerError)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "validation failed"})
        return
    }

    if len(notIndexed) > 0 {
        c.App.Logger.Warn("some blocks not yet indexed",
            zap.Uint64("chain_id", chainIDUint),
            zap.Any("not_indexed", notIndexed))
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]any{
            "error":              "cannot reindex blocks that haven't been indexed yet",
            "not_indexed_blocks": notIndexed,
        })
        return
    }

    // Use only validated blocks that are already indexed
    heights = alreadyIndexed

    // Sort heights in descending order (newest first) as per requirement
    sort.Slice(heights, func(i, j int) bool {
        return heights[i] > heights[j]
    })

    user := c.currentUser(r)

    // Trigger ReindexSchedulerWorkflow for the entire range
    minHeight := heights[len(heights)-1] // Last element (sorted descending)
    maxHeight := heights[0]              // First element (sorted descending)

    // Generate unique request ID for this reindex operation
    // This ensures deterministic workflow IDs and prevents duplicate workflows
    requestID := fmt.Sprintf("%d-%d-%d", chainIDUint, minHeight, time.Now().Unix())

    input := indexertypes.WorkflowReindexSchedulerInput{
        ChainID:     chainIDUint,
        StartHeight: minHeight,
        EndHeight:   maxHeight,
        RequestedBy: user,
        RequestID:   requestID, // Unique ID for deterministic workflow IDs
    }

    workflowID := fmt.Sprintf("reindex-scheduler-chain-%d-range-%d-%d-%d",
        chainIDUint, minHeight, maxHeight, time.Now().Unix())

    if c.App.TemporalManager == nil {
        c.App.Logger.Error("temporal manager unavailable",
            zap.Uint64("chain_id", chainIDUint))
        w.WriteHeader(http.StatusServiceUnavailable)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "temporal manager unavailable"})
        return
    }

    // Fetch namespace from database
    namespace, err := c.getChainNamespace(r.Context(), chainIDUint)
    if err != nil {
        c.App.Logger.Error("failed to get chain namespace",
            zap.Uint64("chain_id", chainIDUint),
            zap.Error(err))
        w.WriteHeader(http.StatusInternalServerError)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to get chain namespace"})
        return
    }

    // Get chain-specific client
    chainClient, err := c.App.TemporalManager.GetChainClient(r.Context(), chainIDUint, namespace)
    if err != nil {
        c.App.Logger.Error("failed to get chain client",
            zap.Uint64("chain_id", chainIDUint),
            zap.Error(err))
        w.WriteHeader(http.StatusServiceUnavailable)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to get chain client"})
        return
    }

    options := client.StartWorkflowOptions{
        ID:        workflowID,
        TaskQueue: chainClient.OpsQueue, // "ops" (simplified, runs in chain namespace)
        RetryPolicy: &sdktemporal.RetryPolicy{
            InitialInterval:    time.Second,
            BackoffCoefficient: 1.2,
            MaximumInterval:    5 * time.Second,
            MaximumAttempts:    0, // Unlimited retries
        },
    }

    run, err := chainClient.TClient.ExecuteWorkflow(r.Context(), options, "ReindexSchedulerWorkflow", input)
    if err != nil {
        c.App.Logger.Error("failed to start reindex scheduler",
            zap.Uint64("chain_id", chainIDUint),
            zap.Uint64("start", minHeight),
            zap.Uint64("end", maxHeight),
            zap.Error(err))
        w.WriteHeader(http.StatusInternalServerError)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to schedule reindex"})
        return
    }

    // Record the reindex request
    if err := c.App.AdminDB.RecordReindexRequests(r.Context(), chainIDUint, user, heights); err != nil {
        c.App.Logger.Warn("failed to record reindex history",
            zap.Uint64("chain_id", chainIDUint),
            zap.Error(err))
    }

    c.App.Logger.Info("reindex scheduler started",
        zap.String("chain_id", id),
        zap.String("user", user),
        zap.Int("count", len(heights)),
        zap.Uint64("start", minHeight),
        zap.Uint64("end", maxHeight),
        zap.String("workflow_id", run.GetID()),
        zap.String("run_id", run.GetRunID()))

    _ = json.NewEncoder(w).Encode(map[string]any{
        "queued":      len(heights),
        "workflow_id": run.GetID(),
        "run_id":      run.GetRunID(),
    })
}

// startOpsWorkflow starts a Temporal workflow to perform operations related to a specific chain and workflow name.
// It dynamically determines the input structure based on the workflow name and uses a Temporal client to execute it.
// Returns an error if the manager is unavailable, chainID is invalid, or workflow execution fails.
func (c *Controller) startOpsWorkflow(ctx context.Context, chainID, workflowName string) error {
    if c.App.TemporalManager == nil {
        return fmt.Errorf("temporal manager unavailable")
    }

    // Convert string chainID to uint64
    chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
    if parseErr != nil {
        return fmt.Errorf("invalid chain ID format: %w", parseErr)
    }

    // Fetch namespace from database
    namespace, err := c.getChainNamespace(ctx, chainIDUint)
    if err != nil {
        return fmt.Errorf("failed to get chain namespace: %w", err)
    }

    // Get chain-specific client
    chainClient, err := c.App.TemporalManager.GetChainClient(ctx, chainIDUint, namespace)
    if err != nil {
        return fmt.Errorf("failed to get chain client: %w", err)
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
        TaskQueue: chainClient.OpsQueue, // "ops" (simplified, runs in chain namespace)
    }

    _, err = chainClient.TClient.ExecuteWorkflow(ctx, options, workflowName, input)
    return err
}

// describeBothQueues fetches metrics for both the ops queue and indexer queue for a given chain.
// It uses caching with a 30s TTL to reduce load on Temporal API.
// Returns (opsQueue, indexerQueue, error).
func (c *Controller) describeBothQueues(ctx context.Context, chainID string) (admintypes.QueueStatus, admintypes.QueueStatus, error) {
    opsStats := admintypes.QueueStatus{}
    indexerStats := admintypes.QueueStatus{}

    if c.App.TemporalManager == nil {
        c.App.Logger.Error("describeBothQueues: temporal manager not initialized", zap.String("chain_id", chainID))
        return opsStats, indexerStats, fmt.Errorf("temporal manager not initialized")
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

    // Convert string chainID to uint64 for getting chain client
    chainIDUint, parseErr := strconv.ParseUint(chainID, 10, 64)
    if parseErr != nil {
        return admintypes.QueueStatus{}, admintypes.QueueStatus{}, fmt.Errorf("invalid chain ID format: %w", parseErr)
    }

    // Fetch namespace for this chain
    namespace, err := c.getChainNamespace(ctx, chainIDUint)
    if err != nil {
        c.App.Logger.Error("describeBothQueues: failed to get chain namespace",
            zap.String("chain_id", chainID),
            zap.Error(err))
        return opsStats, indexerStats, fmt.Errorf("failed to get chain namespace: %w", err)
    }

    // Get chain-specific client for queue stats
    chainClient, err := c.App.TemporalManager.GetChainClient(ctx, chainIDUint, namespace)
    if err != nil {
        c.App.Logger.Error("describeBothQueues: failed to get chain client",
            zap.String("chain_id", chainID),
            zap.Error(err))
        return opsStats, indexerStats, fmt.Errorf("failed to get chain client: %w", err)
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

    // Query ops queue using the simplified queue name
    go func() {
        queueName := chainClient.OpsQueue // "ops" (simplified)
        c.App.Logger.Debug("querying ops queue", zap.String("chain_id", chainID), zap.String("queue_name", queueName))

        // Use the chain client's GetQueueStats function with enhanced API
        pendingWorkflows, pendingActivities, pollerCount, backlogAge, err := chainClient.GetQueueStats(reqCtx, queueName)
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
        // Get stats from both live and historical queues using simplified names
        liveQueue := chainClient.LiveQueue             // "live"
        historicalQueue := chainClient.HistoricalQueue // "historical"

        c.App.Logger.Debug("querying both indexer queues",
            zap.String("chain_id", chainID),
            zap.String("live_queue", liveQueue),
            zap.String("historical_queue", historicalQueue))

        // Query live queue
        livePendingWF, livePendingAct, livePollers, liveBacklogAge, liveErr := chainClient.GetQueueStats(reqCtx, liveQueue)
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
        histPendingWF, histPendingAct, histPollers, histBacklogAge, histErr := chainClient.GetQueueStats(reqCtx, historicalQueue)
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
            zap.String("queue_name", chainClient.OpsQueue),
            zap.Error(opsResult.err),
        )
        return opsStats, indexerStats, fmt.Errorf("failed to describe ops queue: %w", opsResult.err)
    }
    if indexerResult.err != nil {
        c.App.Logger.Error("failed to describe indexer queues",
            zap.String("chain_id", chainID),
            zap.String("live_queue", chainClient.LiveQueue),
            zap.String("historical_queue", chainClient.HistoricalQueue),
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

    // 3) Apply changes (all user-updatable fields except paused/deleted/chain_id)

    // Chain name
    if in.ChainName != "" {
        cur.ChainName = strings.TrimSpace(in.ChainName)
    }

    // Image
    if in.Image != "" {
        cur.Image = strings.TrimSpace(in.Image)
    }

    // Notes
    if in.Notes != "" {
        cur.Notes = strings.TrimSpace(in.Notes)
    }

    // RPC Endpoints
    if in.RPCEndpoints != nil {
        cleaned := make([]string, 0, len(in.RPCEndpoints))
        for _, e := range in.RPCEndpoints {
            e = strings.TrimSpace(e)
            if e != "" {
                cleaned = append(cleaned, e)
            }
        }
        cleaned = utils.Dedup(cleaned)

        // Validate endpoints match chain ID
        if len(cleaned) > 0 {
            detectedChainID, err := rpc.ValidateAndExtractChainID(r.Context(), cleaned, c.App.Logger)
            if err != nil {
                w.WriteHeader(http.StatusBadRequest)
                _ = json.NewEncoder(w).Encode(map[string]string{
                    "error": fmt.Sprintf("failed to validate RPC endpoints: %v", err),
                })
                return
            }
            if detectedChainID != chainIDUint {
                w.WriteHeader(http.StatusBadRequest)
                _ = json.NewEncoder(w).Encode(map[string]string{
                    "error": fmt.Sprintf("chain ID mismatch: chain is %d but RPC endpoints report %d", chainIDUint, detectedChainID),
                })
                return
            }
        }
        cur.RPCEndpoints = cleaned
    }

    // Replicas configuration
    if in.MinReplicas > 0 {
        cur.MinReplicas = in.MinReplicas
    }
    if in.MaxReplicas > 0 {
        cur.MaxReplicas = in.MaxReplicas
    }
    if cur.MaxReplicas < cur.MinReplicas {
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "max_replicas must be greater than or equal to min_replicas"})
        return
    }

    // Reindex replicas configuration
    if in.ReindexMinReplicas > 0 {
        cur.ReindexMinReplicas = in.ReindexMinReplicas
    }
    if in.ReindexMaxReplicas > 0 {
        cur.ReindexMaxReplicas = in.ReindexMaxReplicas
    }
    if cur.ReindexMaxReplicas < cur.ReindexMinReplicas {
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "reindex_max_replicas must be greater than or equal to reindex_min_replicas"})
        return
    }

    // Reindex scale threshold
    if in.ReindexScaleThreshold > 0 {
        cur.ReindexScaleThreshold = in.ReindexScaleThreshold
    }

    // 4) Persist (ReplacingMergeTree upsert)
    if upsertErr := c.App.AdminDB.UpsertChain(r.Context(), cur); upsertErr != nil {
        w.WriteHeader(http.StatusInternalServerError)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": upsertErr.Error()})
        return
    }

    _ = json.NewEncoder(w).Encode(map[string]string{"ok": "1"})
}

// HandleChainsBulkUpdate updates all non-deleted chains with common fields.
// PATCH /api/chains/bulk
func (c *Controller) HandleChainsBulkUpdate(w http.ResponseWriter, r *http.Request) {
    var req types.BulkUpdateChainsRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid json"})
        return
    }

    // Validate that at least one field is being updated
    if req.Image == "" && req.ReindexMinReplicas == 0 && req.ReindexMaxReplicas == 0 &&
            req.ReindexScaleThreshold == 0 && req.MinReplicas == 0 && req.MaxReplicas == 0 {
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "no fields to update"})
        return
    }

    // Validate replica constraints if both are provided
    if req.MinReplicas > 0 && req.MaxReplicas > 0 && req.MaxReplicas < req.MinReplicas {
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "max_replicas must be >= min_replicas"})
        return
    }
    if req.ReindexMinReplicas > 0 && req.ReindexMaxReplicas > 0 && req.ReindexMaxReplicas < req.ReindexMinReplicas {
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "reindex_max_replicas must be >= reindex_min_replicas"})
        return
    }

    ctx := r.Context()
    user := c.currentUser(r)

    // Get all non-deleted chains
    chains, err := c.App.AdminDB.ListChain(ctx, false)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
        return
    }

    updatedChainIDs := make([]uint64, 0, len(chains))

    // Update each chain
    for _, chain := range chains {
        updated := false

        // Apply image if provided
        if req.Image != "" {
            chain.Image = strings.TrimSpace(req.Image)
            updated = true
        }

        // Apply replicas if provided
        if req.MinReplicas > 0 {
            chain.MinReplicas = req.MinReplicas
            updated = true
        }
        if req.MaxReplicas > 0 {
            chain.MaxReplicas = req.MaxReplicas
            updated = true
        }

        // Apply reindex settings if provided
        if req.ReindexMinReplicas > 0 {
            chain.ReindexMinReplicas = req.ReindexMinReplicas
            updated = true
        }
        if req.ReindexMaxReplicas > 0 {
            chain.ReindexMaxReplicas = req.ReindexMaxReplicas
            updated = true
        }
        if req.ReindexScaleThreshold > 0 {
            chain.ReindexScaleThreshold = req.ReindexScaleThreshold
            updated = true
        }

        // Validate replica constraints after updates
        if chain.MaxReplicas < chain.MinReplicas {
            chain.MaxReplicas = chain.MinReplicas
        }
        if chain.ReindexMaxReplicas < chain.ReindexMinReplicas {
            chain.ReindexMaxReplicas = chain.ReindexMinReplicas
        }

        if updated {
            if err := c.App.AdminDB.UpsertChain(ctx, &chain); err != nil {
                c.App.Logger.Error("failed to update chain in bulk update",
                    zap.Uint64("chain_id", chain.ChainID),
                    zap.Error(err))
                continue
            }
            updatedChainIDs = append(updatedChainIDs, chain.ChainID)
        }
    }

    c.App.Logger.Info("bulk chain update completed",
        zap.String("user", user),
        zap.Int("updated_count", len(updatedChainIDs)),
        zap.Any("chain_ids", updatedChainIDs))

    _ = json.NewEncoder(w).Encode(types.BulkUpdateChainsResponse{
        UpdatedCount: len(updatedChainIDs),
        ChainIDs:     updatedChainIDs,
    })
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

// HandleChainDelete permanently deletes a chain and all its data.
// This deletes the Temporal namespace, starts an async workflow to clean all data from the global DB,
// and removes all admin table records. This operation cannot be undone.
func (c *Controller) HandleChainDelete(w http.ResponseWriter, r *http.Request) {
    id := mux.Vars(r)["id"]
    if id == "" {
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "missing chain id"})
        return
    }

    ctx := r.Context()

    // Verify chain exists
    chainIDUint, parseErr := strconv.ParseUint(id, 10, 64)
    if parseErr != nil {
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chain ID"})
        return
    }
    if _, err := c.App.AdminDB.GetChain(ctx, chainIDUint); err != nil {
        w.WriteHeader(http.StatusNotFound)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "chain not found"})
        return
    }

    user := c.currentUser(r)
    c.App.Logger.Info("deleting chain", zap.String("chain_id", id), zap.String("user", user))

    // 1. Delete Temporal namespace (removes all workflows and schedules)
    // Returns the renamed namespace (e.g., "chain_5-deleted-abc123") for cleanup monitoring
    var renamedNamespace string
    if c.App.TemporalManager != nil {
        var nsErr error
        renamedNamespace, nsErr = c.App.DeleteChainNamespace(ctx, id)
        if nsErr != nil {
            c.App.Logger.Warn("failed to delete chain namespace", zap.String("chain_id", id), zap.Error(nsErr))
        }
    }

    // 2. Clear in-memory caches
    c.App.QueueStatsCache.Delete(id)

    // 3. Start async workflow to clean all data (global DB chain_id cleanup, admin tables, namespace cleanup)
    workflowID, err := c.App.StartDeleteChainWorkflow(ctx, chainIDUint, renamedNamespace)
    if err != nil {
        c.App.Logger.Error("failed to start delete chain workflow", zap.String("chain_id", id), zap.Error(err))
        w.WriteHeader(http.StatusInternalServerError)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to start delete workflow"})
        return
    }

    c.App.Logger.Info("chain delete workflow started",
        zap.String("chain_id", id),
        zap.String("workflow_id", workflowID),
        zap.String("renamed_namespace", renamedNamespace),
        zap.String("user", user))

    _ = json.NewEncoder(w).Encode(map[string]string{
        "status":      "deletion_started",
        "workflow_id": workflowID,
    })
}

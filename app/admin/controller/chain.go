package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	admintypes "github.com/canopy-network/canopyx/app/admin/types"
	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	indexertypes "github.com/canopy-network/canopyx/pkg/indexer/types"
	indexerworkflow "github.com/canopy-network/canopyx/pkg/indexer/workflow"
	"github.com/canopy-network/canopyx/pkg/rpc"
	"github.com/canopy-network/canopyx/pkg/utils"
	"github.com/go-jose/go-jose/v4/json"
	"github.com/gorilla/mux"
	"github.com/uptrace/go-clickhouse/ch"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	workflowservicepb "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	sdktemporal "go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

const queueDescribeTimeout = 2 * time.Second

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

	out := make(map[string]admintypes.ChainStatus, len(chains))
	for _, chn := range chains {
		out[chn.ChainID] = admintypes.ChainStatus{
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
		}
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
				cs, ok := out[id]
				if !ok {
					cs = admintypes.ChainStatus{ChainID: id}
				}
				cs.LastIndexed = last
				out[id] = cs
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
				cs, ok := out[id]
				if !ok {
					cs = admintypes.ChainStatus{ChainID: id}
				}
				if cs.LastIndexed == 0 {
					cs.LastIndexed = last
					out[id] = cs
				}
			}
			_ = rows.Err()
		}
	}

	// 4) Heads via RPC (best-effort, bounded concurrency)
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, chn := range chains {
		chn := chn
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()

			cli := rpc.NewHTTPWithOpts(rpc.Opts{Endpoints: chn.RPCEndpoints})
			head, err := cli.ChainHead(ctx)
			if err != nil {
				return
			}
			cs, ok := out[chn.ChainID]
			if !ok {
				cs = admintypes.ChainStatus{ChainID: chn.ChainID}
			}
			cs.Head = head
			out[chn.ChainID] = cs
		}()
	}
	wg.Wait()

	// 5) Fetch queue metrics for both ops and indexer queues
	if c.App.TemporalClient != nil {
		for _, chn := range chains {
			opsStats, indexerStats, err := c.describeBothQueues(ctx, chn.ChainID)
			if err != nil {
				c.App.Logger.Warn("describe queues failed", zap.String("chain_id", chn.ChainID), zap.Error(err))
				continue
			}
			cs := out[chn.ChainID]
			cs.OpsQueue = opsStats
			cs.IndexerQueue = indexerStats
			// For backward compatibility, populate deprecated Queue field with IndexerQueue
			cs.Queue = indexerStats
			out[chn.ChainID] = cs
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
			})
		}
		cs := out[chn.ChainID]
		cs.ReindexHistory = records
		out[chn.ChainID] = cs
	}

	_ = json.NewEncoder(w).Encode(out)
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
	if err := c.App.AdminDB.RecordReindexRequests(r.Context(), id, user, heights); err != nil {
		c.App.Logger.Warn("failed to record reindex history", zap.String("chain_id", id), zap.Error(err))
	}
	for _, h := range heights {
		if err := c.enqueueIndexBlock(r.Context(), id, h, true); err != nil {
			c.App.Logger.Error("reindex enqueue failed", zap.String("chain_id", id), zap.Uint64("height", h), zap.String("user", user), zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
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

func (c *Controller) enqueueIndexBlock(ctx context.Context, chainID string, height uint64, reindex bool) error {
	tc := c.App.TemporalClient
	if tc == nil {
		return fmt.Errorf("temporal client unavailable")
	}

	input := indexertypes.IndexBlockInput{ChainID: chainID, Height: height, Reindex: reindex, PriorityKey: 1}
	options := client.StartWorkflowOptions{
		ID:        fmt.Sprintf("%s:index:%d:%d", chainID, height, time.Now().UnixNano()),
		TaskQueue: tc.GetIndexerQueue(chainID),
		RetryPolicy: &sdktemporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 1.2,
			MaximumInterval:    5 * time.Second,
			MaximumAttempts:    0,
		},
	}

	_, err := tc.TClient.ExecuteWorkflow(ctx, options, "IndexBlockWorkflow", input)
	return err
}

// describeQueue is deprecated in favor of describeBothQueues.
// It queries only the indexer queue for backward compatibility.
// Prefer using describeBothQueues to get metrics for both queues.
func (c *Controller) describeQueue(ctx context.Context, chainID string) (admintypes.QueueStatus, error) {
	stats := admintypes.QueueStatus{}
	client := c.App.TemporalClient
	if client == nil {
		return stats, fmt.Errorf("temporal client not initialized")
	}

	// Check cache first (30s TTL to reduce Temporal API rate limiting)
	if cached, ok := c.App.QueueStatsCache.Load(chainID); ok {
		if time.Since(cached.Fetched) < 30*time.Second {
			return cached.IndexerQueue, nil
		}
	}

	svc := client.TClient.WorkflowService()
	if svc == nil {
		return stats, fmt.Errorf("temporal workflow service unavailable")
	}

	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	queueName := client.GetIndexerQueue(chainID)
	baseReq := func(t enumspb.TaskQueueType) *workflowservicepb.DescribeTaskQueueRequest {
		return &workflowservicepb.DescribeTaskQueueRequest{
			Namespace:     client.Namespace,
			TaskQueue:     &taskqueuepb.TaskQueue{Name: queueName},
			TaskQueueType: t,
			ReportStats:   true,
		}
	}

	resp, err := svc.DescribeTaskQueue(reqCtx, baseReq(enumspb.TASK_QUEUE_TYPE_WORKFLOW))
	if err != nil {
		return stats, err
	}
	if s := resp.GetStats(); s != nil {
		stats.PendingWorkflow = s.GetApproximateBacklogCount()
		if age := s.GetApproximateBacklogAge(); age != nil {
			stats.BacklogAgeSecs = float64(age.Seconds) + float64(age.Nanos)/1e9
		}
	}
	stats.Pollers = len(resp.GetPollers())

	if actResp, err := svc.DescribeTaskQueue(reqCtx, baseReq(enumspb.TASK_QUEUE_TYPE_ACTIVITY)); err == nil {
		if s := actResp.GetStats(); s != nil {
			stats.PendingActivity = s.GetApproximateBacklogCount()
		}
	}

	// Note: Cache storage is now handled by describeBothQueues.
	// This function does not update the cache to avoid conflicts.

	return stats, nil
}

// describeBothQueues fetches metrics for both the ops queue and indexer queue for a given chain.
// It uses caching with a 30s TTL to reduce load on Temporal's API.
// Returns (opsQueue, indexerQueue, error).
func (c *Controller) describeBothQueues(ctx context.Context, chainID string) (admintypes.QueueStatus, admintypes.QueueStatus, error) {
	opsStats := admintypes.QueueStatus{}
	indexerStats := admintypes.QueueStatus{}

	client := c.App.TemporalClient
	if client == nil {
		return opsStats, indexerStats, fmt.Errorf("temporal client not initialized")
	}

	// Check cache first (30s TTL to reduce Temporal API rate limiting)
	if cached, ok := c.App.QueueStatsCache.Load(chainID); ok {
		if time.Since(cached.Fetched) < 30*time.Second {
			return cached.OpsQueue, cached.IndexerQueue, nil
		}
	}

	svc := client.TClient.WorkflowService()
	if svc == nil {
		return opsStats, indexerStats, fmt.Errorf("temporal workflow service unavailable")
	}

	reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	// Helper function to query a specific queue
	describeQueue := func(queueName string) (admintypes.QueueStatus, error) {
		stats := admintypes.QueueStatus{}

		baseReq := func(t enumspb.TaskQueueType) *workflowservicepb.DescribeTaskQueueRequest {
			return &workflowservicepb.DescribeTaskQueueRequest{
				Namespace:     client.Namespace,
				TaskQueue:     &taskqueuepb.TaskQueue{Name: queueName},
				TaskQueueType: t,
				ReportStats:   true,
			}
		}

		// Get workflow queue stats
		resp, err := svc.DescribeTaskQueue(reqCtx, baseReq(enumspb.TASK_QUEUE_TYPE_WORKFLOW))
		if err != nil {
			return stats, err
		}
		if s := resp.GetStats(); s != nil {
			stats.PendingWorkflow = s.GetApproximateBacklogCount()
			if age := s.GetApproximateBacklogAge(); age != nil {
				stats.BacklogAgeSecs = float64(age.Seconds) + float64(age.Nanos)/1e9
			}
		}
		stats.Pollers = len(resp.GetPollers())

		// Get activity queue stats (best effort, ignore errors)
		if actResp, err := svc.DescribeTaskQueue(reqCtx, baseReq(enumspb.TASK_QUEUE_TYPE_ACTIVITY)); err == nil {
			if s := actResp.GetStats(); s != nil {
				stats.PendingActivity = s.GetApproximateBacklogCount()
			}
		}

		return stats, nil
	}

	// Query both queues in parallel for better performance
	type queueResult struct {
		stats admintypes.QueueStatus
		err   error
	}

	opsChannel := make(chan queueResult, 1)
	indexerChannel := make(chan queueResult, 1)

	// Query ops queue
	go func() {
		stats, err := describeQueue(client.GetIndexerOpsQueue(chainID))
		opsChannel <- queueResult{stats: stats, err: err}
	}()

	// Query indexer queue
	go func() {
		stats, err := describeQueue(client.GetIndexerQueue(chainID))
		indexerChannel <- queueResult{stats: stats, err: err}
	}()

	// Collect results
	opsResult := <-opsChannel
	indexerResult := <-indexerChannel

	// If either query failed, return the error
	if opsResult.err != nil {
		return opsStats, indexerStats, fmt.Errorf("failed to describe ops queue: %w", opsResult.err)
	}
	if indexerResult.err != nil {
		return opsStats, indexerStats, fmt.Errorf("failed to describe indexer queue: %w", indexerResult.err)
	}

	opsStats = opsResult.stats
	indexerStats = indexerResult.stats

	// Store both in cache
	c.App.QueueStatsCache.Store(chainID, admintypes.CachedQueueStats{
		OpsQueue:     opsStats,
		IndexerQueue: indexerStats,
		Fetched:      time.Now(),
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

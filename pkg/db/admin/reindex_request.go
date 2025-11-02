package admin

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/admin"
)

// ReindexWorkflowInfo contains workflow execution information for a reindex request.
type ReindexWorkflowInfo struct {
	Height     uint64
	WorkflowID string
	RunID      string
}

// initReindexRequests creates the reindex request log table.
// Engine: ReplacingMergeTree(requested_at)
// Order: (chain_id, requested_at, height)
func (db *AdminDB) initReindexRequests(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS reindex_requests (
			chain_id String,
			height UInt64,
			requested_by String,
			status String DEFAULT 'queued',
			workflow_id String,
			run_id String,
			requested_at DateTime DEFAULT now()
		) ENGINE = ReplacingMergeTree(requested_at)
		ORDER BY (chain_id, requested_at, height)
	`
	return db.Db.Exec(ctx, query)
}

// RecordReindexRequests logs a set of reindex requests for auditing purposes.
func (db *AdminDB) RecordReindexRequests(ctx context.Context, chainID uint64, requestedBy string, heights []uint64) error {
	if len(heights) == 0 {
		return nil
	}

	now := time.Now()
	for _, h := range heights {
		req := &admin.ReindexRequest{
			ChainID:     chainID,
			Height:      h,
			RequestedBy: requestedBy,
			Status:      "queued",
			RequestedAt: now,
		}
		if err := db.insertReindexRequest(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

// insertReindexRequest inserts a new reindex request record.
func (db *AdminDB) insertReindexRequest(ctx context.Context, req *admin.ReindexRequest) error {
	query := `
		INSERT INTO reindex_requests (chain_id, height, requested_by, status, workflow_id, run_id, requested_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	return db.Db.Exec(ctx, query,
		req.ChainID,
		req.Height,
		req.RequestedBy,
		req.Status,
		req.WorkflowID,
		req.RunID,
		req.RequestedAt,
	)
}

// RecordReindexRequestsWithWorkflow logs reindex requests with workflow execution information.
func (db *AdminDB) RecordReindexRequestsWithWorkflow(ctx context.Context, chainID uint64, requestedBy string, infos []ReindexWorkflowInfo) error {
	if len(infos) == 0 {
		return nil
	}

	now := time.Now()
	for _, info := range infos {
		req := &admin.ReindexRequest{
			ChainID:     chainID,
			Height:      info.Height,
			RequestedBy: requestedBy,
			Status:      "queued",
			WorkflowID:  info.WorkflowID,
			RunID:       info.RunID,
			RequestedAt: now,
		}
		if err := db.insertReindexRequest(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

// ListReindexRequests returns the most recent reindex requests for a chain.
func (db *AdminDB) ListReindexRequests(ctx context.Context, chainID string, limit int) ([]admin.ReindexRequest, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT chain_id, height, requested_by, status, workflow_id, run_id, requested_at
		FROM reindex_requests
		WHERE chain_id = ?
		ORDER BY requested_at DESC
		LIMIT ?
	`

	var rows []admin.ReindexRequest
	if err := db.Select(ctx, &rows, query, chainID, limit); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return rows, nil
}

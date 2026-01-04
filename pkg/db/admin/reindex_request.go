package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	adminmodels "github.com/canopy-network/canopyx/pkg/db/models/admin"
)

// ReindexWorkflowInfo contains workflow execution information for a reindex request.
type ReindexWorkflowInfo struct {
	Height     uint64
	WorkflowID string
	RunID      string
}

// initReindexRequests creates the reindex request log table.
// Engine: ReplicatedReplacingMergeTree(requested_at)
// Order: (chain_id, requested_at, height)
func (db *DB) initReindexRequests(ctx context.Context) error {
	schemaSQL := adminmodels.ColumnsToSchemaSQL(adminmodels.ReindexRequestColumns)
	engine := clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "requested_at")

	// Use a fully qualified table name to avoid ambiguity
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, requested_at, height)
	`, db.Name, adminmodels.ReindexRequestsTableName, schemaSQL, engine)

	err := db.Db.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create reindex_requests table: %w", err)
	}
	return nil
}

// RecordReindexRequests logs a set of reindex requests for auditing purposes.
func (db *DB) RecordReindexRequests(ctx context.Context, chainID uint64, requestedBy string, heights []uint64) error {
	if len(heights) == 0 {
		return nil
	}

	now := time.Now().UTC()
	for _, h := range heights {
		req := &adminmodels.ReindexRequest{
			ChainID:     chainID,
			Height:      h,
			RequestedBy: requestedBy,
			Status:      adminmodels.ReindexRequestStatusQueued,
			RequestedAt: now,
		}
		if err := db.insertReindexRequest(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

// insertReindexRequest inserts a new reindex request record.
func (db *DB) insertReindexRequest(ctx context.Context, req *adminmodels.ReindexRequest) error {
	query := fmt.Sprintf(`
		INSERT INTO "%s"."%s" (chain_id, height, requested_by, status, workflow_id, run_id, requested_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, db.Name, adminmodels.ReindexRequestsTableName)
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
func (db *DB) RecordReindexRequestsWithWorkflow(ctx context.Context, chainID uint64, requestedBy string, infos []ReindexWorkflowInfo) error {
	if len(infos) == 0 {
		return nil
	}

	now := time.Now()
	for _, info := range infos {
		req := &adminmodels.ReindexRequest{
			ChainID:     chainID,
			Height:      info.Height,
			RequestedBy: requestedBy,
			Status:      adminmodels.ReindexRequestStatusQueued,
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
func (db *DB) ListReindexRequests(ctx context.Context, chainID uint64, limit int) ([]adminmodels.ReindexRequest, error) {
	if limit <= 0 {
		limit = 10
	}

	query := fmt.Sprintf(`
		SELECT chain_id, height, requested_by, status, workflow_id, run_id, requested_at
		FROM "%s"."%s" FINAL
		WHERE chain_id = ?
		ORDER BY requested_at DESC
		LIMIT ?
	`, db.Name, adminmodels.ReindexRequestsTableName)

	var rows []adminmodels.ReindexRequest
	if err := db.SelectWithFinal(ctx, &rows, query, chainID, limit); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return rows, nil
}

// DeleteReindexRequestsForChain removes all reindex request records for a chain.
func (db *DB) DeleteReindexRequestsForChain(ctx context.Context, chainID uint64) error {
	query := fmt.Sprintf(
		`DELETE FROM "%s"."%s" WHERE chain_id = ?`,
		db.Name, adminmodels.ReindexRequestsTableName,
	)
	return db.Db.Exec(ctx, query, chainID)
}

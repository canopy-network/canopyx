package admin

import (
	"context"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type ReindexRequest struct {
	ChainID     string    `ch:"chain_id"`
	Height      uint64    `ch:"height"`
	RequestedBy string    `ch:"requested_by"`
	Status      string    `ch:"status"`
	WorkflowID  string    `ch:"workflow_id"`
	RunID       string    `ch:"run_id"`
	RequestedAt time.Time `ch:"requested_at"`
}

// InitReindexRequests creates the reindex request log table.
// Engine: ReplacingMergeTree(requested_at)
// Order: (chain_id, requested_at, height)
func InitReindexRequests(ctx context.Context, db driver.Conn) error {
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
	return db.Exec(ctx, query)
}

// InsertReindexRequest inserts a new reindex request record.
func InsertReindexRequest(ctx context.Context, db driver.Conn, req *ReindexRequest) error {
	query := `
		INSERT INTO reindex_requests (chain_id, height, requested_by, status, workflow_id, run_id, requested_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	return db.Exec(ctx, query,
		req.ChainID,
		req.Height,
		req.RequestedBy,
		req.Status,
		req.WorkflowID,
		req.RunID,
		req.RequestedAt,
	)
}

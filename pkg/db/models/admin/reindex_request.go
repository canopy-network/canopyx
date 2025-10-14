package admin

import (
	"context"
	"time"

	"github.com/uptrace/go-clickhouse/ch"
)

type ReindexRequest struct {
	ch.CHModel `ch:"table:reindex_requests"`

	ChainID     string    `ch:"chain_id"`
	Height      uint64    `ch:"height"`
	RequestedBy string    `ch:"requested_by"`
	Status      string    `ch:"status,default:'queued'"`
	RequestedAt time.Time `ch:"requested_at,default:now()"`
}

// InitReindexRequests creates the reindex request log table.
func InitReindexRequests(ctx context.Context, db *ch.DB) error {
	_, err := db.NewCreateTable().
		Model((*ReindexRequest)(nil)).
		IfNotExists().
		Engine("ReplacingMergeTree(requested_at)").
		Order("chain_id,requested_at, height").
		Exec(ctx)
	return err
}

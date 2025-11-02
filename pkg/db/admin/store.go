package admin

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/admin"
)

// Store exposes the subset of admin database operations used by activities and workflows.
type Store interface {
	GetChain(ctx context.Context, id uint64) (*admin.Chain, error)
	RecordIndexed(ctx context.Context, chainID uint64, height uint64, blockTime time.Time, indexingTimeMs float64, indexingDetail string) error
	ListChain(ctx context.Context) ([]admin.Chain, error)
	LastIndexed(ctx context.Context, chainID uint64) (uint64, error)
	FindGaps(ctx context.Context, chainID uint64) ([]Gap, error)
	UpdateRPCHealth(ctx context.Context, chainID uint64, status, message string) error
	IndexProgressHistory(ctx context.Context, chainID uint64, hours, intervalMinutes int) ([]admin.ProgressPoint, error)
}

package admin

import (
	"context"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/canopy-network/canopyx/pkg/db/models/admin"
)

// Store exposes the subset of admin database operations used by activities and workflows.
type Store interface {
	Close() error
	DatabaseName() string
	GetConnection() driver.Conn

	GetChain(ctx context.Context, id uint64) (*admin.Chain, error)
	GetChainRPCEndpoints(ctx context.Context, chainID uint64) (*ChainRPCEndpoints, error)
	RecordIndexed(ctx context.Context, chainID uint64, height uint64, indexingTimeMs float64, indexingDetail string) error
	ListChain(ctx context.Context, includeDeleted bool) ([]admin.Chain, error)
	LastIndexed(ctx context.Context, chainID uint64) (uint64, error)
	FindGaps(ctx context.Context, chainID uint64) ([]admin.Gap, error)
	GetCleanableHeights(ctx context.Context, chainID uint64, lookbackHours int) ([]uint64, error)
	UpdateRPCHealth(ctx context.Context, chainID uint64, status, message string) error
	IndexProgressHistory(ctx context.Context, chainID uint64, hours, intervalMinutes int) ([]admin.ProgressPoint, error)
	UpsertEndpointHealth(ctx context.Context, ep *admin.RPCEndpoint) error
	GetEndpointsForChain(ctx context.Context, chainID uint64) ([]admin.RPCEndpoint, error)
	GetEndpointsWithMinHeight(ctx context.Context, chainID uint64, minHeight uint64) ([]admin.RPCEndpoint, error)

	// Hard delete operations
	DropChainDatabase(ctx context.Context, chainID uint64) error
	DeleteEndpointsForChain(ctx context.Context, chainID uint64) error
	DeleteIndexProgressForChain(ctx context.Context, chainID uint64) error
	DeleteReindexRequestsForChain(ctx context.Context, chainID uint64) error
	HardDeleteChain(ctx context.Context, chainID uint64) error
}

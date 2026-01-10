package admin

import (
	"context"
	"github.com/canopy-network/canopyx/pkg/db/clickhouse"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/canopy-network/canopyx/pkg/db/models/admin"
)

// Store exposes the subset of admin database operations used by activities and workflows.
type Store interface {
	Close() error
	DatabaseName() string
	GetConnection() driver.Conn
	GetClient() clickhouse.Client

	GetChain(ctx context.Context, id uint64) (*admin.Chain, error)
	GetChainRPCEndpoints(ctx context.Context, chainID uint64) (*ChainRPCEndpoints, error)
	GetChainNamespaceInfo(ctx context.Context, chainID uint64) (*ChainNamespaceInfo, error)
	RecordIndexed(ctx context.Context, chainID uint64, height uint64, indexingTimeMs float64, timing *admin.IndexProgress) error
	HasIndexed(ctx context.Context, chainID uint64, height uint64) (bool, error)
	ListChain(ctx context.Context, includeDeleted bool) ([]admin.Chain, error)
	LastIndexed(ctx context.Context, chainID uint64) (uint64, error)
	UpsertChain(ctx context.Context, c *admin.Chain) error
	ListReindexRequests(ctx context.Context, chainID uint64, limit int) ([]admin.ReindexRequest, error)
	RecordReindexRequests(ctx context.Context, chainID uint64, requestedBy string, heights []uint64) error
	PatchChains(ctx context.Context, patches []admin.Chain) error
	FindGaps(ctx context.Context, chainID uint64) ([]admin.Gap, error)
	GetCleanableHeights(ctx context.Context, chainID uint64, lookbackHours int) ([]uint64, error)
	UpdateRPCHealth(ctx context.Context, chainID uint64, status, message string) error
	IndexProgressHistory(ctx context.Context, chainID uint64, hours, intervalSeconds int) ([]admin.ProgressPoint, error)
	UpsertEndpointHealth(ctx context.Context, ep *admin.RPCEndpoint) error
	GetEndpointsForChain(ctx context.Context, chainID uint64) ([]admin.RPCEndpoint, error)
	GetEndpointsWithMinHeight(ctx context.Context, chainID uint64, minHeight uint64) ([]admin.RPCEndpoint, error)

	// --- Hard delete operations

	DeleteEndpointsForChain(ctx context.Context, chainID uint64) error
	DeleteIndexProgressForChain(ctx context.Context, chainID uint64) error
	DeleteReindexRequestsForChain(ctx context.Context, chainID uint64) error
	HardDeleteChain(ctx context.Context, chainID uint64) error
}

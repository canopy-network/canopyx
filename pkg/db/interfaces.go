package db

import (
	"context"

	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// AdminStore exposes the subset of admin database operations used by activities and workflows.
type AdminStore interface {
	GetChain(ctx context.Context, id string) (*admin.Chain, error)
	RecordIndexed(ctx context.Context, chainID string, height uint64) error
	ListChain(ctx context.Context) ([]admin.Chain, error)
	LastIndexed(ctx context.Context, chainID string) (uint64, error)
	FindGaps(ctx context.Context, chainID string) ([]Gap, error)
}

// ChainStore describes the per-chain database operations required by indexer and reporter activities.
type ChainStore interface {
	DatabaseName() string
	ChainKey() string
	InsertBlock(ctx context.Context, block *indexer.Block) error
	InsertTransactions(ctx context.Context, txs []*indexer.Transaction, raw []*indexer.TransactionRaw) error
	Exec(ctx context.Context, query string, args ...any) error
	Close() error
}

// ReportsStore exposes the reports database helpers referenced by reporter activities.
type ReportsStore interface {
	DatabaseName() string
	Close() error
}

package db

import (
	"context"

	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/uptrace/go-clickhouse/ch"
)

// AdminStore exposes the subset of admin database operations used by activities and workflows.
type AdminStore interface {
	GetChain(ctx context.Context, id string) (*admin.Chain, error)
	RecordIndexed(ctx context.Context, chainID string, height uint64) error
	ListChain(ctx context.Context) ([]admin.Chain, error)
	LastIndexed(ctx context.Context, chainID string) (uint64, error)
	FindGaps(ctx context.Context, chainID string) ([]Gap, error)
	UpdateRPCHealth(ctx context.Context, chainID, status, message string) error
}

// ChainStore describes the per-chain database operations required by indexer and reporter activities.
type ChainStore interface {
	DatabaseName() string
	ChainKey() string
	InsertBlock(ctx context.Context, block *indexer.Block) error
	InsertTransactions(ctx context.Context, txs []*indexer.Transaction, raw []*indexer.TransactionRaw) error
	HasBlock(ctx context.Context, height uint64) (bool, error)
	DeleteBlock(ctx context.Context, height uint64) error
	DeleteTransactions(ctx context.Context, height uint64) error
	Exec(ctx context.Context, query string, args ...any) error
	RawDB() *ch.DB
	Close() error
}

// ReportsStore exposes the reports database helpers referenced by reporter activities.
type ReportsStore interface {
	DatabaseName() string
	Close() error
}

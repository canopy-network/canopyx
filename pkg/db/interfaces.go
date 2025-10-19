package db

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/admin"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// AdminStore exposes the subset of admin database operations used by activities and workflows.
type AdminStore interface {
	GetChain(ctx context.Context, id string) (*admin.Chain, error)
	RecordIndexed(ctx context.Context, chainID string, height uint64, indexingTimeMs float64, indexingDetail string) error
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
	GetBlock(ctx context.Context, height uint64) (*indexer.Block, error)
	InsertTransactions(ctx context.Context, txs []*indexer.Transaction, raw []*indexer.TransactionRaw) error
	InsertBlockSummary(ctx context.Context, height uint64, blockTime time.Time, numTxs uint32) error
	GetBlockSummary(ctx context.Context, height uint64) (*indexer.BlockSummary, error)
	HasBlock(ctx context.Context, height uint64) (bool, error)
	DeleteBlock(ctx context.Context, height uint64) error
	DeleteTransactions(ctx context.Context, height uint64) error
	Exec(ctx context.Context, query string, args ...any) error
	QueryBlocks(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.Block, error)
	QueryBlockSummaries(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.BlockSummary, error)
	QueryTransactions(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.Transaction, error)
	QueryTransactionsRaw(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]map[string]interface{}, error)
	DescribeTable(ctx context.Context, tableName string) ([]Column, error)
	Close() error
}

// ReportsStore exposes the reports database helpers referenced by reporter activities.
type ReportsStore interface {
	DatabaseName() string
	Close() error
}

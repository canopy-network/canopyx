package db

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/entities"
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
	InsertBlocksStaging(ctx context.Context, block *indexer.Block) error
	GetBlock(ctx context.Context, height uint64) (*indexer.Block, error)
	InsertTransactions(ctx context.Context, txs []*indexer.Transaction) error
	InsertTransactionsStaging(ctx context.Context, txs []*indexer.Transaction) error
	GetTransactionByHash(ctx context.Context, txHash string) (*indexer.Transaction, error)
	InsertBlockSummary(ctx context.Context, height uint64, blockTime time.Time, numTxs uint32, txCountsByType map[string]uint32) error
	InsertBlockSummariesStaging(ctx context.Context, height uint64, blockTime time.Time, numTxs uint32, txCountsByType map[string]uint32) error
	GetBlockSummary(ctx context.Context, height uint64) (*indexer.BlockSummary, error)
	HasBlock(ctx context.Context, height uint64) (bool, error)
	DeleteBlock(ctx context.Context, height uint64) error
	DeleteTransactions(ctx context.Context, height uint64) error
	Exec(ctx context.Context, query string, args ...any) error
	QueryBlocks(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.Block, error)
	QueryBlockSummaries(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.BlockSummary, error)
	QueryTransactions(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.Transaction, error)
	QueryTransactionsWithFilter(ctx context.Context, cursor uint64, limit int, sortDesc bool, messageType string) ([]indexer.Transaction, error)
	QueryAccounts(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.Account, error)
	QueryEvents(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.Event, error)
	QueryEventsWithFilter(ctx context.Context, cursor uint64, limit int, sortDesc bool, eventType string) ([]indexer.Event, error)
	QueryPools(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.Pool, error)
	QueryOrders(ctx context.Context, cursor uint64, limit int, sortDesc bool, status string) ([]indexer.Order, error)
	QueryDexPrices(ctx context.Context, cursor uint64, limit int, sortDesc bool, localChainID, remoteChainID uint64) ([]indexer.DexPrice, error)
	GetAccountByAddress(ctx context.Context, address string, height *uint64) (*indexer.Account, error)
	QueryTransactionsRaw(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]map[string]interface{}, error)
	DescribeTable(ctx context.Context, tableName string) ([]Column, error)
	PromoteEntity(ctx context.Context, entity entities.Entity, height uint64) error
	CleanEntityStaging(ctx context.Context, entity entities.Entity, height uint64) error
	ValidateQueryHeight(ctx context.Context, requestedHeight *uint64) (uint64, error)
	GetFullyIndexedHeight(ctx context.Context) (uint64, error)
	InitEvents(ctx context.Context) error
	InsertEventsStaging(ctx context.Context, events []*indexer.Event) error
	InitDexPrices(ctx context.Context) error
	InsertDexPricesStaging(ctx context.Context, prices []*indexer.DexPrice) error
	InitPools(ctx context.Context) error
	InsertPoolsStaging(ctx context.Context, pools []*indexer.Pool) error
	GetDexVolume24h(ctx context.Context) ([]DexVolumeStats, error)
	GetOrderBookDepth(ctx context.Context, committee uint64, limit int) ([]OrderBookLevel, error)
	Close() error
}

// ReportsStore exposes the reports database helpers referenced by reporter activities.
type ReportsStore interface {
	DatabaseName() string
	Close() error
}

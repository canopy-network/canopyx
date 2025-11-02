package chain

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/entities"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// Store describes the per-chain database operations required by indexermodels and reporter activities.
type Store interface {
	DatabaseName() string
	ChainKey() string

	// --- Init

	InitializeDB(ctx context.Context) error

	// --- Insert entities (staging-first pattern: InsertXStaging + PromoteEntity)

	InsertAccountsStaging(ctx context.Context, accounts []*indexermodels.Account) error
	InsertBlocksStaging(ctx context.Context, block *indexermodels.Block) error
	InsertTransactionsStaging(ctx context.Context, txs []*indexermodels.Transaction) error
	InsertBlockSummariesStaging(ctx context.Context, height uint64, blockTime time.Time, numTxs uint32, txCountsByType map[string]uint32) error
	InsertEventsStaging(ctx context.Context, events []*indexermodels.Event) error
	InsertDexPricesStaging(ctx context.Context, prices []*indexermodels.DexPrice) error
	InsertPoolsStaging(ctx context.Context, pools []*indexermodels.Pool) error
	InsertOrdersStaging(ctx context.Context, orders []*indexermodels.Order) error
	InsertDexOrdersStaging(ctx context.Context, orders []*indexermodels.DexOrder) error
	InsertDexDepositsStaging(ctx context.Context, deposits []*indexermodels.DexDeposit) error
	InsertDexWithdrawalsStaging(ctx context.Context, withdrawals []*indexermodels.DexWithdrawal) error
	InsertDexPoolPointsByHolderStaging(ctx context.Context, holders []*indexermodels.DexPoolPointsByHolder) error
	InsertGenesis(ctx context.Context, height uint64, data string, fetchedAt time.Time) error

	// --- Query entities

	GetBlock(ctx context.Context, height uint64) (*indexermodels.Block, error)
	GetBlockSummary(ctx context.Context, height uint64) (*indexermodels.BlockSummary, error)
	HasBlock(ctx context.Context, height uint64) (bool, error)
	GetGenesisData(ctx context.Context, height uint64) (string, error)
	HasGenesis(ctx context.Context, height uint64) (bool, error)
	GetEventsByTypeAndHeight(ctx context.Context, height uint64, eventTypes ...string) ([]*indexermodels.Event, error)

	// --- Delete entities

	DeleteBlock(ctx context.Context, height uint64) error
	DeleteTransactions(ctx context.Context, height uint64) error

	// --- Meta / Help queries

	Exec(ctx context.Context, query string, args ...any) error
	Select(ctx context.Context, dest interface{}, query string, args ...any) error
	Close() error
	DescribeTable(ctx context.Context, tableName string) ([]Column, error)
	GetTableSchema(ctx context.Context, tableName string) ([]Column, error)
	GetTableDataPaginated(ctx context.Context, tableName string, limit, offset int, fromHeight, toHeight *uint64) ([]map[string]interface{}, int64, bool, error)
	PromoteEntity(ctx context.Context, entity entities.Entity, height uint64) error
	CleanEntityStaging(ctx context.Context, entity entities.Entity, height uint64) error
}

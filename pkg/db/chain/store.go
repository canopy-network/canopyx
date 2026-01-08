package chain

import (
	"context"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/canopy-network/canopyx/pkg/db/entities"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// Store describes the per-chain database operations required by indexermodels and reporter activities.
type Store interface {
	DatabaseName() string
	ChainKey() string
	GetConnection() driver.Conn

	// --- Init

	InitializeDB(ctx context.Context) error

	// --- Insert entities (staging-first pattern: InsertXStaging + PromoteEntity)

	InsertAccountsStaging(ctx context.Context, accounts []*indexermodels.Account) error
	InsertAccountsProduction(ctx context.Context, accounts []*indexermodels.Account) error
	InsertBlocksStaging(ctx context.Context, block *indexermodels.Block) error
	InsertBlocksProduction(ctx context.Context, block *indexermodels.Block) error
	InsertTransactionsStaging(ctx context.Context, txs []*indexermodels.Transaction) error
	InsertTransactionsProduction(ctx context.Context, txs []*indexermodels.Transaction) error
	InsertBlockSummariesStaging(ctx context.Context, summary *indexermodels.BlockSummary) error
	InsertBlockSummariesProduction(ctx context.Context, summary *indexermodels.BlockSummary) error
	InsertEventsStaging(ctx context.Context, events []*indexermodels.Event) error
	InsertEventsProduction(ctx context.Context, events []*indexermodels.Event) error
	InsertDexPricesStaging(ctx context.Context, prices []*indexermodels.DexPrice) error
	InsertDexPricesProduction(ctx context.Context, prices []*indexermodels.DexPrice) error
	InsertPoolsStaging(ctx context.Context, pools []*indexermodels.Pool) error
	InsertPoolsProduction(ctx context.Context, pools []*indexermodels.Pool) error
	InsertOrdersStaging(ctx context.Context, orders []*indexermodels.Order) error
	InsertOrdersProduction(ctx context.Context, orders []*indexermodels.Order) error
	InsertDexOrdersStaging(ctx context.Context, orders []*indexermodels.DexOrder) error
	InsertDexOrdersProduction(ctx context.Context, orders []*indexermodels.DexOrder) error
	InsertDexDepositsStaging(ctx context.Context, deposits []*indexermodels.DexDeposit) error
	InsertDexDepositsProduction(ctx context.Context, deposits []*indexermodels.DexDeposit) error
	InsertDexWithdrawalsStaging(ctx context.Context, withdrawals []*indexermodels.DexWithdrawal) error
	InsertDexWithdrawalsProduction(ctx context.Context, withdrawals []*indexermodels.DexWithdrawal) error
	InsertPoolPointsByHolderStaging(ctx context.Context, holders []*indexermodels.PoolPointsByHolder) error
	InsertPoolPointsByHolderProduction(ctx context.Context, holders []*indexermodels.PoolPointsByHolder) error
	InsertParamsStaging(ctx context.Context, params *indexermodels.Params) error
	InsertParamsProduction(ctx context.Context, params *indexermodels.Params) error
	InsertValidatorsStaging(ctx context.Context, validators []*indexermodels.Validator) error
	InsertValidatorsProduction(ctx context.Context, validators []*indexermodels.Validator) error
	InsertValidatorNonSigningInfoStaging(ctx context.Context, nonSigningInfos []*indexermodels.ValidatorNonSigningInfo) error
	InsertValidatorNonSigningInfoProduction(ctx context.Context, nonSigningInfos []*indexermodels.ValidatorNonSigningInfo) error
	InsertValidatorDoubleSigningInfoStaging(ctx context.Context, doubleSigningInfos []*indexermodels.ValidatorDoubleSigningInfo) error
	InsertValidatorDoubleSigningInfoProduction(ctx context.Context, doubleSigningInfos []*indexermodels.ValidatorDoubleSigningInfo) error
	InsertCommitteesStaging(ctx context.Context, committees []*indexermodels.Committee) error
	InsertCommitteesProduction(ctx context.Context, committees []*indexermodels.Committee) error
	InsertCommitteeValidatorsStaging(ctx context.Context, cvs []*indexermodels.CommitteeValidator) error
	InsertCommitteeValidatorsProduction(ctx context.Context, cvs []*indexermodels.CommitteeValidator) error
	InsertCommitteePaymentsStaging(ctx context.Context, payments []*indexermodels.CommitteePayment) error
	InsertCommitteePaymentsProduction(ctx context.Context, payments []*indexermodels.CommitteePayment) error
	InsertPollSnapshots(ctx context.Context, snapshots []*indexermodels.PollSnapshot) error
	InsertProposalSnapshots(ctx context.Context, snapshots []*indexermodels.ProposalSnapshot) error
	InsertSupplyStaging(ctx context.Context, supplies []*indexermodels.Supply) error
	InsertSupplyProduction(ctx context.Context, supplies []*indexermodels.Supply) error

	// --- Query entities

	GetBlock(ctx context.Context, height uint64) (*indexermodels.Block, error)
	GetBlockTime(ctx context.Context, height uint64) (time.Time, error)
	GetBlockSummary(ctx context.Context, height uint64, staging bool) (*indexermodels.BlockSummary, error)
	HasBlock(ctx context.Context, height uint64) (bool, error)
	GetHighestBlockBeforeTime(ctx context.Context, targetTime time.Time) (*indexermodels.Block, error)
	GetEventsByTypeAndHeight(ctx context.Context, height uint64, staging bool, eventTypes ...string) ([]*indexermodels.Event, error)

	// --- Lightweight event queries (optimized for specific activities)

	GetRewardSlashEvents(ctx context.Context, height uint64, staging bool) ([]EventRewardSlash, error)
	GetValidatorLifecycleEvents(ctx context.Context, height uint64, staging bool) ([]EventLifecycle, error)
	GetOrderBookSwapEvents(ctx context.Context, height uint64, staging bool) ([]EventWithOrderID, error)
	GetDexOrderEvents(ctx context.Context, height uint64, staging bool) ([]EventWithOrderID, error)
	GetEventCountsByType(ctx context.Context, height uint64, staging bool, eventTypes ...string) (map[string]uint64, error)
	GetDexBatchEvents(ctx context.Context, height uint64, staging bool) ([]EventDexBatch, error)

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
	CleanAllEntitiesStaging(ctx context.Context, entitiesToClean []entities.Entity, height uint64) error
	CleanStagingBatch(ctx context.Context, heights []uint64) (int, error)
}

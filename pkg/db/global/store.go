package global

import (
    "context"
    "time"

    "github.com/ClickHouse/clickhouse-go/v2/lib/driver"

    "github.com/canopy-network/canopyx/pkg/db/clickhouse"
    "github.com/canopy-network/canopyx/pkg/db/entities"
    indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// Store describes the global database operations required by indexer and reporter activities.
// This interface operates on a single database with chain_id context.
// All write operations use the chain_id stored in the DB instance.
// Note: Staging pattern has been eliminated - all writes go directly to production tables.
type Store interface {
    DatabaseName() string
    ChainKey() string
    GetChainID() uint64
    GetConnection() driver.Conn
    GetClient() clickhouse.Client

    // --- Init

    InitializeDB(ctx context.Context) error

    // --- Insert entities (direct to production)

    InsertAccounts(ctx context.Context, accounts []*indexermodels.Account) error
    InsertBlocks(ctx context.Context, block *indexermodels.Block) error
    InsertTransactions(ctx context.Context, txs []*indexermodels.Transaction) error
    InsertBlockSummaries(ctx context.Context, summary *indexermodels.BlockSummary) error
    InsertEvents(ctx context.Context, events []*indexermodels.Event) error
    InsertDexPrices(ctx context.Context, prices []*indexermodels.DexPrice) error
    InsertPools(ctx context.Context, pools []*indexermodels.Pool) error
    InsertOrders(ctx context.Context, orders []*indexermodels.Order) error
    InsertDexOrders(ctx context.Context, orders []*indexermodels.DexOrder) error
    InsertDexDeposits(ctx context.Context, deposits []*indexermodels.DexDeposit) error
    InsertDexWithdrawals(ctx context.Context, withdrawals []*indexermodels.DexWithdrawal) error
    InsertPoolPointsByHolder(ctx context.Context, holders []*indexermodels.PoolPointsByHolder) error
    InsertParams(ctx context.Context, params *indexermodels.Params) error
    InsertValidators(ctx context.Context, validators []*indexermodels.Validator) error
    InsertValidatorNonSigningInfo(ctx context.Context, nonSigningInfos []*indexermodels.ValidatorNonSigningInfo) error
    InsertValidatorDoubleSigningInfo(ctx context.Context, doubleSigningInfos []*indexermodels.ValidatorDoubleSigningInfo) error
    InsertCommittees(ctx context.Context, committees []*indexermodels.Committee) error
    InsertCommitteeValidators(ctx context.Context, cvs []*indexermodels.CommitteeValidator) error
    InsertCommitteePayments(ctx context.Context, payments []*indexermodels.CommitteePayment) error
    InsertPollSnapshots(ctx context.Context, snapshots []*indexermodels.PollSnapshot) error
    InsertProposalSnapshots(ctx context.Context, snapshots []*indexermodels.ProposalSnapshot) error
    InsertSupply(ctx context.Context, supplies []*indexermodels.Supply) error
    InsertLPPositionSnapshots(ctx context.Context, snapshots []*indexermodels.LPPositionSnapshot) error

    // --- Query entities

    GetBlock(ctx context.Context, height uint64) (*indexermodels.Block, error)
    GetBlockTime(ctx context.Context, height uint64) (time.Time, error)
    GetBlockSummary(ctx context.Context, height uint64) (*indexermodels.BlockSummary, error)
    HasBlock(ctx context.Context, height uint64) (bool, error)
    GetHighestBlockBeforeTime(ctx context.Context, targetTime time.Time) (*indexermodels.Block, error)
    GetEventsByTypeAndHeight(ctx context.Context, height uint64, eventTypes ...string) ([]*indexermodels.Event, error)

    // --- Lightweight event queries (optimized for specific activities)

    GetRewardSlashEvents(ctx context.Context, height uint64) ([]EventRewardSlash, error)
    GetValidatorLifecycleEvents(ctx context.Context, height uint64) ([]EventLifecycle, error)
    GetOrderBookSwapEvents(ctx context.Context, height uint64) ([]EventWithOrderID, error)
    GetDexOrderEvents(ctx context.Context, height uint64) ([]EventWithOrderID, error)
    GetEventCountsByType(ctx context.Context, height uint64, eventTypes ...string) (map[string]uint64, error)
    GetDexBatchEvents(ctx context.Context, height uint64) ([]EventDexBatch, error)

    // --- LP Position Snapshot queries

    QueryLPPositionSnapshots(ctx context.Context, params LPPositionSnapshotQueryParams) ([]*indexermodels.LPPositionSnapshot, error)
    GetLatestLPPositionSnapshot(ctx context.Context, sourceChainID uint64, address string, poolID uint64) (*indexermodels.LPPositionSnapshot, error)

    // --- Delete entities (single height)

    DeleteBlock(ctx context.Context, height uint64) error
    DeleteTransactions(ctx context.Context, height uint64) error

    // --- Delete chain data (for hard-delete operations)

    // DeleteChainData removes ALL data for a chain_id from ALL tables
    DeleteChainData(ctx context.Context, chainID uint64) error
    // DeleteEntityData removes ALL data for a chain_id from a specific entity's tables
    DeleteEntityData(ctx context.Context, chainID uint64, entity entities.Entity) error
    // DeleteHeightRange removes data for a chain within a height range from ALL tables
    DeleteHeightRange(ctx context.Context, chainID uint64, fromHeight, toHeight uint64) error

    // --- Meta / Help queries

    Exec(ctx context.Context, query string, args ...any) error
    Select(ctx context.Context, dest interface{}, query string, args ...any) error
    Close() error
    DescribeTable(ctx context.Context, tableName string) ([]Column, error)
    GetTableSchema(ctx context.Context, tableName string) ([]Column, error)
    GetTableDataPaginated(ctx context.Context, tableName string, limit, offset int, fromHeight, toHeight *uint64) ([]map[string]interface{}, int64, bool, error)
}

// Verify DB implements Store at compile time
var _ Store = (*DB)(nil)

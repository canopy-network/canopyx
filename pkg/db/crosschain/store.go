package crosschain

import (
	"context"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

type Store interface {
	DatabaseName() string
	GetConnection() driver.Conn

	// --- Init

	InitializeDB(ctx context.Context) error
	Close() error
	Exec(ctx context.Context, query string, args ...interface{}) error
	QueryRow(ctx context.Context, query string, args ...interface{}) driver.Row
	Query(ctx context.Context, query string, args ...interface{}) (driver.Rows, error)
	Select(ctx context.Context, dest interface{}, query string, args ...interface{}) error

	// --- Sync

	SetupChainSync(ctx context.Context, chainID uint64) error
	GetSyncStatus(ctx context.Context, chainID uint64, tableName string) (*SyncStatus, error)
	ResyncTable(ctx context.Context, chainID uint64, tableName string) error
	ResyncChain(ctx context.Context, chainID uint64) error
	RemoveChainSync(ctx context.Context, chainID uint64) error

	// --- Health

	GetHealthStatus(ctx context.Context) (*HealthStatus, error)

	// --- Maintenance

	OptimizeTables(ctx context.Context) error
	OptimizeTable(ctx context.Context, database, table string, final bool) error

	// --- Queries

	QueryLatestAccounts(ctx context.Context, opts QueryOptions) ([]AccountCrossChain, *QueryMetadata, error)
	QueryLatestValidators(ctx context.Context, opts QueryOptions) ([]ValidatorCrossChain, *QueryMetadata, error)
	QueryLatestValidatorNonSigningInfo(ctx context.Context, opts QueryOptions) ([]ValidatorNonSigningInfoCrossChain, *QueryMetadata, error)
	QueryLatestValidatorDoubleSigningInfo(ctx context.Context, opts QueryOptions) ([]ValidatorDoubleSigningInfoCrossChain, *QueryMetadata, error)
	QueryLatestPools(ctx context.Context, opts QueryOptions) ([]PoolCrossChain, *QueryMetadata, error)
	QueryLatestPoolPointsByHolder(ctx context.Context, opts QueryOptions) ([]PoolPointsByHolderCrossChain, *QueryMetadata, error)
	QueryLatestDexOrders(ctx context.Context, opts QueryOptions) ([]DexOrderCrossChain, *QueryMetadata, error)
	QueryLatestDexDeposits(ctx context.Context, opts QueryOptions) ([]DexDepositCrossChain, *QueryMetadata, error)
	QueryLatestDexWithdrawals(ctx context.Context, opts QueryOptions) ([]DexWithdrawalCrossChain, *QueryMetadata, error)
	QueryLatestBlockSummaries(ctx context.Context, opts QueryOptions) ([]BlockSummaryCrossChain, *QueryMetadata, error)
	QueryLatestCommitteePayments(ctx context.Context, opts QueryOptions) ([]CommitteePaymentCrossChain, *QueryMetadata, error)
	QueryLatestEvents(ctx context.Context, opts QueryOptions) ([]EventCrossChain, *QueryMetadata, error)
	QueryLatestTransactions(ctx context.Context, opts QueryOptions) ([]TransactionCrossChain, *QueryMetadata, error)
	QueryLatestEntity(ctx context.Context, entityName string, opts QueryOptions) ([]map[string]interface{}, *QueryMetadata, error)
	QueryLPPositionSnapshots(ctx context.Context, params LPPositionSnapshotQueryParams) ([]*indexermodels.LPPositionSnapshot, error)
	GetLatestLPPositionSnapshot(ctx context.Context, sourceChainID uint64, address string, poolID uint64) (*indexermodels.LPPositionSnapshot, error)

	// --- Inserts

	InsertLPPositionSnapshots(ctx context.Context, snapshots []*indexermodels.LPPositionSnapshot) error
	InsertTVLSnapshots(ctx context.Context, snapshots []*indexermodels.TVLSnapshot) error
}

package db

import (
	"context"
	"strings"

	"github.com/canopy-network/canopyx/pkg/utils"
	"go.uber.org/zap"

	"github.com/uptrace/go-clickhouse/ch"
	"github.com/uptrace/go-clickhouse/chdebug"
)

type Client struct {
	Logger *zap.Logger
	Db     *ch.DB
}

// NewDB initializes and returns a new database client for ClickHouse with provided context and logger.
func NewDB(ctx context.Context, logger *zap.Logger, dbName string) (client Client, e error) {
	dsn := utils.Env("CLICKHOUSE_ADDR", "clickhouse://localhost:9000?sslmode=disable")

	client.Logger = logger
	client.Db = ch.Connect(
		// clickhouse://<user>:<password>@<host>:<port>/<database>?sslmode=disable
		ch.WithDSN(dsn),
		ch.WithAutoCreateDatabase(true),
		ch.WithDatabase(dbName),
		ch.WithQuerySettings(map[string]interface{}{
			"prefer_column_name_to_alias": 1,
			// this could be useful for some data (but we should avoid 100% - so maybe restrict will be better)
			"allow_experimental_object_type": 1,
		}),
		ch.WithCompression(true),
	)

	client.Db.AddQueryHook(chdebug.NewQueryHook(
		// disable the hook
		chdebug.WithVerbose(true),
		// CHDEBUG=1 logs failed queries
		// CHDEBUG=2 logs all queries
		chdebug.FromEnv("CHDEBUG"),
	))

	logger.Info("Pinging ClickHouse connection", zap.String("dsn", dsn))
	err := client.Db.Ping(ctx)
	if err != nil {
		return client, err
	}

	logger.Info("ClickHouse connection ready to work", zap.String("dsn", dsn))
	return client, nil
}

// NewBasicDbs creates and returns a new instance of AdminDB and ReportsDB configured with the provided client.
func NewBasicDbs(ctx context.Context, logger *zap.Logger) (*AdminDB, *ReportsDB, error) {
	// Database name for the db that hold chains to be indexed and the tracking of them
	indexerDbName := utils.Env("INDEXER_DB", "canopyx_indexer")
	// Database name for the db that holds reports across chains
	reportsDbName := utils.Env("REPORTS_DB", "canopyx_reports")

	logger.Info("Creating databases", zap.String("indexerDbName", indexerDbName), zap.String("reportsDbName", reportsDbName))
	indexerDb, indexerDbErr := NewDB(ctx, logger, indexerDbName)
	if indexerDbErr != nil {
		return nil, nil, indexerDbErr
	}

	reportsDb, reportsDbErr := NewDB(ctx, logger, reportsDbName)
	if reportsDbErr != nil {
		return nil, nil, reportsDbErr
	}

	return &AdminDB{
			Client: indexerDb,
			Name:   indexerDbName,
		}, &ReportsDB{
			Client: reportsDb,
			Name:   reportsDbName,
		}, nil
}

// NewChainDb creates and returns a new instance of ChainDB configured with the provided client.
func NewChainDb(ctx context.Context, logger *zap.Logger, chainId string) (*ChainDB, error) {
	dbName := SanitizeDbName(chainId)
	chainDb, chainDbErr := NewDB(ctx, logger.With(
		zap.String("db", dbName),
		zap.String("component", "chain_db"),
		zap.String("chainID", chainId),
	), dbName)
	if chainDbErr != nil {
		return nil, chainDbErr
	}

	chainDbWrapper := &ChainDB{
		Client:  chainDb,
		Name:    dbName,
		ChainID: chainId,
	}

	chainDbInitErr := chainDbWrapper.InitializeDB(ctx)
	if chainDbInitErr != nil {
		return nil, chainDbInitErr
	}

	return chainDbWrapper, nil
}

func SanitizeDbName(id string) string {
	s := strings.ToLower(id)
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	return s
}

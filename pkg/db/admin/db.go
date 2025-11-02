package admin

import (
	"context"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	"go.uber.org/zap"
)

// AdminDB represents a database connection for handling indexing operations and tracking progress.
type AdminDB struct {
	clickhouse.Client
	Name string
}

// New creates and initializes the admin database.
func New(ctx context.Context, logger *zap.Logger, name string) (*AdminDB, error) {
	client, err := clickhouse.New(ctx, logger.With(
		zap.String("db", name),
		zap.String("component", "admin_db"),
	), name)
	if err != nil {
		return nil, err
	}

	adminDB := &AdminDB{
		Client: client,
		Name:   name,
	}

	if err := adminDB.InitializeDB(ctx); err != nil {
		return nil, err
	}

	return adminDB, nil
}

// Close terminates the underlying ClickHouse connection.
func (db *AdminDB) Close() error {
	return db.Db.Close()
}

// InitializeDB ensures the required database and tables for indexing are created if they do not already exist.
func (db *AdminDB) InitializeDB(ctx context.Context) error {
	db.Logger.Debug("Initializing indexer database", zap.String("name", db.Name))

	db.Logger.Debug("Initialize chains model", zap.String("name", db.Name))
	if err := db.initChains(ctx); err != nil {
		return err
	}

	db.Logger.Debug("Initialize index_progress model", zap.String("name", db.Name))
	err := db.initIndexProgress(ctx)
	if err != nil {
		return err
	}

	db.Logger.Debug("Initialize reindex request model", zap.String("name", db.Name))
	if err := db.initReindexRequests(ctx); err != nil {
		return err
	}

	return nil
}

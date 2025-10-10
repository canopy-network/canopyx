package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/uptrace/go-clickhouse/ch"
	"go.uber.org/zap"
)

// ChainDB represents a database associated with a blockchain and provides methods to manage and query its data.
// It includes a database client, a logger for capturing logs, the chain's name, and its unique identifier.
type ChainDB struct {
	Client
	Name    string
	ChainID string
}

// InitializeDB ensures the required database and tables for indexing are created if they do not already exist.
func (db *ChainDB) InitializeDB(ctx context.Context) error {
	db.Logger.Debug("Initialize blocks model", zap.String("name", db.Name))
	if err := indexer.InitBlocks(ctx, db.Db); err != nil {
		return err
	}

	db.Logger.Debug("Initialize transactions model", zap.String("name", db.Name))
	if err := indexer.InitTransactions(ctx, db.Db, db.Name); err != nil {
		return err
	}

	return nil
}

// DatabaseName returns the ClickHouse database backing this chain.
func (db *ChainDB) DatabaseName() string {
	return db.Name
}

// ChainID returns the identifier associated with this chain store.
func (db *ChainDB) ChainKey() string {
	return db.ChainID
}

// InsertBlock persists an indexed block into the chain database.
func (db *ChainDB) InsertBlock(ctx context.Context, block *indexer.Block) error {
	_, err := db.Db.NewInsert().Model(block).Exec(ctx)
	return err
}

// InsertTransactions persists indexed transactions and raw payloads into the chain database.
func (db *ChainDB) InsertTransactions(ctx context.Context, txs []*indexer.Transaction, raws []*indexer.TransactionRaw) error {
	return indexer.InsertTransactions(ctx, db.Db, txs, raws)
}

// RawDB exposes the underlying ClickHouse connection.
func (db *ChainDB) RawDB() *ch.DB {
	return db.Db
}

// HasBlock reports whether a block exists at the specified height.
func (db *ChainDB) HasBlock(ctx context.Context, height uint64) (bool, error) {
	_, err := indexer.GetBlock(ctx, db.Db, height)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DeleteBlock removes a block record for the given height.
func (db *ChainDB) DeleteBlock(ctx context.Context, height uint64) error {
	stmt := fmt.Sprintf(`ALTER TABLE "%s"."blocks" DELETE WHERE height = ?`, db.Name)
	_, err := db.Db.ExecContext(ctx, stmt, height)
	return err
}

// DeleteTransactions removes transaction rows for the specified height from both fact and raw tables.
func (db *ChainDB) DeleteTransactions(ctx context.Context, height uint64) error {
	coreStmt := fmt.Sprintf(`ALTER TABLE "%s"."txs" DELETE WHERE height = ?`, db.Name)
	if _, err := db.Db.ExecContext(ctx, coreStmt, height); err != nil {
		return err
	}
	rawStmt := fmt.Sprintf(`ALTER TABLE "%s"."txs_raw" DELETE WHERE height = ?`, db.Name)
	if _, err := db.Db.ExecContext(ctx, rawStmt, height); err != nil {
		return err
	}
	return nil
}

// Exec executes an arbitrary query against the chain database.
func (db *ChainDB) Exec(ctx context.Context, query string, args ...any) error {
	_, err := db.Db.ExecContext(ctx, query, args...)
	return err
}

// Close terminates the underlying ClickHouse connection.
func (db *ChainDB) Close() error {
	return db.Db.Close()
}

package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
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

// QueryBlocks retrieves a paginated list of blocks ordered by height descending.
// If cursor > 0, only blocks with height < cursor are returned.
// The limit parameter controls the maximum number of rows returned (+1 for pagination detection).
func (db *ChainDB) QueryBlocks(ctx context.Context, cursor uint64, limit int) ([]indexer.BlockRow, error) {
	type rowInternal struct {
		Height          uint64    `ch:"height"`
		Hash            string    `ch:"hash"`
		Time            time.Time `ch:"time"`
		ProposerAddress string    `ch:"proposer_address"`
		NumTxs          uint32    `ch:"num_txs"`
	}

	conds := make([]string, 0)
	args := make([]any, 0)
	if cursor > 0 {
		conds = append(conds, "height < ?")
		args = append(args, cursor)
	}

	query := fmt.Sprintf(`SELECT height, hash, time, proposer_address, num_txs FROM "%s"."blocks"`, db.Name)
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += " ORDER BY height DESC LIMIT ?"
	args = append(args, limit)

	rows := make([]rowInternal, 0, limit)
	if err := db.Db.NewRaw(query, args...).Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("query blocks failed: %w", err)
	}

	// Convert to BlockRow to decouple from ClickHouse-specific tags
	result := make([]indexer.BlockRow, len(rows))
	for i, row := range rows {
		result[i] = indexer.BlockRow{
			Height:          row.Height,
			Hash:            row.Hash,
			Time:            row.Time,
			ProposerAddress: row.ProposerAddress,
			NumTxs:          row.NumTxs,
		}
	}

	return result, nil
}

// QueryTransactions retrieves a paginated list of transactions ordered by height descending.
// If cursor > 0, only transactions with height < cursor are returned.
// The limit parameter controls the maximum number of rows returned (+1 for pagination detection).
func (db *ChainDB) QueryTransactions(ctx context.Context, cursor uint64, limit int) ([]indexer.TransactionRow, error) {
	type rowInternal struct {
		Height       uint64    `ch:"height"`
		TxHash       string    `ch:"tx_hash"`
		Time         time.Time `ch:"time"`
		MessageType  string    `ch:"message_type"`
		Counterparty *string   `ch:"counterparty"`
		Signer       string    `ch:"signer"`
		Amount       *uint64   `ch:"amount"`
		Fee          uint64    `ch:"fee"`
	}

	conds := make([]string, 0)
	args := make([]any, 0)
	if cursor > 0 {
		conds = append(conds, "height < ?")
		args = append(args, cursor)
	}

	query := fmt.Sprintf(`SELECT height, tx_hash, time, message_type, counterparty, signer, amount, fee FROM "%s"."txs"`, db.Name)
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += " ORDER BY height DESC LIMIT ?"
	args = append(args, limit)

	rows := make([]rowInternal, 0, limit)
	if err := db.Db.NewRaw(query, args...).Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("query transactions failed: %w", err)
	}

	// Convert to TransactionRow to decouple from ClickHouse-specific tags
	result := make([]indexer.TransactionRow, len(rows))
	for i, row := range rows {
		result[i] = indexer.TransactionRow{
			Height:       row.Height,
			TxHash:       row.TxHash,
			Time:         row.Time,
			MessageType:  row.MessageType,
			Counterparty: row.Counterparty,
			Signer:       row.Signer,
			Amount:       row.Amount,
			Fee:          row.Fee,
		}
	}

	return result, nil
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

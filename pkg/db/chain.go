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

// Column represents a database column with its name and type information.
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

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

	db.Logger.Debug("Initialize block_summaries model", zap.String("name", db.Name))
	if err := indexer.InitBlockSummaries(ctx, db.Db); err != nil {
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

// InsertBlockSummary persists block summary data (entity counts) into the chain database.
// blockTime is the timestamp from the block, used to populate height_time for time-range queries.
func (db *ChainDB) InsertBlockSummary(ctx context.Context, height uint64, blockTime time.Time, numTxs uint32) error {
	summary := &indexer.BlockSummary{
		Height:     height,
		HeightTime: blockTime,
		NumTxs:     numTxs,
	}
	_, err := db.Db.NewInsert().Model(summary).Exec(ctx)
	return err
}

// GetBlockSummary retrieves the block summary for a given height.
func (db *ChainDB) GetBlockSummary(ctx context.Context, height uint64) (*indexer.BlockSummary, error) {
	return indexer.GetBlockSummary(ctx, db.Db, height)
}

// QueryBlocks retrieves a paginated list of blocks ordered by height.
// If sortDesc is true, orders by height DESC (newest first), otherwise ASC (oldest first).
// If cursor > 0 and sortDesc is true, only blocks with height < cursor are returned.
// If cursor > 0 and sortDesc is false, only blocks with height > cursor are returned.
// The limit parameter controls the maximum number of rows returned (+1 for pagination detection).
func (db *ChainDB) QueryBlocks(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.Block, error) {
	conds := make([]string, 0)
	args := make([]any, 0)
	if cursor > 0 {
		if sortDesc {
			conds = append(conds, "height < ?")
		} else {
			conds = append(conds, "height > ?")
		}
		args = append(args, cursor)
	}

	sortOrder := "DESC"
	if !sortDesc {
		sortOrder = "ASC"
	}

	query := fmt.Sprintf(`SELECT height, hash, parent_hash, time, proposer_address, size FROM "%s"."blocks"`, db.Name)
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY height %s LIMIT ?", sortOrder)
	args = append(args, limit)

	var blocks []indexer.Block
	if err := db.Db.NewRaw(query, args...).Scan(ctx, &blocks); err != nil {
		return nil, fmt.Errorf("query blocks failed: %w", err)
	}

	return blocks, nil
}

// QueryBlockSummaries retrieves a paginated list of block summaries ordered by height.
// If sortDesc is true, orders by height DESC (newest first), otherwise ASC (oldest first).
// If cursor > 0 and sortDesc is true, only summaries with height < cursor are returned.
// If cursor > 0 and sortDesc is false, only summaries with height > cursor are returned.
// The limit parameter controls the maximum number of rows returned (+1 for pagination detection).
func (db *ChainDB) QueryBlockSummaries(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.BlockSummary, error) {
	conds := make([]string, 0)
	args := make([]any, 0)
	if cursor > 0 {
		if sortDesc {
			conds = append(conds, "height < ?")
		} else {
			conds = append(conds, "height > ?")
		}
		args = append(args, cursor)
	}

	sortOrder := "DESC"
	if !sortDesc {
		sortOrder = "ASC"
	}

	query := fmt.Sprintf(`SELECT height, height_time, num_txs FROM "%s"."block_summaries" FINAL`, db.Name)
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY height %s LIMIT ?", sortOrder)
	args = append(args, limit)

	var rows []indexer.BlockSummary
	if err := db.Db.NewRaw(query, args...).Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("query block summaries failed: %w", err)
	}

	return rows, nil
}

// QueryTransactions retrieves a paginated list of transactions ordered by height.
// If sortDesc is true, orders by height DESC (newest first), otherwise ASC (oldest first).
// If cursor > 0 and sortDesc is true, only transactions with height < cursor are returned.
// If cursor > 0 and sortDesc is false, only transactions with height > cursor are returned.
// The limit parameter controls the maximum number of rows returned (+1 for pagination detection).
func (db *ChainDB) QueryTransactions(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.Transaction, error) {
	conds := make([]string, 0)
	args := make([]any, 0)
	if cursor > 0 {
		if sortDesc {
			conds = append(conds, "height < ?")
		} else {
			conds = append(conds, "height > ?")
		}
		args = append(args, cursor)
	}

	sortOrder := "DESC"
	if !sortDesc {
		sortOrder = "ASC"
	}

	query := fmt.Sprintf(`SELECT height, tx_hash, time, height_time, message_type, counterparty, signer, amount, fee, created_height FROM "%s"."txs"`, db.Name)
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY height %s LIMIT ?", sortOrder)
	args = append(args, limit)

	var txs []indexer.Transaction
	if err := db.Db.NewRaw(query, args...).Scan(ctx, &txs); err != nil {
		return nil, fmt.Errorf("query transactions failed: %w", err)
	}

	return txs, nil
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

// QueryTransactionsRaw retrieves raw transaction data with all columns.
// If sortDesc is true, orders by height DESC (newest first), otherwise ASC (oldest first).
// If cursor > 0 and sortDesc is true, only transactions with height < cursor are returned.
// If cursor > 0 and sortDesc is false, only transactions with height > cursor are returned.
// The limit parameter controls the maximum number of rows returned (+1 for pagination detection).
func (db *ChainDB) QueryTransactionsRaw(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]map[string]interface{}, error) {
	conds := make([]string, 0)
	args := make([]any, 0)
	if cursor > 0 {
		if sortDesc {
			conds = append(conds, "height < ?")
		} else {
			conds = append(conds, "height > ?")
		}
		args = append(args, cursor)
	}

	sortOrder := "DESC"
	if !sortDesc {
		sortOrder = "ASC"
	}

	query := fmt.Sprintf(`SELECT height, tx_hash, height_time, msg_raw, public_key, signature, created_at FROM "%s"."txs_raw"`, db.Name)
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY height %s LIMIT ?", sortOrder)
	args = append(args, limit)

	rows := make([]indexer.TransactionRaw, 0, limit)
	if err := db.Db.NewRaw(query, args...).Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("query transactions raw failed: %w", err)
	}

	// Convert to map[string]interface{} for flexible JSON response
	results := make([]map[string]interface{}, len(rows))
	for i, row := range rows {
		results[i] = map[string]interface{}{
			"height":      row.Height,
			"tx_hash":     row.TxHash,
			"height_time": row.HeightTime.UTC().Format(time.RFC3339),
			"msg_raw":     row.MsgRaw,
			"public_key":  row.PublicKey,
			"signature":   row.Signature,
			"created_at":  row.CreatedAt.UTC().Format(time.RFC3339),
		}
	}

	return results, nil
}

// DescribeTable returns column information for a table using DESCRIBE TABLE.
// Returns a slice of Column structs with name and type information.
func (db *ChainDB) DescribeTable(ctx context.Context, tableName string) ([]Column, error) {
	query := fmt.Sprintf(`DESCRIBE TABLE "%s"."%s"`, db.Name, tableName)

	rows, err := db.Db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("describe table failed: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			db.Logger.Warn("failed to close rows", zap.Error(closeErr))
		}
	}()

	var columns []Column
	for rows.Next() {
		// We only need name and type, ignore the rest of the columns
		// DESCRIBE TABLE returns: name, type, default_type, default_expression, comment, codec_expression, ttl_expression
		var col Column
		var dummy1, dummy2, dummy3, dummy4, dummy5 string

		if err := rows.Scan(&col.Name, &col.Type, &dummy1, &dummy2, &dummy3, &dummy4, &dummy5); err != nil {
			return nil, fmt.Errorf("failed to scan describe row: %w", err)
		}

		columns = append(columns, col)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return columns, nil
}

// Close terminates the underlying ClickHouse connection.
func (db *ChainDB) Close() error {
	return db.Db.Close()
}

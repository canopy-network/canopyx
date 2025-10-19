package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/entities"
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

	// Create staging tables for all entities
	db.Logger.Debug("Creating staging tables", zap.String("name", db.Name))
	if err := db.createStagingTables(ctx); err != nil {
		return fmt.Errorf("create staging tables: %w", err)
	}

	return nil
}

// createStagingTables creates staging tables for all entities defined in entities.All().
// Staging tables have the same schema as production tables but are used for temporary data
// before promotion to production.
func (db *ChainDB) createStagingTables(ctx context.Context) error {
	// Create blocks_staging table
	// Schema matches the Block model in pkg/db/models/indexer/block.go
	blocksStaging := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."blocks_staging" (
			height UInt64,
			hash String,
			time DateTime64(6),
			parent_hash String,
			proposer_address String,
			size Int32
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (height)
	`, db.Name)

	if err := db.Exec(ctx, blocksStaging); err != nil {
		return fmt.Errorf("create blocks_staging: %w", err)
	}

	// Create txs_staging table
	// Schema matches the Transaction model in pkg/db/models/indexer/tx.go
	txsStaging := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."txs_staging" (
			height UInt64,
			tx_hash String,
			time DateTime64(6),
			height_time DateTime64(6),
			message_type LowCardinality(String),
			counterparty Nullable(String),
			signer String,
			amount Nullable(UInt64),
			fee UInt64,
			created_height UInt64
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (height, tx_hash)
	`, db.Name)

	if err := db.Exec(ctx, txsStaging); err != nil {
		return fmt.Errorf("create txs_staging: %w", err)
	}

	// Create txs_raw_staging table
	// Schema matches the TransactionRaw model in pkg/db/models/indexer/tx.go
	// Note: TTL is only applied to production table, not staging
	txsRawStaging := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."txs_raw_staging" (
			height UInt64,
			tx_hash String,
			height_time DateTime64(6),
			msg_raw Nullable(String),
			public_key Nullable(String),
			signature Nullable(String),
			created_at DateTime DEFAULT now()
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (height, tx_hash)
	`, db.Name)

	if err := db.Exec(ctx, txsRawStaging); err != nil {
		return fmt.Errorf("create txs_raw_staging: %w", err)
	}

	// Create block_summaries_staging table
	// Schema matches the BlockSummary model in pkg/db/models/indexer/block_summary.go
	blockSummariesStaging := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."block_summaries_staging" (
			height UInt64,
			height_time DateTime64(6),
			num_txs UInt32 DEFAULT 0
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (height)
	`, db.Name)

	if err := db.Exec(ctx, blockSummariesStaging); err != nil {
		return fmt.Errorf("create block_summaries_staging: %w", err)
	}

	db.Logger.Info("Staging tables created successfully",
		zap.String("database", db.Name),
		zap.Strings("tables", []string{"blocks_staging", "txs_staging", "txs_raw_staging", "block_summaries_staging"}))

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

// GetBlock retrieves a single block by height from the chain database.
func (db *ChainDB) GetBlock(ctx context.Context, height uint64) (*indexer.Block, error) {
	block := new(indexer.Block)
	err := db.Db.NewSelect().
		Model(block).
		Where("height = ?", height).
		Final(). // CRITICAL: Use FINAL with ReplacingMergeTree to deduplicate
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return block, nil
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

	query := fmt.Sprintf(`SELECT height, hash, parent_hash, time, proposer_address, size FROM "%s"."blocks" FINAL`, db.Name)
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

	query := fmt.Sprintf(`SELECT height, tx_hash, time, height_time, message_type, counterparty, signer, amount, fee, created_height FROM "%s"."txs" FINAL`, db.Name)
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

	query := fmt.Sprintf(`SELECT height, tx_hash, height_time, msg_raw, public_key, signature, created_at FROM "%s"."txs_raw" FINAL`, db.Name)
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

// PromoteEntity promotes entity data from staging to production table.
// Uses entity constants for type-safe table names.
// The operation is idempotent - safe to retry if it fails.
func (db *ChainDB) PromoteEntity(ctx context.Context, entity entities.Entity, height uint64) error {
	// Validate entity is known
	if !entity.IsValid() {
		return fmt.Errorf("invalid entity: %q", entity)
	}

	// Build promotion query: INSERT INTO production SELECT * FROM staging WHERE height = ?
	query := fmt.Sprintf(
		`INSERT INTO "%s"."%s" SELECT * FROM "%s"."%s" WHERE height = ?`,
		db.Name, entity.TableName(),
		db.Name, entity.StagingTableName(),
	)

	if _, err := db.Db.ExecContext(ctx, query, height); err != nil {
		return fmt.Errorf("promote %s at height %d: %w", entity, height, err)
	}

	db.Logger.Debug("Promoted entity data",
		zap.String("entity", entity.String()),
		zap.Uint64("height", height),
		zap.String("database", db.Name))

	return nil
}

// CleanEntityStaging removes promoted data from staging table.
// Uses ALTER TABLE DELETE for efficient deletion in ClickHouse.
// The operation is idempotent - safe to retry if it fails.
func (db *ChainDB) CleanEntityStaging(ctx context.Context, entity entities.Entity, height uint64) error {
	// Validate entity is known
	if !entity.IsValid() {
		return fmt.Errorf("invalid entity: %q", entity)
	}

	// Build cleanup query: ALTER TABLE staging DELETE WHERE height = ?
	query := fmt.Sprintf(
		`ALTER TABLE "%s"."%s" DELETE WHERE height = ?`,
		db.Name, entity.StagingTableName(),
	)

	if _, err := db.Db.ExecContext(ctx, query, height); err != nil {
		// Log warning but don't fail - cleanup is non-critical
		db.Logger.Warn("Failed to clean staging data",
			zap.String("entity", entity.String()),
			zap.Uint64("height", height),
			zap.String("database", db.Name),
			zap.Error(err))
		return fmt.Errorf("clean %s staging at height %d: %w", entity, height, err)
	}

	db.Logger.Debug("Cleaned staging data",
		zap.String("entity", entity.String()),
		zap.Uint64("height", height),
		zap.String("database", db.Name))

	return nil
}

// ValidateQueryHeight validates that the requested height is fully indexed.
// Returns the validated height (or latest if nil), or error if not indexed.
// This MUST be called before any query to ensure consistency.
func (db *ChainDB) ValidateQueryHeight(ctx context.Context, requestedHeight *uint64) (uint64, error) {
	// Get the fully indexed height from index_progress
	fullyIndexed, err := db.GetFullyIndexedHeight(ctx)
	if err != nil {
		return 0, fmt.Errorf("get fully indexed height: %w", err)
	}

	// If no height specified, use latest fully indexed height
	if requestedHeight == nil {
		return fullyIndexed, nil
	}

	// Validate the requested height is not beyond what's indexed
	if *requestedHeight > fullyIndexed {
		return 0, fmt.Errorf("height %d not fully indexed (latest indexed: %d)", *requestedHeight, fullyIndexed)
	}

	return *requestedHeight, nil
}

// GetFullyIndexedHeight returns the latest fully indexed height from index_progress.
// This uses the admin database's index_progress table which tracks completed indexing.
func (db *ChainDB) GetFullyIndexedHeight(ctx context.Context) (uint64, error) {
	// We need to query the admin database's index_progress table
	// First, try the aggregate table for performance
	adminDbName := "admin" // The admin database name is always "admin"

	var height uint64
	query := fmt.Sprintf(
		`SELECT maxMerge(max_height) FROM "%s"."index_progress_agg" WHERE chain_id = ?`,
		adminDbName,
	)

	err := db.Db.NewRaw(query, db.ChainID).Scan(ctx, &height)
	if err == nil && height > 0 {
		return height, nil
	}

	// Fallback to querying the base table if aggregate is empty
	fallbackQuery := fmt.Sprintf(
		`SELECT max(height) FROM "%s"."index_progress" WHERE chain_id = ?`,
		adminDbName,
	)

	var fallbackHeight uint64
	if err := db.Db.NewRaw(fallbackQuery, db.ChainID).Scan(ctx, &fallbackHeight); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("no indexed heights found for chain %s", db.ChainID)
		}
		return 0, fmt.Errorf("query index progress: %w", err)
	}

	return fallbackHeight, nil
}

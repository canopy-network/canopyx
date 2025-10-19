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

	db.Logger.Debug("Initialize accounts model", zap.String("name", db.Name))
	if err := indexer.InitAccounts(ctx, db.Db); err != nil {
		return err
	}

	db.Logger.Debug("Initialize genesis table", zap.String("name", db.Name))
	if err := indexer.InitGenesis(ctx, db.Db, db.Name); err != nil {
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
	// Schema matches the Transaction model in pkg/db/models/indexer/tx.go (single-table design)
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
			msg String CODEC(ZSTD(3)),
			public_key Nullable(String) CODEC(ZSTD(3)),
			signature Nullable(String) CODEC(ZSTD(3)),
			created_height UInt64
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (height, tx_hash)
	`, db.Name)

	if err := db.Exec(ctx, txsStaging); err != nil {
		return fmt.Errorf("create txs_staging: %w", err)
	}

	// Create block_summaries_staging table
	// Schema matches the BlockSummary model in pkg/db/models/indexer/block_summary.go
	blockSummariesStaging := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."block_summaries_staging" (
			height UInt64,
			height_time DateTime64(6),
			num_txs UInt32 DEFAULT 0,
			tx_counts_by_type Map(LowCardinality(String), UInt32)
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (height)
	`, db.Name)

	if err := db.Exec(ctx, blockSummariesStaging); err != nil {
		return fmt.Errorf("create block_summaries_staging: %w", err)
	}

	// Create accounts_staging table
	// Schema matches the Account model in pkg/db/models/indexer/account.go
	accountsStaging := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."accounts_staging" (
			address String CODEC(ZSTD(1)),
			amount UInt64 CODEC(Delta, ZSTD(3)),
			height UInt64 CODEC(DoubleDelta, LZ4),
			height_time DateTime64(6) CODEC(DoubleDelta, LZ4),
			created_height UInt64 CODEC(DoubleDelta, LZ4)
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (address, height)
	`, db.Name)

	if err := db.Exec(ctx, accountsStaging); err != nil {
		return fmt.Errorf("create accounts_staging: %w", err)
	}

	// Create events_staging table
	// Schema matches the Event model in pkg/db/models/indexer/event.go
	eventsStaging := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."events_staging" (
			height UInt64,
			address String,
			reference String,
			event_type LowCardinality(String),
			amount Nullable(UInt64),
			sold_amount Nullable(UInt64),
			bought_amount Nullable(UInt64),
			local_amount Nullable(UInt64),
			remote_amount Nullable(UInt64),
			success Nullable(Bool),
			local_origin Nullable(Bool),
			order_id Nullable(String),
			msg String CODEC(ZSTD(3)),
			height_time DateTime64(6)
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (height, address, reference, event_type)
	`, db.Name)

	if err := db.Exec(ctx, eventsStaging); err != nil {
		return fmt.Errorf("create events_staging: %w", err)
	}

	db.Logger.Info("Staging tables created successfully",
		zap.String("database", db.Name),
		zap.Strings("tables", []string{"blocks_staging", "txs_staging", "block_summaries_staging", "accounts_staging", "events_staging"}))

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

// GetTransactionByHash retrieves a transaction by its hash, including the msg JSON field.
// Uses FINAL to deduplicate with ReplacingMergeTree.
func (db *ChainDB) GetTransactionByHash(ctx context.Context, txHash string) (*indexer.Transaction, error) {
	query := fmt.Sprintf(`SELECT height, tx_hash, time, height_time, message_type, counterparty, signer, amount, fee, msg, public_key, signature, created_height FROM "%s"."txs" FINAL WHERE tx_hash = ? LIMIT 1`, db.Name)

	var tx indexer.Transaction
	if err := db.Db.NewRaw(query, txHash).Scan(ctx, &tx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("transaction not found: %s", txHash)
		}
		return nil, fmt.Errorf("query transaction failed: %w", err)
	}

	return &tx, nil
}

// InsertTransactions persists indexed transactions into the single transactions table.
func (db *ChainDB) InsertTransactions(ctx context.Context, txs []*indexer.Transaction) error {
	if len(txs) == 0 {
		return nil
	}
	_, err := db.Db.NewInsert().Model(&txs).Exec(ctx)
	return err
}

// InitEvents creates the events table with ZSTD compression.
// This wraps the indexer package's InitEvents function.
func (db *ChainDB) InitEvents(ctx context.Context) error {
	return indexer.InitEvents(ctx, db.Db)
}

// InsertEventsStaging inserts events into the events_staging table.
// This follows the two-phase commit pattern for data consistency.
func (db *ChainDB) InsertEventsStaging(ctx context.Context, events []*indexer.Event) error {
	stagingTable := fmt.Sprintf("%s.events_staging", db.Name)
	return indexer.InsertEventsStaging(ctx, db.Db, stagingTable, events)
}

// InsertBlockSummary persists block summary data (entity counts) into the chain database.
// blockTime is the timestamp from the block, used to populate height_time for time-range queries.
// txCountsByType contains a breakdown of transaction counts by message type.
func (db *ChainDB) InsertBlockSummary(ctx context.Context, height uint64, blockTime time.Time, numTxs uint32, txCountsByType map[string]uint32) error {
	summary := &indexer.BlockSummary{
		Height:         height,
		HeightTime:     blockTime,
		NumTxs:         numTxs,
		TxCountsByType: txCountsByType,
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
	return db.QueryTransactionsWithFilter(ctx, cursor, limit, sortDesc, "")
}

// QueryTransactionsWithFilter retrieves a paginated list of transactions with optional message type filtering.
// If messageType is non-empty, only transactions matching that message type are returned.
// If sortDesc is true, orders by height DESC (newest first), otherwise ASC (oldest first).
// If cursor > 0 and sortDesc is true, only transactions with height < cursor are returned.
// If cursor > 0 and sortDesc is false, only transactions with height > cursor are returned.
// The limit parameter controls the maximum number of rows returned (+1 for pagination detection).
func (db *ChainDB) QueryTransactionsWithFilter(ctx context.Context, cursor uint64, limit int, sortDesc bool, messageType string) ([]indexer.Transaction, error) {
	conds := make([]string, 0)
	args := make([]any, 0)

	// Apply cursor filtering
	if cursor > 0 {
		if sortDesc {
			conds = append(conds, "height < ?")
		} else {
			conds = append(conds, "height > ?")
		}
		args = append(args, cursor)
	}

	// Apply message type filtering if specified
	if messageType != "" {
		conds = append(conds, "message_type = ?")
		args = append(args, messageType)
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

// QueryAccounts retrieves a paginated list of accounts ordered by height.
// If sortDesc is true, orders by height DESC (newest first), otherwise ASC (oldest first).
// If cursor > 0 and sortDesc is true, only accounts with height < cursor are returned.
// If cursor > 0 and sortDesc is false, only accounts with height > cursor are returned.
// The limit parameter controls the maximum number of rows returned (+1 for pagination detection).
func (db *ChainDB) QueryAccounts(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.Account, error) {
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

	query := fmt.Sprintf(`SELECT address, amount, height, height_time, created_height FROM "%s"."accounts" FINAL`, db.Name)
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY height %s LIMIT ?", sortOrder)
	args = append(args, limit)

	var accounts []indexer.Account
	if err := db.Db.NewRaw(query, args...).Scan(ctx, &accounts); err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
	}

	return accounts, nil
}

// QueryEvents retrieves a paginated list of events ordered by height.
// Delegates to QueryEventsWithFilter with an empty event type filter.
func (db *ChainDB) QueryEvents(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.Event, error) {
	return db.QueryEventsWithFilter(ctx, cursor, limit, sortDesc, "")
}

// QueryEventsWithFilter retrieves a paginated list of events with optional event type filtering.
// If sortDesc is true, orders by height DESC (newest first), otherwise ASC (oldest first).
// If cursor > 0 and sortDesc is true, only events with height < cursor are returned.
// If cursor > 0 and sortDesc is false, only events with height > cursor are returned.
// If eventType is non-empty, filters to only events of that type.
// The limit parameter controls the maximum number of rows returned (+1 for pagination detection).
func (db *ChainDB) QueryEventsWithFilter(ctx context.Context, cursor uint64, limit int, sortDesc bool, eventType string) ([]indexer.Event, error) {
	conds := make([]string, 0)
	args := make([]any, 0)

	// Apply cursor filtering
	if cursor > 0 {
		if sortDesc {
			conds = append(conds, "height < ?")
		} else {
			conds = append(conds, "height > ?")
		}
		args = append(args, cursor)
	}

	// Apply event type filtering if specified
	if eventType != "" {
		conds = append(conds, "event_type = ?")
		args = append(args, eventType)
	}

	sortOrder := "DESC"
	if !sortDesc {
		sortOrder = "ASC"
	}

	query := fmt.Sprintf(`SELECT height, address, reference, event_type, amount, sold_amount, bought_amount, local_amount, remote_amount, success, local_origin, order_id, height_time FROM "%s"."events" FINAL`, db.Name)
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY height %s LIMIT ?", sortOrder)
	args = append(args, limit)

	var events []indexer.Event
	if err := db.Db.NewRaw(query, args...).Scan(ctx, &events); err != nil {
		return nil, fmt.Errorf("query events failed: %w", err)
	}

	return events, nil
}

// GetAccountByAddress retrieves an account by address at a specific height.
// Uses FINAL to deduplicate with ReplacingMergeTree.
// If height is nil, returns the latest state.
// If height is specified, returns the account state at or before that height.
// Returns sql.ErrNoRows wrapped in an error if the account doesn't exist.
func (db *ChainDB) GetAccountByAddress(ctx context.Context, address string, height *uint64) (*indexer.Account, error) {
	query := fmt.Sprintf(`SELECT address, amount, height, height_time, created_height FROM "%s"."accounts" FINAL WHERE address = ?`, db.Name)
	args := []any{address}

	if height != nil {
		query += " AND height <= ? ORDER BY height DESC LIMIT 1"
		args = append(args, *height)
	} else {
		query += " ORDER BY height DESC LIMIT 1"
	}

	var account indexer.Account
	if err := db.Db.NewRaw(query, args...).Scan(ctx, &account); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("account not found: %s", address)
		}
		return nil, fmt.Errorf("query account failed: %w", err)
	}

	return &account, nil
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

// DeleteTransactions removes transaction rows for the specified height from the transactions table.
func (db *ChainDB) DeleteTransactions(ctx context.Context, height uint64) error {
	stmt := fmt.Sprintf(`ALTER TABLE "%s"."txs" DELETE WHERE height = ?`, db.Name)
	_, err := db.Db.ExecContext(ctx, stmt, height)
	return err
}

// Exec executes an arbitrary query against the chain database.
func (db *ChainDB) Exec(ctx context.Context, query string, args ...any) error {
	_, err := db.Db.ExecContext(ctx, query, args...)
	return err
}

// QueryTransactionsRaw is deprecated - use GetTransactionByHash to get msg, public_key, signature fields.
// Single-table design includes all fields in the txs table.
func (db *ChainDB) QueryTransactionsRaw(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]map[string]interface{}, error) {
	// For backward compatibility, query from txs table which now includes msg, public_key, signature
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

	query := fmt.Sprintf(`SELECT height, tx_hash, height_time, msg, public_key, signature FROM "%s"."txs" FINAL`, db.Name)
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY height %s LIMIT ?", sortOrder)
	args = append(args, limit)

	var txs []indexer.Transaction
	if err := db.Db.NewRaw(query, args...).Scan(ctx, &txs); err != nil {
		return nil, fmt.Errorf("query transactions failed: %w", err)
	}

	// Convert to map[string]interface{} for flexible JSON response
	results := make([]map[string]interface{}, len(txs))
	for i, tx := range txs {
		results[i] = map[string]interface{}{
			"height":      tx.Height,
			"tx_hash":     tx.TxHash,
			"height_time": tx.HeightTime.UTC().Format(time.RFC3339),
			"msg":         tx.Msg,
			"public_key":  tx.PublicKey,
			"signature":   tx.Signature,
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

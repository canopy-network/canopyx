package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

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
// It implements ChainStore.
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

	db.Logger.Debug("Initialize events model", zap.String("name", db.Name))
	if err := indexer.InitEvents(ctx, db.Db); err != nil {
		return err
	}

	db.Logger.Debug("Initialize pools model", zap.String("name", db.Name))
	if err := indexer.InitPools(ctx, db.Db); err != nil {
		return err
	}

	db.Logger.Debug("Initialize orders model", zap.String("name", db.Name))
	if err := indexer.InitOrders(ctx, db.Db); err != nil {
		return err
	}

	db.Logger.Debug("Initialize dex_prices model", zap.String("name", db.Name))
	if err := indexer.InitDexPrices(ctx, db.Db); err != nil {
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
	// All 25 columns including extracted queryable fields for all transaction types
	txsStaging := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."txs_staging" (
			height UInt64,
			tx_hash String,
			time DateTime64(6),
			height_time DateTime64(6),
			message_type LowCardinality(String),
			signer String,
			counterparty Nullable(String),
			amount Nullable(UInt64),
			fee UInt64,
			validator_address Nullable(String),
			commission Nullable(Float64),
			chain_id Nullable(UInt64),
			sell_amount Nullable(UInt64),
			buy_amount Nullable(UInt64),
			liquidity_amount Nullable(UInt64),
			order_id Nullable(String),
			price Nullable(Float64),
			param_key Nullable(String),
			param_value Nullable(String),
			committee_id Nullable(UInt64),
			recipient Nullable(String),
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
			chain_id UInt64,
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
		ORDER BY (height, chain_id, address, reference, event_type)
	`, db.Name)

	if err := db.Exec(ctx, eventsStaging); err != nil {
		return fmt.Errorf("create events_staging: %w", err)
	}

	// Create dex_prices_staging table
	// Schema matches the DexPrice model in pkg/db/models/indexer/dexprice.go
	dexPricesStaging := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."dex_prices_staging" (
			local_chain_id UInt64,
			remote_chain_id UInt64,
			height UInt64,
			local_pool UInt64,
			remote_pool UInt64,
			price_e6 UInt64,
			height_time DateTime64(6)
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (local_chain_id, remote_chain_id, height)
	`, db.Name)

	if err := db.Exec(ctx, dexPricesStaging); err != nil {
		return fmt.Errorf("create dex_prices_staging: %w", err)
	}

	// Create pools_staging table
	// Schema matches the Pool model in pkg/db/models/indexer/pool.go
	poolsStaging := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."pools_staging" (
			pool_id UInt64,
			height UInt64,
			chain_id UInt64,
			amount UInt64,
			total_points UInt64,
			lp_count UInt32,
			height_time DateTime64(6)
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (pool_id, height)
	`, db.Name)

	if err := db.Exec(ctx, poolsStaging); err != nil {
		return fmt.Errorf("create pools_staging: %w", err)
	}

	// Create orders_staging table
	// Schema matches the Order model in pkg/db/models/indexer/order.go
	ordersStaging := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."orders_staging" (
			order_id String,
			height UInt64,
			height_time DateTime64(6),
			committee UInt64,
			amount_for_sale UInt64,
			requested_amount UInt64,
			seller_address String,
			buyer_address Nullable(String),
			deadline UInt64,
			status LowCardinality(String),
			created_height UInt64
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (order_id, height)
	`, db.Name)

	if err := db.Exec(ctx, ordersStaging); err != nil {
		return fmt.Errorf("create orders_staging: %w", err)
	}

	db.Logger.Info("Staging tables created successfully",
		zap.String("database", db.Name),
		zap.Strings("tables", []string{"blocks_staging", "txs_staging", "block_summaries_staging", "accounts_staging", "events_staging", "dex_prices_staging", "pools_staging", "orders_staging"}))

	return nil
}

// DatabaseName returns the ClickHouse database backing this chain.
func (db *ChainDB) DatabaseName() string {
	return db.Name
}

// ChainKey returns the identifier associated with this chain store.
func (db *ChainDB) ChainKey() string {
	return db.ChainID
}

// InsertBlock persists an indexed block into the chain database.
func (db *ChainDB) InsertBlock(ctx context.Context, block *indexer.Block) error {
	query := fmt.Sprintf(`INSERT INTO "%s".blocks (height, hash, time, parent_hash, proposer_address, size) VALUES`, db.Name)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	err = batch.Append(
		block.Height,
		block.Hash,
		block.Time,
		block.LastBlockHash,
		block.ProposerAddress,
		block.Size,
	)
	if err != nil {
		return err
	}

	return batch.Send()
}

// GetBlock retrieves a single block by height from the chain database.
func (db *ChainDB) GetBlock(ctx context.Context, height uint64) (*indexer.Block, error) {
	query := fmt.Sprintf(`SELECT height, hash, time, parent_hash, proposer_address, size FROM "%s"."blocks" FINAL WHERE height = ? LIMIT 1`, db.Name)

	var block indexer.Block
	err := db.Db.QueryRow(ctx, query, height).Scan(
		&block.Height,
		&block.Hash,
		&block.Time,
		&block.LastBlockHash,
		&block.ProposerAddress,
		&block.Size,
	)
	if err != nil {
		return nil, err
	}
	return &block, nil
}

// GetTransactionByHash retrieves a transaction by its hash, including the msg JSON field.
// Uses FINAL to deduplicate with ReplacingMergeTree.
func (db *ChainDB) GetTransactionByHash(ctx context.Context, txHash string) (*indexer.Transaction, error) {
	query := fmt.Sprintf(`SELECT height, tx_hash, time, height_time, message_type, counterparty, signer, amount, fee, msg, public_key, signature, created_height FROM "%s"."txs" FINAL WHERE tx_hash = ? LIMIT 1`, db.Name)

	var tx indexer.Transaction
	err := db.Db.QueryRow(ctx, query, txHash).Scan(
		&tx.Height,
		&tx.TxHash,
		&tx.Time,
		&tx.HeightTime,
		&tx.MessageType,
		&tx.Counterparty,
		&tx.Signer,
		&tx.Amount,
		&tx.Fee,
		&tx.Msg,
		&tx.PublicKey,
		&tx.Signature,
		&tx.CreatedHeight,
	)
	if err != nil {
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

	query := fmt.Sprintf(`INSERT INTO "%s".txs (height, tx_hash, time, height_time, message_type, counterparty, signer, amount, fee, msg, public_key, signature, created_height) VALUES`, db.Name)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, tx := range txs {
		err = batch.Append(
			tx.Height,
			tx.TxHash,
			tx.Time,
			tx.HeightTime,
			tx.MessageType,
			tx.Counterparty,
			tx.Signer,
			tx.Amount,
			tx.Fee,
			tx.Msg,
			tx.PublicKey,
			tx.Signature,
			tx.CreatedHeight,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
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

// InitDexPrices creates the dex_prices table.
// This wraps the indexer package's InitDexPrices function.
func (db *ChainDB) InitDexPrices(ctx context.Context) error {
	return indexer.InitDexPrices(ctx, db.Db)
}

// InsertDexPricesStaging inserts DEX prices into the dex_prices_staging table.
// This follows the two-phase commit pattern for data consistency.
func (db *ChainDB) InsertDexPricesStaging(ctx context.Context, prices []*indexer.DexPrice) error {
	stagingTable := fmt.Sprintf("%s.dex_prices_staging", db.Name)
	return indexer.InsertDexPricesStaging(ctx, db.Db, stagingTable, prices)
}

// InitPools creates the pools table with ReplacingMergeTree engine.
// This wraps the indexer package's InitPools function.
func (db *ChainDB) InitPools(ctx context.Context) error {
	return indexer.InitPools(ctx, db.Db)
}

// InsertPoolsStaging inserts pools into the pools_staging table.
// This follows the two-phase commit pattern for data consistency.
func (db *ChainDB) InsertPoolsStaging(ctx context.Context, pools []*indexer.Pool) error {
	stagingTable := fmt.Sprintf("%s.pools_staging", db.Name)
	return indexer.InsertPoolsStaging(ctx, db.Db, stagingTable, pools)
}

// InsertBlockSummary persists block summary data (entity counts) into the chain database.
// blockTime is the timestamp from the block, used to populate height_time for time-range queries.
// txCountsByType contains a breakdown of transaction counts by message type.
func (db *ChainDB) InsertBlockSummary(ctx context.Context, height uint64, blockTime time.Time, numTxs uint32, txCountsByType map[string]uint32) error {
	query := fmt.Sprintf(`INSERT INTO "%s".block_summaries (height, height_time, num_txs, tx_counts_by_type) VALUES`, db.Name)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	err = batch.Append(height, blockTime, numTxs, txCountsByType)
	if err != nil {
		return err
	}

	return batch.Send()
}

// InsertBlocksStaging persists blocks into the blocks_staging table.
// This follows the two-phase commit pattern for data consistency.
func (db *ChainDB) InsertBlocksStaging(ctx context.Context, block *indexer.Block) error {
	query := fmt.Sprintf(`INSERT INTO "%s".blocks_staging (height, hash, time, parent_hash, proposer_address, size) VALUES`, db.Name)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	err = batch.Append(
		block.Height,
		block.Hash,
		block.Time,
		block.LastBlockHash,
		block.ProposerAddress,
		block.Size,
	)
	if err != nil {
		return err
	}

	return batch.Send()
}

// InsertTransactionsStaging persists transactions into the txs_staging table.
// This follows the two-phase commit pattern for data consistency.
// IMPORTANT: Must include all 25 columns to match the staging table schema.
func (db *ChainDB) InsertTransactionsStaging(ctx context.Context, txs []*indexer.Transaction) error {
	if len(txs) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s".txs_staging (
		height, tx_hash, time, height_time, message_type, signer, counterparty, amount, fee,
		validator_address, commission, chain_id, sell_amount, buy_amount, liquidity_amount,
		order_id, price, param_key, param_value, committee_id, recipient,
		msg, public_key, signature, created_height
	) VALUES`, db.Name)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, tx := range txs {
		err = batch.Append(
			tx.Height,
			tx.TxHash,
			tx.Time,
			tx.HeightTime,
			tx.MessageType,
			tx.Signer,
			tx.Counterparty,
			tx.Amount,
			tx.Fee,
			tx.ValidatorAddress,
			tx.Commission,
			tx.ChainID,
			tx.SellAmount,
			tx.BuyAmount,
			tx.LiquidityAmt,
			tx.OrderID,
			tx.Price,
			tx.ParamKey,
			tx.ParamValue,
			tx.CommitteeID,
			tx.Recipient,
			tx.Msg,
			tx.PublicKey,
			tx.Signature,
			tx.CreatedHeight,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

// InsertBlockSummariesStaging persists block summary data into the block_summaries_staging table.
// This follows the two-phase commit pattern for data consistency.
func (db *ChainDB) InsertBlockSummariesStaging(ctx context.Context, height uint64, blockTime time.Time, numTxs uint32, txCountsByType map[string]uint32) error {
	query := fmt.Sprintf(`INSERT INTO "%s".block_summaries_staging (height, height_time, num_txs, tx_counts_by_type) VALUES`, db.Name)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func() { _ = batch.Abort() }()

	err = batch.Append(height, blockTime, numTxs, txCountsByType)
	if err != nil {
		return err
	}

	return batch.Send()
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
	if err := db.Select(ctx, &blocks, query, args...); err != nil {
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
	if err := db.Select(ctx, &rows, query, args...); err != nil {
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
	if err := db.Select(ctx, &txs, query, args...); err != nil {
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
	if err := db.Select(ctx, &accounts, query, args...); err != nil {
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
	if err := db.Select(ctx, &events, query, args...); err != nil {
		return nil, fmt.Errorf("query events failed: %w", err)
	}

	return events, nil
}

// QueryPools retrieves a paginated list of pools ordered by height.
// If sortDesc is true, orders by height DESC (newest first), otherwise ASC (oldest first).
// If cursor > 0 and sortDesc is true, only pools with height < cursor are returned.
// If cursor > 0 and sortDesc is false, only pools with height > cursor are returned.
// The limit parameter controls the maximum number of rows returned (+1 for pagination detection).
func (db *ChainDB) QueryPools(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]indexer.Pool, error) {
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

	sortOrder := "DESC"
	if !sortDesc {
		sortOrder = "ASC"
	}

	query := fmt.Sprintf(`SELECT pool_id, height, chain_id, amount, total_points, lp_count, height_time FROM "%s"."pools" FINAL`, db.Name)
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY height %s LIMIT ?", sortOrder)
	args = append(args, limit)

	var pools []indexer.Pool
	if err := db.Select(ctx, &pools, query, args...); err != nil {
		return nil, fmt.Errorf("query pools failed: %w", err)
	}

	return pools, nil
}

// QueryOrders retrieves a paginated list of orders ordered by height with optional status filtering.
// If sortDesc is true, orders by height DESC (newest first), otherwise ASC (oldest first).
// If cursor > 0 and sortDesc is true, only orders with height < cursor are returned.
// If cursor > 0 and sortDesc is false, only orders with height > cursor are returned.
// If status is non-empty, filters to only orders with that status ("open", "filled", "cancelled", "expired").
// The limit parameter controls the maximum number of rows returned (+1 for pagination detection).
func (db *ChainDB) QueryOrders(ctx context.Context, cursor uint64, limit int, sortDesc bool, status string) ([]indexer.Order, error) {
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

	// Apply status filtering if specified
	if status != "" {
		conds = append(conds, "status = ?")
		args = append(args, status)
	}

	sortOrder := "DESC"
	if !sortDesc {
		sortOrder = "ASC"
	}

	query := fmt.Sprintf(`SELECT order_id, height, height_time, committee, amount_for_sale, requested_amount, seller_address, buyer_address, deadline, status, created_height FROM "%s"."orders" FINAL`, db.Name)
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY height %s LIMIT ?", sortOrder)
	args = append(args, limit)

	var orders []indexer.Order
	if err := db.Select(ctx, &orders, query, args...); err != nil {
		return nil, fmt.Errorf("query orders failed: %w", err)
	}

	return orders, nil
}

// QueryDexPrices retrieves a paginated list of DEX prices ordered by height with optional chain pair filtering.
// If sortDesc is true, orders by height DESC (newest first), otherwise ASC (oldest first).
// If cursor > 0 and sortDesc is true, only prices with height < cursor are returned.
// If cursor > 0 and sortDesc is false, only prices with height > cursor are returned.
// If localChainID > 0 and remoteChainID > 0, filters to only prices for that specific chain pair.
// The limit parameter controls the maximum number of rows returned (+1 for pagination detection).
func (db *ChainDB) QueryDexPrices(ctx context.Context, cursor uint64, limit int, sortDesc bool, localChainID, remoteChainID uint64) ([]indexer.DexPrice, error) {
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

	// Apply chain pair filtering if both IDs are specified
	if localChainID > 0 && remoteChainID > 0 {
		conds = append(conds, "local_chain_id = ? AND remote_chain_id = ?")
		args = append(args, localChainID, remoteChainID)
	}

	sortOrder := "DESC"
	if !sortDesc {
		sortOrder = "ASC"
	}

	query := fmt.Sprintf(`SELECT local_chain_id, remote_chain_id, height, local_pool, remote_pool, price_e6, height_time FROM "%s"."dex_prices" FINAL`, db.Name)
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY height %s LIMIT ?", sortOrder)
	args = append(args, limit)

	var prices []indexer.DexPrice
	if err := db.Select(ctx, &prices, query, args...); err != nil {
		return nil, fmt.Errorf("query dex prices failed: %w", err)
	}

	return prices, nil
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
	err := db.Db.QueryRow(ctx, query, args...).Scan(
		&account.Address,
		&account.Amount,
		&account.Height,
		&account.HeightTime,
		&account.CreatedHeight,
	)
	if err != nil {
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
	return db.Db.Exec(ctx, stmt, height)
}

// DeleteTransactions removes transaction rows for the specified height from the transactions table.
func (db *ChainDB) DeleteTransactions(ctx context.Context, height uint64) error {
	stmt := fmt.Sprintf(`ALTER TABLE "%s"."txs" DELETE WHERE height = ?`, db.Name)
	return db.Db.Exec(ctx, stmt, height)
}

// Exec executes an arbitrary query against the chain database.
func (db *ChainDB) Exec(ctx context.Context, query string, args ...any) error {
	return db.Db.Exec(ctx, query, args...)
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
	if err := db.Select(ctx, &txs, query, args...); err != nil {
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

	rows, err := db.Db.Query(ctx, query)
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

	if err := db.Db.Exec(ctx, query, height); err != nil {
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

	if err := db.Db.Exec(ctx, query, height); err != nil {
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

	err := db.Db.QueryRow(ctx, query, db.ChainID).Scan(&height)
	if err == nil && height > 0 {
		return height, nil
	}

	// Fallback to querying the base table if aggregate is empty
	fallbackQuery := fmt.Sprintf(
		`SELECT max(height) FROM "%s"."index_progress" WHERE chain_id = ?`,
		adminDbName,
	)

	var fallbackHeight uint64
	if err := db.Db.QueryRow(ctx, fallbackQuery, db.ChainID).Scan(&fallbackHeight); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("no indexed heights found for chain %s", db.ChainID)
		}
		return 0, fmt.Errorf("query index progress: %w", err)
	}

	return fallbackHeight, nil
}

// GetGenesisData retrieves the genesis data JSON for a specific height (usually 0).
// Returns the data string and an error if not found.
func (db *ChainDB) GetGenesisData(ctx context.Context, height uint64) (string, error) {
	query := fmt.Sprintf(`SELECT data FROM "%s"."genesis" WHERE height = ? LIMIT 1`, db.Name)

	var data string
	err := db.Db.QueryRow(ctx, query, height).Scan(&data)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("genesis data not found for height %d", height)
		}
		return "", fmt.Errorf("query genesis data: %w", err)
	}

	return data, nil
}

// HasGenesis checks if genesis data exists for a specific height.
func (db *ChainDB) HasGenesis(ctx context.Context, height uint64) (bool, error) {
	query := fmt.Sprintf(`SELECT count(*) FROM "%s"."genesis" WHERE height = ?`, db.Name)

	var count uint64
	err := db.Db.QueryRow(ctx, query, height).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check genesis exists: %w", err)
	}

	return count > 0, nil
}

// InsertGenesis inserts genesis data into the database.
func (db *ChainDB) InsertGenesis(ctx context.Context, height uint64, data string, fetchedAt time.Time) error {
	query := fmt.Sprintf(`INSERT INTO "%s"."genesis" (height, data, fetched_at) VALUES (?, ?, ?)`, db.Name)

	err := db.Db.Exec(ctx, query, height, data, fetchedAt)
	if err != nil {
		return fmt.Errorf("insert genesis: %w", err)
	}

	return nil
}

// InsertAccountsStaging persists staged account snapshots for the chain.
func (db *ChainDB) InsertAccountsStaging(ctx context.Context, accounts []*indexer.Account) error {
	return indexer.InsertAccountsStaging(ctx, db.Db, "accounts_staging", accounts)
}

// GetAccountCreatedHeight retrieves the created_height for an existing account.
// Uses FINAL to ensure we get the most recent version from ReplacingMergeTree.
// Returns 0 if the account is not found.
func (db *ChainDB) GetAccountCreatedHeight(ctx context.Context, address string) uint64 {
	query := fmt.Sprintf(`SELECT created_height FROM "%s"."accounts" FINAL WHERE address = ? ORDER BY height DESC LIMIT 1`, db.Name)

	var createdHeight uint64
	err := db.Db.QueryRow(ctx, query, address).Scan(&createdHeight)
	if err != nil {
		// Account not found in DB yet - return 0
		return 0
	}

	return createdHeight
}

// GetOrderCreatedHeight retrieves the created_height for an existing order.
// Uses FINAL to ensure we get the most recent version from ReplacingMergeTree.
// Returns 0 if the order is not found.
func (db *ChainDB) GetOrderCreatedHeight(ctx context.Context, orderID string) uint64 {
	query := fmt.Sprintf(`SELECT created_height FROM "%s"."orders" FINAL WHERE order_id = ? ORDER BY height DESC LIMIT 1`, db.Name)

	var createdHeight uint64
	err := db.Db.QueryRow(ctx, query, orderID).Scan(&createdHeight)
	if err != nil {
		// Order not found in DB yet - return 0
		return 0
	}

	return createdHeight
}

package chain

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initEvents creates the events table and its staging table with ZSTD compression.
// Uses ReplacingMergeTree with height as the deduplication key.
// ORDER BY (height, event_type, ...) for optimal GetEventsByTypeAndHeight queries.
func (db *DB) initEvents(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.EventColumns)

	// Production and staging both use (height, event_type) as first two columns
	// This optimizes the common query: WHERE height = ? AND event_type IN (...)
	// Used by all indexer activities that call GetEventsByTypeAndHeight
	queryTemplate := `
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (height, event_type, chain_id, address, reference)
	`

	// Create a production table
	productionQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.EventsProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.EventsProductionTableName, err)
	}

	// Create staging table
	stagingQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.EventsStagingTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.EventsStagingTableName, err)
	}

	return nil
}

// InsertEventsStaging inserts events into the events_staging table.
// This follows the two-phase commit pattern for data consistency.
func (db *DB) InsertEventsStaging(ctx context.Context, events []*indexermodels.Event) error {
	if len(events) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		height, chain_id, address, reference, event_type, block_height,
		amount, sold_amount, bought_amount, local_amount, remote_amount,
		success, local_origin, order_id, points_received, points_burned,
		data, seller_receive_address, buyer_send_address, sellers_send_address,
		msg, height_time
	) VALUES`, db.Name, indexermodels.EventsStagingTableName)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, event := range events {
		err = batch.Append(
			event.Height,
			event.ChainID,
			event.Address,
			event.Reference,
			event.EventType,
			event.BlockHeight,
			event.Amount,
			event.SoldAmount,
			event.BoughtAmount,
			event.LocalAmount,
			event.RemoteAmount,
			event.Success,
			event.LocalOrigin,
			event.OrderID,
			event.PointsReceived,
			event.PointsBurned,
			event.Data,
			event.SellerReceiveAddress,
			event.BuyerSendAddress,
			event.SellersSendAddress,
			event.Msg,
			event.HeightTime,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

// GetEventsByTypeAndHeight retrieves events at a specific height filtered by event types.
// This method queries the table to get fresh events for entity processing.
// Event types are matched using OR logic: (event_type = 'type1' OR event_type = 'type2' ...)
//
// Example usage:
//
//	events, err: = db.GetEventsByTypeAndHeight(ctx, 1000, "EventDexSwap", "EventDexLiquidityDeposit")
//
// This is used by the events-first architecture where IndexEvents stores events to staging,
// then entity-specific activities (IndexDexBatch, etc.) query relevant events to avoid
// duplicate RPC calls.
func (db *DB) GetEventsByTypeAndHeight(ctx context.Context, height uint64, staging bool, eventTypes ...string) ([]*indexermodels.Event, error) {
	if len(eventTypes) == 0 {
		return []*indexermodels.Event{}, nil
	}

	// Build OR clause for event types
	typeClause := "event_type = ?"
	if len(eventTypes) > 1 {
		typeClause = "event_type IN ("
		for i := range eventTypes {
			if i > 0 {
				typeClause += ", "
			}
			typeClause += "?"
		}
		typeClause += ")"
	}

	tableName := indexermodels.EventsStagingTableName
	finalClause := "" // No FINAL for staging - we control insert patterns
	if !staging {
		tableName = indexermodels.EventsProductionTableName
		finalClause = "FINAL"
	}

	query := fmt.Sprintf(`
		SELECT
			height, chain_id, address, reference, event_type, block_height,
			amount, sold_amount, bought_amount, local_amount, remote_amount,
			success, local_origin, order_id, points_received, points_burned,
			data, seller_receive_address, buyer_send_address, sellers_send_address,
			msg, height_time
		FROM "%s"."%s" %s
		WHERE height = ? AND %s
		ORDER BY reference, event_type
	`, db.Name, tableName, finalClause, typeClause)

	// Build args: height + eventTypes
	args := make([]interface{}, 0, len(eventTypes)+1)
	args = append(args, height)
	for _, eventType := range eventTypes {
		args = append(args, eventType)
	}

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query events by type at height %d: %w", height, err)
	}
	defer func() { _ = rows.Close() }()

	events := make([]*indexermodels.Event, 0)
	for rows.Next() {
		var event indexermodels.Event
		if err := rows.Scan(
			&event.Height,
			&event.ChainID,
			&event.Address,
			&event.Reference,
			&event.EventType,
			&event.BlockHeight,
			&event.Amount,
			&event.SoldAmount,
			&event.BoughtAmount,
			&event.LocalAmount,
			&event.RemoteAmount,
			&event.Success,
			&event.LocalOrigin,
			&event.OrderID,
			&event.PointsReceived,
			&event.PointsBurned,
			&event.Data,
			&event.SellerReceiveAddress,
			&event.BuyerSendAddress,
			&event.SellersSendAddress,
			&event.Msg,
			&event.HeightTime,
		); err != nil {
			return nil, fmt.Errorf("scan event row: %w", err)
		}
		events = append(events, &event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate event rows: %w", err)
	}

	return events, nil
}

// =============================================================================
// Lightweight Event Query Functions
// =============================================================================
// These functions return only the columns actually needed by each activity,
// following the pattern: primary key (height) + lookup key + used fields.
// This reduces data transfer by 80-95% compared to GetEventsByTypeAndHeight.

// EventRewardSlash is a lightweight struct for reward/slash events.
// Used by IndexAccounts which needs address for lookup and amount for tracking.
// Uses non-Nullable types (0 as default for amount).
type EventRewardSlash struct {
	Height    uint64 `ch:"height"`
	Address   string `ch:"address"`
	EventType string `ch:"event_type"`
	Amount    uint64 `ch:"amount"`
}

// GetRewardSlashEvents returns lightweight reward/slash events for IndexAccounts.
// Returns only 4 columns instead of 21 (~80% reduction).
// Note: FINAL is only used for production reads (deduplication needed).
// For staging reads during indexing, FINAL is skipped as we control insert patterns.
func (db *DB) GetRewardSlashEvents(ctx context.Context, height uint64, staging bool) ([]EventRewardSlash, error) {
	tableName := indexermodels.EventsStagingTableName
	finalClause := "" // No FINAL for staging - we control insert patterns
	if !staging {
		tableName = indexermodels.EventsProductionTableName
		finalClause = " FINAL"
	}

	query := fmt.Sprintf(`
		SELECT height, address, event_type, amount
		FROM "%s"."%s"%s
		WHERE height = ? AND event_type IN ('reward', 'slash')
	`, db.Name, tableName, finalClause)

	var events []EventRewardSlash
	if err := db.Select(ctx, &events, query, height); err != nil {
		return nil, fmt.Errorf("query reward/slash events at height %d: %w", height, err)
	}

	return events, nil
}

// EventLifecycle is a lightweight struct for validator lifecycle events.
// Used by IndexValidators which only needs address and event_type for state tracking.
type EventLifecycle struct {
	Height    uint64 `ch:"height"`
	Address   string `ch:"address"`
	EventType string `ch:"event_type"`
}

// GetValidatorLifecycleEvents returns lightweight lifecycle events for IndexValidators.
// Returns only 3 columns instead of 21 (~85% reduction).
// Note: FINAL is only used for production reads.
func (db *DB) GetValidatorLifecycleEvents(ctx context.Context, height uint64, staging bool) ([]EventLifecycle, error) {
	tableName := indexermodels.EventsStagingTableName
	finalClause := ""
	if !staging {
		tableName = indexermodels.EventsProductionTableName
		finalClause = " FINAL"
	}

	query := fmt.Sprintf(`
		SELECT height, address, event_type
		FROM "%s"."%s"%s
		WHERE height = ? AND event_type IN ('reward', 'slash', 'automatic-pause', 'automatic-begin-unstaking', 'automatic-finish-unstaking')
	`, db.Name, tableName, finalClause)

	var events []EventLifecycle
	if err := db.Select(ctx, &events, query, height); err != nil {
		return nil, fmt.Errorf("query validator lifecycle events at height %d: %w", height, err)
	}

	return events, nil
}

// EventWithOrderID is a lightweight struct for DEX events that need order_id matching.
// Used by IndexOrders and IndexDexBatch.
// Uses non-Nullable types (” as default for order_id).
type EventWithOrderID struct {
	Height    uint64 `ch:"height"`
	EventType string `ch:"event_type"`
	OrderID   string `ch:"order_id"`
}

// GetOrderBookSwapEvents returns lightweight order-book-swap events for IndexOrders.
// Returns only 3 columns instead of 21 (~85% reduction).
// Note: FINAL is only used for production reads.
func (db *DB) GetOrderBookSwapEvents(ctx context.Context, height uint64, staging bool) ([]EventWithOrderID, error) {
	tableName := indexermodels.EventsStagingTableName
	finalClause := "" // No FINAL for staging - we control insert patterns
	if !staging {
		tableName = indexermodels.EventsProductionTableName
		finalClause = "FINAL"
	}

	query := fmt.Sprintf(`
		SELECT height, event_type, order_id
		FROM "%s"."%s" %s
		WHERE height = ? AND event_type = 'order-book-swap'
	`, db.Name, tableName, finalClause)

	var events []EventWithOrderID
	if err := db.Select(ctx, &events, query, height); err != nil {
		return nil, fmt.Errorf("query order-book-swap events at height %d: %w", height, err)
	}

	return events, nil
}

// GetDexOrderEvents returns lightweight DEX events (swap, deposit, withdraw) for IndexDexBatch.
// Returns only 3 columns instead of 21 (~85% reduction).
// Note: FINAL is only used for production reads.
func (db *DB) GetDexOrderEvents(ctx context.Context, height uint64, staging bool) ([]EventWithOrderID, error) {
	tableName := indexermodels.EventsStagingTableName
	finalClause := "" // No FINAL for staging - we control insert patterns
	if !staging {
		tableName = indexermodels.EventsProductionTableName
		finalClause = "FINAL"
	}

	query := fmt.Sprintf(`
		SELECT height, event_type, order_id
		FROM "%s"."%s" %s
		WHERE height = ? AND event_type IN ('dex-swap', 'dex-liquidity-deposit', 'dex-liquidity-withdraw')
	`, db.Name, tableName, finalClause)

	var events []EventWithOrderID
	if err := db.Select(ctx, &events, query, height); err != nil {
		return nil, fmt.Errorf("query dex order events at height %d: %w", height, err)
	}

	return events, nil
}

// EventTypeCount holds the count of events by type.
type EventTypeCount struct {
	EventType string `ch:"event_type"`
	Count     uint64 `ch:"count"`
}

// GetEventCountsByType returns counts of events by type for IndexPools.
// Returns only counts, no event data (~95% reduction).
// Note: FINAL is only used for production reads.
func (db *DB) GetEventCountsByType(ctx context.Context, height uint64, staging bool, eventTypes ...string) (map[string]uint64, error) {
	if len(eventTypes) == 0 {
		return make(map[string]uint64), nil
	}

	tableName := indexermodels.EventsStagingTableName
	finalClause := "" // No FINAL for staging - we control insert patterns
	if !staging {
		tableName = indexermodels.EventsProductionTableName
		finalClause = "FINAL"
	}

	// Build IN clause
	typeClause := "event_type IN ("
	for i := range eventTypes {
		if i > 0 {
			typeClause += ", "
		}
		typeClause += "?"
	}
	typeClause += ")"

	query := fmt.Sprintf(`
		SELECT event_type, count() as count
		FROM "%s"."%s" %s
		WHERE height = ? AND %s
		GROUP BY event_type
	`, db.Name, tableName, finalClause, typeClause)

	// Build args: height + eventTypes
	args := make([]interface{}, 0, len(eventTypes)+1)
	args = append(args, height)
	for _, eventType := range eventTypes {
		args = append(args, eventType)
	}

	var counts []EventTypeCount
	if err := db.Select(ctx, &counts, query, args...); err != nil {
		return nil, fmt.Errorf("query event counts at height %d: %w", height, err)
	}

	result := make(map[string]uint64)
	for _, c := range counts {
		result[c.EventType] = c.Count
	}

	return result, nil
}

// EventDexBatch is a lightweight struct for DEX batch events (swap, deposit, withdrawal).
// Used by IndexDexBatch which needs order_id for lookup plus specific fields per event type.
// Contains 11 columns instead of 21 (~48% reduction).
// Uses non-Nullable types with defaults (0/false/”) for non-applicable fields.
type EventDexBatch struct {
	Height         uint64 `ch:"height"`
	EventType      string `ch:"event_type"`
	OrderID        string `ch:"order_id"`
	Success        bool   `ch:"success"`         // swap only
	SoldAmount     uint64 `ch:"sold_amount"`     // swap only
	BoughtAmount   uint64 `ch:"bought_amount"`   // swap only
	LocalOrigin    bool   `ch:"local_origin"`    // swap, deposit
	PointsReceived uint64 `ch:"points_received"` // deposit only
	LocalAmount    uint64 `ch:"local_amount"`    // withdrawal only
	RemoteAmount   uint64 `ch:"remote_amount"`   // withdrawal only
	PointsBurned   uint64 `ch:"points_burned"`   // withdrawal only
}

// GetDexBatchEvents returns lightweight DEX events for IndexDexBatch.
// Returns only 11 columns instead of 21 (~48% reduction).
// Includes: height, event_type, order_id, success, sold_amount, bought_amount,
// local_origin, points_received, local_amount, remote_amount, points_burned.
// Note: FINAL is only used for production reads.
func (db *DB) GetDexBatchEvents(ctx context.Context, height uint64, staging bool) ([]EventDexBatch, error) {
	tableName := indexermodels.EventsStagingTableName
	finalClause := "" // No FINAL for staging - we control insert patterns
	if !staging {
		tableName = indexermodels.EventsProductionTableName
		finalClause = "FINAL"
	}

	query := fmt.Sprintf(`
		SELECT height, event_type, order_id, success, sold_amount, bought_amount,
		       local_origin, points_received, local_amount, remote_amount, points_burned
		FROM "%s"."%s" %s
		WHERE height = ? AND event_type IN ('dex-swap', 'dex-liquidity-deposit', 'dex-liquidity-withdraw')
	`, db.Name, tableName, finalClause)

	var events []EventDexBatch
	if err := db.Select(ctx, &events, query, height); err != nil {
		return nil, fmt.Errorf("query dex batch events at height %d: %w", height, err)
	}

	return events, nil
}

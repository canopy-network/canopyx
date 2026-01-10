package global

import (
    "context"
    "fmt"

    "github.com/canopy-network/canopyx/pkg/db/clickhouse"
    indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initEvents creates the events table with chain_id support.
func (db *DB) initEvents(ctx context.Context) error {
    schemaSQL := indexermodels.ColumnsToGlobalCrossChainSchemaSQL(indexermodels.EventColumns)

    query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, event_type, address, reference, height)
	`, db.Name, indexermodels.EventsProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

    return db.Exec(ctx, query)
}

// InsertEvents inserts events into the events table with chain_id.
func (db *DB) InsertEvents(ctx context.Context, events []*indexermodels.Event) error {
    if len(events) == 0 {
        return nil
    }

    // chain_id is the tenant ID, event_chain_id is the semantic chain_id from the event
    query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, height, event_chain_id, address, reference, event_type, block_height,
		amount, sold_amount, bought_amount, local_amount, remote_amount,
		success, local_origin, order_id, points_received, points_burned,
		data, seller_receive_address, buyer_send_address, sellers_send_address,
		msg, height_time
	) VALUES`, db.Name, indexermodels.EventsProductionTableName)
    batch, err := db.PrepareBatch(ctx, query)
    if err != nil {
        return err
    }
    // Ensure the batch is closed, especially if not all data is sent immediately
    defer func() { _ = batch.Close() }()

    for _, event := range events {
        err = batch.Append(
            db.ChainID, // chain_id (tenant)
            event.Height,
            event.ChainID, // event_chain_id (semantic)
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
            _ = batch.Abort()
            return err
        }
    }

    return batch.Send()
}

// GetEventsByTypeAndHeight retrieves events at a specific height filtered by event types.
func (db *DB) GetEventsByTypeAndHeight(ctx context.Context, height uint64, eventTypes ...string) ([]*indexermodels.Event, error) {
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

    query := fmt.Sprintf(`
		SELECT
			height, event_chain_id, address, reference, event_type, block_height,
			amount, sold_amount, bought_amount, local_amount, remote_amount,
			success, local_origin, order_id, points_received, points_burned,
			data, seller_receive_address, buyer_send_address, sellers_send_address,
			msg, height_time
		FROM "%s"."%s" FINAL
		WHERE chain_id = ? AND height = ? AND %s
		ORDER BY reference, event_type
	`, db.Name, indexermodels.EventsProductionTableName, typeClause)

    // Build args: chain_id, height, eventTypes...
    args := make([]interface{}, 0, len(eventTypes)+2)
    args = append(args, db.ChainID)
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
func (db *DB) GetRewardSlashEvents(ctx context.Context, height uint64) ([]EventRewardSlash, error) {
    query := fmt.Sprintf(`
		SELECT height, address, event_type, amount
		FROM "%s"."%s" FINAL
        PREWHERE chain_id = ?
		WHERE height = ? AND event_type IN ('reward', 'slash')
	`, db.Name, indexermodels.EventsProductionTableName)

    var events []EventRewardSlash
    if err := db.Select(ctx, &events, query, db.ChainID, height); err != nil {
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
func (db *DB) GetValidatorLifecycleEvents(ctx context.Context, height uint64) ([]EventLifecycle, error) {
    query := fmt.Sprintf(`
		SELECT height, address, event_type
		FROM "%s"."%s" FINAL
        PREWHERE chain_id = ?
		WHERE height = ? AND event_type IN ('reward', 'slash', 'automatic-pause', 'automatic-begin-unstaking', 'automatic-finish-unstaking')
	`, db.Name, indexermodels.EventsProductionTableName)

    var events []EventLifecycle
    if err := db.Select(ctx, &events, query, db.ChainID, height); err != nil {
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
func (db *DB) GetOrderBookSwapEvents(ctx context.Context, height uint64) ([]EventWithOrderID, error) {
    // todo: replace hardcode event type using string(lib.EventTypeOrderBookSwap)
    query := fmt.Sprintf(`
		SELECT height, event_type, order_id
		FROM "%s"."%s" FINAL
        PREWHERE chain_id = ?
		WHERE height = ? AND event_type = 'order-book-swap'
	`, db.Name, indexermodels.EventsProductionTableName)

    var events []EventWithOrderID
    if err := db.Select(ctx, &events, query, db.ChainID, height); err != nil {
        return nil, fmt.Errorf("query order-book-swap events at height %d: %w", height, err)
    }

    return events, nil
}

// GetDexOrderEvents returns lightweight DEX events for IndexDexBatch.
func (db *DB) GetDexOrderEvents(ctx context.Context, height uint64) ([]EventWithOrderID, error) {
    query := fmt.Sprintf(`
		SELECT height, event_type, order_id
		FROM "%s"."%s" FINAL
        PREWHERE chain_id = ?
		WHERE height = ? AND event_type IN ('dex-swap', 'dex-liquidity-deposit', 'dex-liquidity-withdraw')
	`, db.Name, indexermodels.EventsProductionTableName)

    var events []EventWithOrderID
    if err := db.Select(ctx, &events, query, db.ChainID, height); err != nil {
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
func (db *DB) GetEventCountsByType(ctx context.Context, height uint64, eventTypes ...string) (map[string]uint64, error) {
    if len(eventTypes) == 0 {
        return make(map[string]uint64), nil
    }

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
		FROM "%s"."%s" FINAL
        PREWHERE chain_id = ?
		WHERE height = ? AND %s
		GROUP BY event_type
	`, db.Name, indexermodels.EventsProductionTableName, typeClause)

    args := make([]interface{}, 0, len(eventTypes)+2)
    args = append(args, db.ChainID)
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
func (db *DB) GetDexBatchEvents(ctx context.Context, height uint64) ([]EventDexBatch, error) {
    query := fmt.Sprintf(`
		SELECT height, event_type, order_id, success, sold_amount, bought_amount,
		       local_origin, points_received, local_amount, remote_amount, points_burned
		FROM "%s"."%s" FINAL
        PREWHERE chain_id = ?
		WHERE height = ? AND event_type IN ('dex-swap', 'dex-liquidity-deposit', 'dex-liquidity-withdraw')
	`, db.Name, indexermodels.EventsProductionTableName)

    var events []EventDexBatch
    if err := db.Select(ctx, &events, query, db.ChainID, height); err != nil {
        return nil, fmt.Errorf("query dex batch events at height %d: %w", height, err)
    }

    return events, nil
}

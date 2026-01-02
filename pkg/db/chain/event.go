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
    if !staging {
        tableName = indexermodels.EventsProductionTableName
    }

    query := fmt.Sprintf(`
		SELECT
			height, chain_id, address, reference, event_type, block_height,
			amount, sold_amount, bought_amount, local_amount, remote_amount,
			success, local_origin, order_id, points_received, points_burned,
			data, seller_receive_address, buyer_send_address, sellers_send_address,
			msg, height_time
		FROM "%s"."%s" FINAL
		WHERE height = ? AND %s
		ORDER BY reference, event_type
	`, db.Name, tableName, typeClause)

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

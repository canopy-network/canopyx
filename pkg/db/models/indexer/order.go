package indexer

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Order represents a versioned snapshot of an order's state.
// Snapshots are created ONLY when order state changes (not every block).
// This enables temporal queries like "What was order X's state at height 5000?"
// while using significantly less storage than storing all orders at every height.
//
// The snapshot-on-change pattern works correctly with parallel/unordered indexing because:
// - We compare RPC(height H) vs RPC(height H-1) to detect changes
// - We don't rely on database state which may be incomplete during parallel indexing
// - ReplacingMergeTree handles deduplication if the same height is indexed multiple times
type Order struct {
	// Identity - OrderID is the unique identifier for this order
	OrderID string `ch:"order_id"` // Unique order identifier

	// Version tracking - every state change creates a new snapshot
	Height     uint64    `ch:"height"`      // Height at which this snapshot was created
	HeightTime time.Time `ch:"height_time"` // Block timestamp for time-range queries

	// Order Details
	Committee       uint64  `ch:"committee"`        // Committee ID for the order
	AmountForSale   uint64  `ch:"amount_for_sale"`  // Amount being sold
	RequestedAmount uint64  `ch:"requested_amount"` // Amount requested in return
	SellerAddress   string  `ch:"seller_address"`   // Address of the seller
	BuyerAddress    *string `ch:"buyer_address"`    // Address of the buyer (null if not filled)
	Deadline        *uint64 `ch:"deadline"`         // Order deadline height (null if no deadline)

	// Status tracking (LowCardinality for efficient filtering)
	// Possible values: "open", "filled", "cancelled", "expired"
	Status string `ch:"status"` // LowCardinality(String) for efficient filtering

	// Lifecycle tracking
	// For new orders: created_height = current height
	// For existing orders: preserved from previous snapshot
	CreatedHeight uint64 `ch:"created_height"` // Height when order was first created
}

// InitOrders initializes the orders table and its staging table.
// The production table uses aggressive compression for storage optimization.
// The staging table has the same schema but no TTL (TTL only on production).
func InitOrders(ctx context.Context, db driver.Conn) error {
	query := `
		CREATE TABLE IF NOT EXISTS orders (
			order_id String CODEC(ZSTD(1)),
			height UInt64 CODEC(DoubleDelta, LZ4),
			height_time DateTime64(6) CODEC(DoubleDelta, LZ4),
			committee UInt64 CODEC(Delta, ZSTD(3)),
			amount_for_sale UInt64 CODEC(Delta, ZSTD(3)),
			requested_amount UInt64 CODEC(Delta, ZSTD(3)),
			seller_address String CODEC(ZSTD(1)),
			buyer_address Nullable(String) CODEC(ZSTD(1)),
			deadline Nullable(UInt64) CODEC(Delta, ZSTD(3)),
			status LowCardinality(String),
			created_height UInt64 CODEC(DoubleDelta, LZ4)
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (order_id, height)
	`
	return db.Exec(ctx, query)
}

// InsertOrdersStaging inserts order snapshots to the staging table.
// Staging tables are used for new data before promotion to production.
// This follows the two-phase commit pattern for data consistency.
func InsertOrdersStaging(ctx context.Context, db driver.Conn, tableName string, orders []*Order) error {
	if len(orders) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO %s (order_id, height, height_time, committee, amount_for_sale, requested_amount, seller_address, buyer_address, deadline, status, created_height) VALUES`, tableName)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, order := range orders {
		err = batch.Append(
			order.OrderID,
			order.Height,
			order.HeightTime,
			order.Committee,
			order.AmountForSale,
			order.RequestedAmount,
			order.SellerAddress,
			order.BuyerAddress,
			order.Deadline,
			order.Status,
			order.CreatedHeight,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

// InsertOrdersProduction inserts order snapshots directly to the production table.
// This is used when bypassing the staging pattern (e.g., for batch imports).
func InsertOrdersProduction(ctx context.Context, db driver.Conn, orders []*Order) error {
	if len(orders) == 0 {
		return nil
	}

	query := `INSERT INTO orders (order_id, height, height_time, committee, amount_for_sale, requested_amount, seller_address, buyer_address, deadline, status, created_height) VALUES`
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, order := range orders {
		err = batch.Append(
			order.OrderID,
			order.Height,
			order.HeightTime,
			order.Committee,
			order.AmountForSale,
			order.RequestedAmount,
			order.SellerAddress,
			order.BuyerAddress,
			order.Deadline,
			order.Status,
			order.CreatedHeight,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

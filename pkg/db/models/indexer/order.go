package indexer

import (
	"time"
)

const OrdersProductionTableName = "orders"
const OrdersStagingTableName = "orders_staging"

// Order represents a versioned snapshot of an order's state.
// Snapshots are created ONLY when order state changes (not every block).
// This enables temporal queries like "What was order X's state at height 5000?"
// while using significantly less storage than storing all orders at every height.
//
// The snapshot-on-change pattern works correctly with parallel/unordered indexing because:
// - We compare RPC(height H) vs RPC(height H-1) to detect changes
// - We don't rely on database state which may be incomplete during parallel indexing
// - ReplacingMergeTree handles deduplication if the same height is indexed multiple times
//
// Note: Order creation time is tracked via the order_created_height materialized view,
// which calculates MIN(height) for each order_id. Consumers should JOIN with this view
// if they need to know when an order was created.
type Order struct {
	// Identity - OrderID is the unique identifier for this order
	OrderID string `ch:"order_id" json:"order_id"` // Unique order identifier

	// Version tracking - every state change creates a new snapshot
	Height     uint64    `ch:"height" json:"height"`           // Height at which this snapshot was created
	HeightTime time.Time `ch:"height_time" json:"height_time"` // Block timestamp for time-range queries

	// Order Details
	Committee       uint64  `ch:"committee" json:"committee"`               // Committee ID for the order
	AmountForSale   uint64  `ch:"amount_for_sale" json:"amount_for_sale"`   // Amount being sold
	RequestedAmount uint64  `ch:"requested_amount" json:"requested_amount"` // Amount requested in return
	SellerAddress   string  `ch:"seller_address" json:"seller_address"`     // Address of the seller
	BuyerAddress    *string `ch:"buyer_address" json:"buyer_address"`       // Address of the buyer (null if not filled)
	Deadline        *uint64 `ch:"deadline" json:"deadline"`                 // Order deadline height (null if no deadline)

	// Status tracking (LowCardinality for efficient filtering)
	// Possible values: "open", "filled", "cancelled", "expired"
	Status string `ch:"status" json:"status"` // LowCardinality(String) for efficient filtering
}

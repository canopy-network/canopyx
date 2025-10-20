package db

import (
	"context"
	"fmt"
)

// DexVolumeStats represents 24-hour trading volume statistics for a committee (chain pair).
type DexVolumeStats struct {
	Committee       uint64  `json:"committee" ch:"committee"`
	TotalOrders     uint64  `json:"total_orders" ch:"total_orders"`
	VolumeForSale   uint64  `json:"volume_for_sale" ch:"volume_for_sale"`
	VolumeRequested uint64  `json:"volume_requested" ch:"volume_requested"`
	LastPrice       *uint64 `json:"last_price,omitempty" ch:"last_price"`
}

// OrderBookLevel represents aggregated order book depth at a price level.
type OrderBookLevel struct {
	Committee      uint64 `json:"committee" ch:"committee"`
	PriceE6        uint64 `json:"price_e6" ch:"price_e6"`
	TotalForSale   uint64 `json:"total_for_sale" ch:"total_for_sale"`
	TotalRequested uint64 `json:"total_requested" ch:"total_requested"`
	OrderCount     uint32 `json:"order_count" ch:"order_count"`
}

// GetDexVolume24h returns 24-hour trading volume statistics grouped by committee.
// Only includes orders with status='filled' within the last 24 hours.
func (db *ChainDB) GetDexVolume24h(ctx context.Context) ([]DexVolumeStats, error) {
	query := fmt.Sprintf(`
		SELECT
			committee,
			COUNT(*) as total_orders,
			SUM(amount_for_sale) as volume_for_sale,
			SUM(requested_amount) as volume_requested,
			argMax((requested_amount * 1000000) / amount_for_sale, height) as last_price
		FROM "%s"."orders" FINAL
		WHERE status = 'filled'
		  AND height_time >= NOW() - INTERVAL 24 HOUR
		GROUP BY committee
		ORDER BY volume_for_sale DESC
	`, db.Name)

	var stats []DexVolumeStats
	if err := db.Select(ctx, &stats, query); err != nil {
		return nil, fmt.Errorf("query dex volume 24h failed: %w", err)
	}

	return stats, nil
}

// GetOrderBookDepth returns aggregated order book depth at price levels for a specific committee.
// Only includes orders with status='open'. Groups by price buckets (rounded to nearest 100).
func (db *ChainDB) GetOrderBookDepth(ctx context.Context, committee uint64, limit int) ([]OrderBookLevel, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query := fmt.Sprintf(`
		SELECT
			committee,
			round((requested_amount * 1000000) / amount_for_sale, -2) as price_e6,
			SUM(amount_for_sale) as total_for_sale,
			SUM(requested_amount) as total_requested,
			COUNT(*) as order_count
		FROM "%s"."orders" FINAL
		WHERE committee = ?
		  AND status = 'open'
		  AND amount_for_sale > 0
		GROUP BY committee, price_e6
		ORDER BY price_e6 DESC
		LIMIT ?
	`, db.Name)

	var levels []OrderBookLevel
	if err := db.Select(ctx, &levels, query, committee, limit); err != nil {
		return nil, fmt.Errorf("query order book depth failed: %w", err)
	}

	return levels, nil
}

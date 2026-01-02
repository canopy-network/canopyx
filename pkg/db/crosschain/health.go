package crosschain

import (
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

// HealthStatus represents the overall health of the cross-chain sync system.
type HealthStatus struct {
	DatabaseName      string         `json:"database_name"`
	TotalChainsSynced int            `json:"total_chains_synced"`
	MaxLag            int64          `json:"max_lag"`
	TablesHealthy     int            `json:"tables_healthy"`
	TablesUnhealthy   int            `json:"tables_unhealthy"`
	Details           []HealthDetail `json:"details,omitempty"`
	CheckedAt         time.Time      `json:"checked_at"`
}

// HealthDetail provides per-chain health information.
type HealthDetail struct {
	ChainID       uint64 `json:"chain_id"`
	TablesHealthy int    `json:"tables_healthy"`
	TablesWithLag int    `json:"tables_with_lag"`
	MaxLag        int64  `json:"max_lag"`
}

// GetHealthStatus returns overall health status of the cross-chain sync system (base on a accounts table as sample)
// Checks all chains that have data in global tables.
func (db *DB) GetHealthStatus(ctx context.Context) (*HealthStatus, error) {
	health := &HealthStatus{
		DatabaseName: db.Name,
		CheckedAt:    time.Now(),
	}

	// Get list of chains with data in global tables
	chainsQuery := fmt.Sprintf(`
		SELECT DISTINCT chain_id
		FROM "%s"."%s"
		ORDER BY chain_id
	`, db.Name, db.getGlobalTableName(indexer.AccountsProductionTableName))

	rows, err := db.Query(ctx, chainsQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query chains: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var chainIDs []uint64
	for rows.Next() {
		var chainID uint64
		if err := rows.Scan(&chainID); err != nil {
			db.Logger.Warn("Failed to scan chain ID", zap.Error(err))
			continue
		}
		chainIDs = append(chainIDs, chainID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	health.TotalChainsSynced = len(chainIDs)

	// Check health for each chain
	configs := GetTableConfigs()
	for _, chainID := range chainIDs {
		detail := HealthDetail{ChainID: chainID}

		for _, cfg := range configs {
			status, err := db.GetSyncStatus(ctx, chainID, cfg.TableName)
			if err != nil {
				db.Logger.Warn("Failed to get sync status",
					zap.Uint64("chain_id", chainID),
					zap.String("table", cfg.TableName),
					zap.Error(err))
				continue
			}

			// Check if the table is healthy (lag <= 10 rows or 0.1% of the total)
			isHealthy := status.Lag >= 0 && (status.Lag <= 10 || status.Lag <= int64(status.SourceRowCount/1000))
			if isHealthy {
				detail.TablesHealthy++
				health.TablesHealthy++
			} else {
				detail.TablesWithLag++
				health.TablesUnhealthy++
			}

			// Track max lag
			if status.Lag > detail.MaxLag {
				detail.MaxLag = status.Lag
			}
			if status.Lag > health.MaxLag {
				health.MaxLag = status.Lag
			}
		}

		health.Details = append(health.Details, detail)
	}

	return health, nil
}

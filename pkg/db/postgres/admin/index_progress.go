package admin

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/postgres"
	adminmodels "github.com/canopy-network/canopyx/pkg/db/models/admin"
)

// initIndexProgress creates the index_progress table
// This table tracks the last indexed height per chain (single row per chain)
func (db *DB) initIndexProgress(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS index_progress (
			chain_id BIGINT PRIMARY KEY,
			last_height BIGINT NOT NULL DEFAULT 0,
			last_indexed_at TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		)
	`

	return db.Exec(ctx, query)
}

// RecordIndexed records that a height was successfully indexed
// Uses GREATEST to ensure we never go backwards in height
func (db *DB) RecordIndexed(ctx context.Context, chainID uint64, height uint64, indexingTimeMs float64, indexingDetail string) error {
	query := `
		INSERT INTO index_progress (chain_id, last_height, last_indexed_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (chain_id) DO UPDATE SET
			last_height = GREATEST(index_progress.last_height, EXCLUDED.last_height),
			last_indexed_at = NOW(),
			updated_at = NOW()
	`

	return db.Exec(ctx, query, chainID, height)
}

// LastIndexed returns the highest indexed height for a chain
func (db *DB) LastIndexed(ctx context.Context, chainID uint64) (uint64, error) {
	query := `
		SELECT COALESCE(last_height, 0)
		FROM index_progress
		WHERE chain_id = $1
	`

	var height uint64
	err := db.QueryRow(ctx, query, chainID).Scan(&height)
	if err != nil {
		if postgres.IsNoRows(err) {
			return 0, nil // Chain not yet indexed
		}
		return 0, fmt.Errorf("failed to query last indexed height: %w", err)
	}

	return height, nil
}

// FindGaps finds missing height ranges in the indexing progress
// NOTE: Postgres admin store only tracks last_height per chain, not individual heights
// To find gaps, query the chain database directly (blocks table)
// This method returns empty for Postgres implementation
func (db *DB) FindGaps(ctx context.Context, chainID uint64) ([]adminmodels.Gap, error) {
	// TODO: Implement by querying chain database blocks table
	return []adminmodels.Gap{}, nil
}

// GetCleanableHeights returns heights that can be cleaned from staging (older than lookbackHours)
// NOTE: Postgres implementation doesn't use staging tables, so this returns empty
func (db *DB) GetCleanableHeights(ctx context.Context, chainID uint64, lookbackHours int) ([]uint64, error) {
	// Postgres doesn't use staging pattern, return empty
	return []uint64{}, nil
}

// IndexProgressHistory returns index progress metrics grouped by time intervals
// NOTE: Postgres admin store doesn't track individual height history
// Returns empty for now - could be implemented with a separate history table if needed
func (db *DB) IndexProgressHistory(ctx context.Context, chainID uint64, hours, intervalMinutes int) ([]adminmodels.ProgressPoint, error) {
	// TODO: Implement with separate history table if needed
	return []adminmodels.ProgressPoint{}, nil
}

// DeleteIndexProgressForChain deletes all index progress records for a chain
func (db *DB) DeleteIndexProgressForChain(ctx context.Context, chainID uint64) error {
	query := `DELETE FROM index_progress WHERE chain_id = $1`
	return db.Exec(ctx, query, chainID)
}

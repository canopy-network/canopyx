package chain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// initGenesis creates the genesis table for caching full genesis state.
// The genesis table stores the full genesis JSON at height 0, which is used
// by the accounts indexer when processing height 1 (comparing RPC(1) vs Genesis(0)).
func (db *DB) initGenesis(ctx context.Context) error {
	ddl := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."genesis" (
			height UInt64,
			data String,
			fetched_at DateTime DEFAULT now()
		) ENGINE = ReplacingMergeTree()
		ORDER BY height
	`, db.Name)

	err := db.Db.Exec(ctx, ddl)
	if err != nil {
		return fmt.Errorf("create genesis table: %w", err)
	}

	return nil
}

// GetGenesisData retrieves the genesis data JSON for a specific height (usually 0).
// Returns the data string and an error if not found.
func (db *DB) GetGenesisData(ctx context.Context, height uint64) (string, error) {
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
func (db *DB) HasGenesis(ctx context.Context, height uint64) (bool, error) {
	query := fmt.Sprintf(`SELECT count(*) FROM "%s"."genesis" WHERE height = ?`, db.Name)

	var count uint64
	err := db.Db.QueryRow(ctx, query, height).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check genesis exists: %w", err)
	}

	return count > 0, nil
}

// InsertGenesis inserts genesis data into the database.
func (db *DB) InsertGenesis(ctx context.Context, height uint64, data string, fetchedAt time.Time) error {
	query := fmt.Sprintf(`INSERT INTO "%s"."genesis" (height, data, fetched_at) VALUES (?, ?, ?)`, db.Name)

	err := db.Db.Exec(ctx, query, height, data, fetchedAt)
	if err != nil {
		return fmt.Errorf("insert genesis: %w", err)
	}

	return nil
}

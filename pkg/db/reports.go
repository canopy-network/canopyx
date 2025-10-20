package db

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/models/reports"
)

// ReportsDB represents a database connection for storing and managing reports across chains.
// It includes a database client, a logger for capturing logs, and the database name.
type ReportsDB struct {
	Client
	Name string
}

// DatabaseName returns the reports database name.
func (db *ReportsDB) DatabaseName() string {
	return db.Name
}

// Close terminates the underlying ClickHouse connection.
func (db *ReportsDB) Close() error {
	return db.Db.Close()
}

// InitializeDB creates the reports tables using raw SQL.
func (db *ReportsDB) InitializeDB(ctx context.Context) error {
	// 1) chain_tx_hourly
	query1 := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."chain_tx_hourly" (
			chain_id String,
			hour DateTime,
			count UInt64,
			version UInt64
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY (chain_id, hour)
	`, db.Name)
	if err := db.Db.Exec(ctx, query1); err != nil {
		return err
	}

	// 2) chain_tx_daily
	query2 := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."chain_tx_daily" (
			chain_id String,
			day Date,
			count UInt64,
			version UInt64
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY (chain_id, day)
	`, db.Name)
	if err := db.Db.Exec(ctx, query2); err != nil {
		return err
	}

	// 3) chain_tx_24h
	query3 := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."chain_tx_24h" (
			chain_id String,
			asof DateTime,
			count UInt64,
			version UInt64
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY (chain_id)
	`, db.Name)
	if err := db.Db.Exec(ctx, query3); err != nil {
		return err
	}

	return nil
}

// GetChainTxHourly returns the last N hourly transaction buckets for a chain.
func (db *ReportsDB) GetChainTxHourly(ctx context.Context, chainID string, limit int) ([]reports.ChainTxHourly, error) {
	query := `
		SELECT hour, count
		FROM chain_tx_hourly
		WHERE chain_id = ?
		ORDER BY hour DESC
		LIMIT ?
	`

	var rows []reports.ChainTxHourly
	if err := db.Select(ctx, &rows, query, chainID, limit); err != nil {
		return nil, fmt.Errorf("get chain tx hourly: %w", err)
	}

	return rows, nil
}

// GetChainTxDaily returns the last N daily transaction buckets for a chain.
func (db *ReportsDB) GetChainTxDaily(ctx context.Context, chainID string, limit int) ([]reports.ChainTxDaily, error) {
	query := `
		SELECT day, count
		FROM chain_tx_daily
		WHERE chain_id = ?
		ORDER BY day DESC
		LIMIT ?
	`

	var rows []reports.ChainTxDaily
	if err := db.Select(ctx, &rows, query, chainID, limit); err != nil {
		return nil, fmt.Errorf("get chain tx daily: %w", err)
	}

	return rows, nil
}

// GetChainTx24h returns the most recent 24h snapshot for a chain.
func (db *ReportsDB) GetChainTx24h(ctx context.Context, chainID string) ([]reports.ChainTx24h, error) {
	query := `
		SELECT asof, count
		FROM chain_tx_24hs
		WHERE chain_id = ?
		ORDER BY asof DESC
		LIMIT 1
	`

	var rows []reports.ChainTx24h
	if err := db.Select(ctx, &rows, query, chainID); err != nil {
		return nil, fmt.Errorf("get chain tx 24h: %w", err)
	}

	return rows, nil
}

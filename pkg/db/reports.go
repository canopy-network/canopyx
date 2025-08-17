package db

import (
	"context"

	"github.com/canopy-network/canopyx/pkg/db/models/reports"
)

// ReportsDB represents a database connection for storing and managing reports across chains.
// It includes a database client, a logger for capturing logs, and the database name.
type ReportsDB struct {
	Client
	Name string
}

// InitializeDB TODO: implement this
func (db *ReportsDB) InitializeDB(ctx context.Context) error {
	// 1) chain_tx_hourly
	if _, err := db.Db.NewCreateTable().
		Model((*reports.ChainTxHourly)(nil)).
		IfNotExists().
		Engine("ReplacingMergeTree(version)").
		Order("chain_id, hour").Exec(ctx); err != nil {
		return err
	}

	// 2) chain_tx_daily
	if _, err := db.Db.NewCreateTable().
		Model((*reports.ChainTxDaily)(nil)).
		IfNotExists().
		Engine("ReplacingMergeTree(version)").
		Order("chain_id, day").Exec(ctx); err != nil {
		return err
	}

	// 3) chain_tx_24h
	if _, err := db.Db.NewCreateTable().
		Model((*reports.ChainTx24h)(nil)).
		IfNotExists().
		Engine("ReplacingMergeTree(version)").
		Order("chain_id").Exec(ctx); err != nil {
		return err
	}

	return nil
}

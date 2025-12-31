package chain

import (
	"context"
	"fmt"
	"github.com/canopy-network/canopyx/pkg/db/clickhouse"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"go.uber.org/zap"
)

// initAccounts initializes the account table and its staging table.
// The production table uses aggressive compression for storage optimization.
// The staging table has the same schema but no TTL (TTL only on production).
func (db *DB) initAccounts(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.AccountColumns)

	// Create a production table
	productionQuery := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS "%s"."%s" %s (
			%s
		) ENGINE = %s
		ORDER BY (address, height)
	`, db.Name, indexermodels.AccountsProductionTableName, db.OnCluster(), schemaSQL, db.Engine(indexermodels.AccountsProductionTableName, clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.AccountsProductionTableName, err)
	}

	// Create the staging table
	// IMPORTANT: ORDER BY (height, address) - height FIRST for efficient cleanup/promotion
	// Staging tables are always queried by height: DELETE/INSERT SELECT WHERE height = ?
	stagingQuery := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS "%s"."%s" %s (
			%s
		) ENGINE = %s
		ORDER BY (height, address)
	`, db.Name, indexermodels.AccountsStagingTableName, db.OnCluster(), schemaSQL, db.Engine(indexermodels.AccountsStagingTableName, clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.AccountsStagingTableName, err)
	}

	return nil
}

// InsertAccountsStaging inserts account snapshots to the staging table.
// Staging tables are used for new data before promotion to production.
// This follows the two-phase commit pattern for data consistency.
func (db *DB) InsertAccountsStaging(ctx context.Context, accounts []*indexermodels.Account) error {
	if len(accounts) == 0 {
		return nil
	}

	query := fmt.Sprintf(
		`INSERT INTO "%s"."%s" (address, amount, rewards, slashes, height, height_time) VALUES`,
		db.Name, indexermodels.AccountsStagingTableName,
	)
	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, account := range accounts {
		err = batch.Append(
			account.Address,
			account.Amount,
			account.Rewards,
			account.Slashes,
			account.Height,
			account.HeightTime,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

// initAccountCreatedHeightView creates a materialized view to calculate the minimum height
// at which each account was created. This replaces the removed created_height column.
//
// The materialized view automatically updates as new data is inserted into the accounts table,
// providing an efficient way to query account creation heights without storing the value
// in every account snapshot row.
//
// Query usage: SELECT address, created_height FROM account_created_height WHERE address = ?
func (db *DB) initAccountCreatedHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."account_created_height" %s
		ENGINE = %s
		ORDER BY address
		AS SELECT
			address,
			min(height) as created_height
		FROM "%s"."accounts"
		GROUP BY address
	`, db.Name, db.OnCluster(), db.Engine("account_created_height", "AggregatingMergeTree", ""), db.Name)

	db.Logger.Debug("Creating materialized view", zap.String("view", "account_created_height"), zap.String("query", query))
	return db.Exec(ctx, query)
}

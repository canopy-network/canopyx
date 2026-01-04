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
        CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (address, height)
	`, db.Name, indexermodels.AccountsProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.AccountsProductionTableName, err)
	}

	// Create the staging table
	// IMPORTANT: ORDER BY (height, address) - height FIRST for efficient cleanup/promotion
	// Staging tables are always queried by height: DELETE/INSERT SELECT WHERE height = ?
	stagingQuery := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (height, address)
	`, db.Name, indexermodels.AccountsStagingTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
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
// Design: Uses SimpleAggregateFunction via minSimpleState() for optimal performance:
// - Storage: 8 bytes per row (vs 24 bytes with AggregateFunction)
// - Memory: Lower footprint during aggregation - state equals final value
// - Query: Direct access without -Merge suffix (just SELECT created_height)
// - Merge: Proper combining of values when parts merge
//
// Memory Considerations:
// When bulk data is promoted to the accounts table (e.g., during initial sync or reindex),
// this MV triggers GROUP BY aggregation that can consume significant memory.
// The ClickHouse server is configured with max_bytes_before_external_group_by
// to enable disk spill and prevent OOM errors during large bulk inserts.
// See: deploy/k8s/clickhouse/base/installation.yaml
//
// See: https://kb.altinity.com/altinity-kb-queries-and-syntax/simplestateif-or-ifstate-for-simple-aggregate-functions/
// See: https://clickhouse.com/docs/examples/aggregate-function-combinators/minSimpleState
//
// Query usage: SELECT address, created_height FROM account_created_height WHERE address = ?
func (db *DB) initAccountCreatedHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."account_created_height"
		ENGINE = %s
		ORDER BY address
		AS SELECT
			address,
			minSimpleState(height) as created_height
		FROM "%s"."accounts"
		GROUP BY address
	`, db.Name, clickhouse.ReplicatedEngine(clickhouse.AggregatingMergeTree, ""), db.Name)

	db.Logger.Debug("Creating materialized view", zap.String("view", "account_created_height"), zap.String("query", query))
	return db.Exec(ctx, query)
}

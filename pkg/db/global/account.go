package global

import (
    "context"
    "fmt"

    "github.com/canopy-network/canopyx/pkg/db/clickhouse"
    indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initAccounts initializes the account table for the global database.
// Uses chain_id as the first column in ORDER BY for optimal multi-tenant queries.
func (db *DB) initAccounts(ctx context.Context) error {
    schemaSQL := indexermodels.ColumnsToGlobalSchemaSQL(indexermodels.AccountColumns)

    // Production table: ORDER BY (chain_id, address, height) - entity lookups-
    query := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, address, height)
	`, db.Name, indexermodels.AccountsProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

    return db.Exec(ctx, query)
}

// InsertAccounts inserts account snapshots to the table with chain_id context.
func (db *DB) InsertAccounts(ctx context.Context, accounts []*indexermodels.Account) error {
    if len(accounts) == 0 {
        return nil
    }

    // chain_id is first column
    query := fmt.Sprintf(
        `INSERT INTO "%s"."%s" (chain_id, address, amount, rewards, slashes, height, height_time) VALUES`,
        db.Name, indexermodels.AccountsProductionTableName,
    )
    batch, err := db.PrepareBatch(ctx, query)
    if err != nil {
        return err
    }
    // Ensure the batch is closed, especially if not all data is sent immediately
    defer func() { _ = batch.Close() }()

    for _, account := range accounts {
        err = batch.Append(
            db.ChainID, // chain_id from DB context
            account.Address,
            account.Amount,
            account.Rewards,
            account.Slashes,
            account.Height,
            account.HeightTime,
        )
        if err != nil {
            _ = batch.Abort()
            return err
        }
    }

    return batch.Send()
}

// initAccountCreatedHeightView creates a materialized view with chain_id grouping.
func (db *DB) initAccountCreatedHeightView(ctx context.Context) error {
    query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."account_created_height"
		ENGINE = %s
		ORDER BY (chain_id, address)
		AS SELECT
			chain_id,
			address,
			minSimpleState(height) as created_height
		FROM "%s"."accounts"
		GROUP BY chain_id, address
	`, db.Name, clickhouse.ReplicatedEngine(clickhouse.AggregatingMergeTree, ""), db.Name)

    return db.Exec(ctx, query)
}

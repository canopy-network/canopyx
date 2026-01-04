package chain

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initCommittees initializes the committees table and staging table.
// Uses ReplacingMergeTree with height as the deduplication key.
// All numeric columns use Delta+ZSTD compression for optimal storage.
// Boolean columns are stored as UInt8 (0=false, 1=true).
func (db *DB) initCommittees(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.CommitteeColumns)

	// Production table: ORDER BY (chain_id, height) for efficient chain_id lookups
	productionQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, height)
	`, db.Name, indexermodels.CommitteeProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.CommitteeProductionTableName, err)
	}

	// Staging table: ORDER BY (height, chain_id) for efficient cleanup/promotion WHERE height = ?
	stagingQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (height, chain_id)
	`, db.Name, indexermodels.CommitteeStagingTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.CommitteeStagingTableName, err)
	}

	return nil
}

// initCommitteeCreatedHeightView creates a materialized view to track the minimum height (created_height) for each committee.
// This provides an efficient way to determine when each committee was first seen without scanning the entire committees table.
//
// The materialized view automatically updates as new data is inserted into the committees table,
// maintaining the MIN(height) for each chain_id.
//
// Query usage: SELECT chain_id, created_height FROM committee_created_height WHERE chain_id = ?
func (db *DB) initCommitteeCreatedHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."committee_created_height"
		ENGINE = %s
		ORDER BY chain_id
		AS SELECT
			chain_id,
			minSimpleState(height) as created_height,
			maxSimpleState(height_time) as height_time
		FROM "%s"."committees"
		GROUP BY chain_id
	`, db.Name, clickhouse.ReplicatedEngine(clickhouse.AggregatingMergeTree, ""), db.Name)

	return db.Exec(ctx, query)
}

// InsertCommitteesStaging persists committees into the committees_staging table.
// This follows the two-phase commit pattern for data consistency.
// Committees are only inserted when their data differs from the previous height.
func (db *DB) InsertCommitteesStaging(ctx context.Context, committees []*indexermodels.Committee) error {
	if len(committees) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, last_root_height_updated, last_chain_height_updated,
		number_of_samples, subsidized, retired, height, height_time
	) VALUES`, db.Name, indexermodels.CommitteeStagingTableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, committee := range committees {
		err = batch.Append(
			committee.ChainID,
			committee.LastRootHeightUpdated,
			committee.LastChainHeightUpdated,
			committee.NumberOfSamples,
			committee.Subsidized,
			committee.Retired,
			committee.Height,
			committee.HeightTime,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

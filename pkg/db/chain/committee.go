package chain

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initCommittees initializes the committees table and staging table.
// Uses ReplacingMergeTree with height as the deduplication key.
// All numeric columns use Delta+ZSTD compression for optimal storage.
// Boolean columns are stored as UInt8 (0=false, 1=true).
func (db *DB) initCommittees(ctx context.Context) error {
	queryTemplate := `
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			chain_id UInt64 CODEC(Delta, ZSTD(3)),
			last_root_height_updated UInt64 CODEC(Delta, ZSTD(3)),
			last_chain_height_updated UInt64 CODEC(Delta, ZSTD(3)),
			number_of_samples UInt64 CODEC(Delta, ZSTD(3)),
			subsidized UInt8,
			retired UInt8,
			height UInt64 CODEC(Delta, ZSTD(3)),
			height_time DateTime64(6)
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (chain_id, height)
	`

	// Create production table
	productionQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.CommitteeProductionTableName)
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.CommitteeProductionTableName, err)
	}

	// Create staging table
	stagingQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.CommitteeStagingTableName)
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
		ENGINE = AggregatingMergeTree()
		ORDER BY chain_id
		AS SELECT
			chain_id,
			min(height) as created_height,
			max(height_time) as height_time
		FROM "%s"."committees"
		GROUP BY chain_id
	`, db.Name, db.Name)

	return db.Exec(ctx, query)
}

// InsertCommitteesStaging persists committees into the committees_staging table.
// This follows the two-phase commit pattern for data consistency.
// Committees are only inserted when their data differs from the previous height.
func (db *DB) InsertCommitteesStaging(ctx context.Context, committees []*indexermodels.Committee) error {
	if len(committees) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s".committees_staging (
		chain_id, last_root_height_updated, last_chain_height_updated,
		number_of_samples, subsidized, retired, height, height_time
	) VALUES`, db.Name)

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

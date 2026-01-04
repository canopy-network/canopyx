package chain

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initCommitteePayments creates the committee_payments and committee_payments_staging tables.
// Uses ReplacingMergeTree engine with height as the version key.
// Stores payment distribution percentages for committee members.
//
// Compression strategy:
// - ZSTD for string fields (address)
// - ZSTD for uint64 fields (committee_id, percent, height)
// - DoubleDelta + ZSTD for DateTime64 (timestamps are monotonic)
func (db *DB) initCommitteePayments(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.CommitteePaymentColumns)

	// Production table: ORDER BY (committee_id, address, height) for efficient committee_id+address lookups
	productionQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (committee_id, address, height)
	`, db.Name, indexermodels.CommitteePaymentsProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.CommitteePaymentsProductionTableName, err)
	}

	// Staging table: ORDER BY (height, committee_id, address) for efficient cleanup/promotion WHERE height = ?
	stagingTableName := indexermodels.CommitteePaymentsProductionTableName + "_staging"
	stagingQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (height, committee_id, address)
	`, db.Name, stagingTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", stagingTableName, err)
	}

	return nil
}

// InsertCommitteePaymentsStaging inserts committee payment records to the staging table.
// Uses two-phase commit pattern: insert to staging, then promote to production after block is fully indexed.
func (db *DB) InsertCommitteePaymentsStaging(ctx context.Context, payments []*indexermodels.CommitteePayment) error {
	if len(payments) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		committee_id, address, percent, height, height_time
	) VALUES`, db.Name, indexermodels.CommitteePaymentsStagingTableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	for _, payment := range payments {
		err = batch.Append(
			payment.CommitteeID,
			payment.Address,
			payment.Percent,
			payment.Height,
			payment.HeightTime,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

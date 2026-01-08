package chain

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initParams initializes the params table and staging table.
// Uses ReplacingMergeTree with height as the deduplication key.
// All numeric columns use Delta+ZSTD compression for optimal storage of sequential values.
// The protocol_version string column uses ZSTD compression.
func (db *DB) initParams(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.ParamsColumns)

	queryTemplate := `
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (height)
	`

	// Create production table
	productionQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.ParamsProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.ParamsProductionTableName, err)
	}

	// Create staging table
	stagingQuery := fmt.Sprintf(queryTemplate, db.Name, indexermodels.ParamsStagingTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))
	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.ParamsStagingTableName, err)
	}

	return nil
}

// initParamsChangeHeightView creates a materialized view to track all heights where params changed.
// This provides an efficient way to query historical param changes without scanning the entire params table.
//
// The materialized view automatically updates as new data is inserted into the params table,
// maintaining a list of all heights where governance parameter changes occurred.
//
// Query usage: SELECT height FROM params_change_height ORDER BY height
func (db *DB) initParamsChangeHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."params_change_height"
		ENGINE = %s
		ORDER BY height
		AS SELECT
			height,
			maxSimpleState(height_time) as height_time
		FROM "%s"."params"
		GROUP BY height
	`, db.Name, clickhouse.ReplicatedEngine(clickhouse.AggregatingMergeTree, ""), db.Name)

	return db.Exec(ctx, query)
}

// InsertParamsStaging persists params into the params_staging table.
// This follows the two-phase commit pattern for data consistency.
// Params are only inserted when they differ from the previous height.
func (db *DB) InsertParamsStaging(ctx context.Context, params *indexermodels.Params) error {
	return db.insertParams(ctx, indexermodels.ParamsStagingTableName, params)
}

// InsertParamsProduction persists params into the params production table.
func (db *DB) InsertParamsProduction(ctx context.Context, params *indexermodels.Params) error {
	return db.insertParams(ctx, indexermodels.ParamsProductionTableName, params)
}

func (db *DB) insertParams(ctx context.Context, tableName string, params *indexermodels.Params) error {
	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		height, height_time,
		block_size, protocol_version, root_chain_id, retired,
		unstaking_blocks, max_pause_blocks, double_sign_slash_percentage,
		non_sign_slash_percentage, max_non_sign, non_sign_window,
		max_committees, max_committee_size, early_withdrawal_penalty,
		delegate_unstaking_blocks, minimum_order_size, stake_percent_for_subsidized,
		max_slash_per_committee, delegate_reward_percentage, buy_deadline_blocks,
		lock_order_fee_multiplier, minimum_stake_for_validators, minimum_stake_for_delegates,
		maximum_delegates_per_committee,
		send_fee, stake_fee, edit_stake_fee, unstake_fee,
		pause_fee, unpause_fee, change_parameter_fee, dao_transfer_fee,
		certificate_results_fee, subsidy_fee, create_order_fee, edit_order_fee,
		delete_order_fee, dex_limit_order_fee, dex_liquidity_deposit_fee,
		dex_liquidity_withdraw_fee,
		dao_reward_percentage
	) VALUES`, db.Name, tableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func(batch driver.Batch) {
		_ = batch.Abort()
	}(batch)

	err = batch.Append(
		params.Height, params.HeightTime,
		params.BlockSize, params.ProtocolVersion, params.RootChainID, params.Retired,
		params.UnstakingBlocks, params.MaxPauseBlocks, params.DoubleSignSlashPercentage,
		params.NonSignSlashPercentage, params.MaxNonSign, params.NonSignWindow,
		params.MaxCommittees, params.MaxCommitteeSize, params.EarlyWithdrawalPenalty,
		params.DelegateUnstakingBlocks, params.MinimumOrderSize, params.StakePercentForSubsidized,
		params.MaxSlashPerCommittee, params.DelegateRewardPercentage, params.BuyDeadlineBlocks,
		params.LockOrderFeeMultiplier, params.MinimumStakeForValidators, params.MinimumStakeForDelegates,
		params.MaximumDelegatesPerCommittee,
		params.SendFee, params.StakeFee, params.EditStakeFee, params.UnstakeFee,
		params.PauseFee, params.UnpauseFee, params.ChangeParameterFee, params.DaoTransferFee,
		params.CertificateResultsFee, params.SubsidyFee, params.CreateOrderFee, params.EditOrderFee,
		params.DeleteOrderFee, params.DexLimitOrderFee, params.DexLiquidityDepositFee,
		params.DexLiquidityWithdrawFee,
		params.DaoRewardPercentage,
	)
	if err != nil {
		return err
	}

	return batch.Send()
}

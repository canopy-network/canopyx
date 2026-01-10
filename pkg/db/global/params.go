package global

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

func (db *DB) initParams(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToGlobalSchemaSQL(indexermodels.ParamsColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, height)
	`, db.Name, indexermodels.ParamsProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	return db.Exec(ctx, query)
}

// InsertParams inserts params into the params table with chain_id.
func (db *DB) InsertParams(ctx context.Context, params *indexermodels.Params) error {
	if params == nil {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO "%s"."%s" (
		chain_id, height, height_time,
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
	) VALUES`, db.Name, indexermodels.ParamsProductionTableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	// Ensure the batch is closed, especially if not all data is sent immediately
	defer func() { _ = batch.Close() }()

	err = batch.Append(
		db.ChainID,
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
		_ = batch.Abort()
		return err
	}

	return batch.Send()
}

func (db *DB) initParamsChangeHeightView(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE MATERIALIZED VIEW IF NOT EXISTS "%s"."params_change_height"
		ENGINE = %s
		ORDER BY (chain_id, height)
		AS SELECT
			chain_id,
			height,
			maxSimpleState(height_time) as height_time
		FROM "%s"."params"
		GROUP BY chain_id, height
	`, db.Name, clickhouse.ReplicatedEngine(clickhouse.AggregatingMergeTree, ""), db.Name)

	return db.Exec(ctx, query)
}
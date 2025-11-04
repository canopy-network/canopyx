package chain

import (
	"context"
	"fmt"

	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initBlockSummaries initializes the block_summaries table and its staging table.
// This table tracks all 16 indexed entities with comprehensive field coverage (90+ fields).
func (db *DB) initBlockSummaries(ctx context.Context) error {
	// Common column definitions for both production and staging tables
	// Using Delta codec for counter fields (efficient for incrementing values)
	// Using ZSTD for boolean fields
	columnsDefinition := `
		height UInt64,
		height_time DateTime64(6),

		-- Transactions (24 fields)
		num_txs UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_send UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_delegate UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_undelegate UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_stake UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_unstake UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_edit_stake UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_vote UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_proposal UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_contract UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_system UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_unknown UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_pause UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_unpause UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_change_parameter UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_dao_transfer UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_certificate_results UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_subsidy UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_create_order UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_edit_order UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_delete_order UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_dex_limit_order UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_dex_liquidity_deposit UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_txs_dex_liquidity_withdraw UInt32 DEFAULT 0 CODEC(Delta, ZSTD),

		-- Accounts (2 fields)
		num_accounts UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_accounts_new UInt32 DEFAULT 0 CODEC(Delta, ZSTD),

		-- Events (10 fields)
		num_events UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_events_reward UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_events_slash UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_events_dex_liquidity_deposit UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_events_dex_liquidity_withdraw UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_events_dex_swap UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_events_order_book_swap UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_events_automatic_pause UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_events_automatic_begin_unstaking UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_events_automatic_finish_unstaking UInt32 DEFAULT 0 CODEC(Delta, ZSTD),

		-- Orders (6 fields)
		num_orders UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_orders_new UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_orders_open UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_orders_filled UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_orders_cancelled UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_orders_expired UInt32 DEFAULT 0 CODEC(Delta, ZSTD),

		-- PoolsByHeight (2 fields)
		num_pools UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_pools_new UInt32 DEFAULT 0 CODEC(Delta, ZSTD),

		-- DexPricesByHeight (1 field)
		num_dex_prices UInt32 DEFAULT 0 CODEC(Delta, ZSTD),

		-- DexOrders (6 fields)
		num_dex_orders UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_dex_orders_future UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_dex_orders_locked UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_dex_orders_complete UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_dex_orders_success UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_dex_orders_failed UInt32 DEFAULT 0 CODEC(Delta, ZSTD),

		-- DexDeposits (3 fields)
		num_dex_deposits UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_dex_deposits_pending UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_dex_deposits_complete UInt32 DEFAULT 0 CODEC(Delta, ZSTD),

		-- DexWithdrawals (3 fields)
		num_dex_withdrawals UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_dex_withdrawals_pending UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_dex_withdrawals_complete UInt32 DEFAULT 0 CODEC(Delta, ZSTD),

		-- DexPoolPointsByHolder (2 fields)
		num_dex_pool_points_holders UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_dex_pool_points_holders_new UInt32 DEFAULT 0 CODEC(Delta, ZSTD),

		-- Params (1 field)
		params_changed Bool DEFAULT false CODEC(ZSTD),

		-- Validators (5 fields)
		num_validators UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_validators_new UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_validators_active UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_validators_paused UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_validators_unstaking UInt32 DEFAULT 0 CODEC(Delta, ZSTD),

		-- ValidatorSigningInfo (2 fields)
		num_validator_signing_info UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_validator_signing_info_new UInt32 DEFAULT 0 CODEC(Delta, ZSTD),

		-- Committees (4 fields)
		num_committees UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_committees_new UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_committees_subsidized UInt32 DEFAULT 0 CODEC(Delta, ZSTD),
		num_committees_retired UInt32 DEFAULT 0 CODEC(Delta, ZSTD),

		-- CommitteeValidators (1 field)
		num_committee_validators UInt32 DEFAULT 0 CODEC(Delta, ZSTD),

		-- PollSnapshots (1 field)
		num_poll_snapshots UInt32 DEFAULT 0 CODEC(Delta, ZSTD)
	`

	// Create production table with ReplacingMergeTree
	productionQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (height)
	`, db.Name, indexermodels.BlockSummariesProductionTableName, columnsDefinition)

	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.BlockSummariesProductionTableName, err)
	}

	// Create staging table with MergeTree
	stagingQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (height)
	`, db.Name, indexermodels.BlockSummariesStagingTableName, columnsDefinition)

	if err := db.Exec(ctx, stagingQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.BlockSummariesStagingTableName, err)
	}

	return nil
}

// InsertBlockSummariesStaging persists block summary data into the block_summaries_staging table.
// This follows the two-phase commit pattern for data consistency.
// All 16 entities are tracked with comprehensive field coverage.
func (db *DB) InsertBlockSummariesStaging(ctx context.Context, summary *indexermodels.BlockSummary) error {
	query := fmt.Sprintf(`
		INSERT INTO "%s".block_summaries_staging (
			height,
			height_time,
			num_txs,
			num_txs_send,
			num_txs_delegate,
			num_txs_undelegate,
			num_txs_stake,
			num_txs_unstake,
			num_txs_edit_stake,
			num_txs_vote,
			num_txs_proposal,
			num_txs_contract,
			num_txs_system,
			num_txs_unknown,
			num_txs_pause,
			num_txs_unpause,
			num_txs_change_parameter,
			num_txs_dao_transfer,
			num_txs_certificate_results,
			num_txs_subsidy,
			num_txs_create_order,
			num_txs_edit_order,
			num_txs_delete_order,
			num_txs_dex_limit_order,
			num_txs_dex_liquidity_deposit,
			num_txs_dex_liquidity_withdraw,
			num_accounts,
			num_accounts_new,
			num_events,
			num_events_reward,
			num_events_slash,
			num_events_dex_liquidity_deposit,
			num_events_dex_liquidity_withdraw,
			num_events_dex_swap,
			num_events_order_book_swap,
			num_events_automatic_pause,
			num_events_automatic_begin_unstaking,
			num_events_automatic_finish_unstaking,
			num_orders,
			num_orders_new,
			num_orders_open,
			num_orders_filled,
			num_orders_cancelled,
			num_orders_expired,
			num_pools,
			num_pools_new,
			num_dex_prices,
			num_dex_orders,
			num_dex_orders_future,
			num_dex_orders_locked,
			num_dex_orders_complete,
			num_dex_orders_success,
			num_dex_orders_failed,
			num_dex_deposits,
			num_dex_deposits_pending,
			num_dex_deposits_complete,
			num_dex_withdrawals,
			num_dex_withdrawals_pending,
			num_dex_withdrawals_complete,
			num_dex_pool_points_holders,
			num_dex_pool_points_holders_new,
			params_changed,
			num_validators,
			num_validators_new,
			num_validators_active,
			num_validators_paused,
			num_validators_unstaking,
			num_validator_signing_info,
			num_validator_signing_info_new,
			num_committees,
			num_committees_new,
			num_committees_subsidized,
			num_committees_retired,
			num_committee_validators,
			num_poll_snapshots
		) VALUES`, db.Name)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	defer func() { _ = batch.Abort() }()

	err = batch.Append(
		summary.Height,
		summary.HeightTime,
		summary.NumTxs,
		summary.NumTxsSend,
		summary.NumTxsDelegate,
		summary.NumTxsUndelegate,
		summary.NumTxsStake,
		summary.NumTxsUnstake,
		summary.NumTxsEditStake,
		summary.NumTxsVote,
		summary.NumTxsProposal,
		summary.NumTxsContract,
		summary.NumTxsSystem,
		summary.NumTxsUnknown,
		summary.NumTxsPause,
		summary.NumTxsUnpause,
		summary.NumTxsChangeParameter,
		summary.NumTxsDaoTransfer,
		summary.NumTxsCertificateResults,
		summary.NumTxsSubsidy,
		summary.NumTxsCreateOrder,
		summary.NumTxsEditOrder,
		summary.NumTxsDeleteOrder,
		summary.NumTxsDexLimitOrder,
		summary.NumTxsDexLiquidityDeposit,
		summary.NumTxsDexLiquidityWithdraw,
		summary.NumAccounts,
		summary.NumAccountsNew,
		summary.NumEvents,
		summary.NumEventsReward,
		summary.NumEventsSlash,
		summary.NumEventsDexLiquidityDeposit,
		summary.NumEventsDexLiquidityWithdraw,
		summary.NumEventsDexSwap,
		summary.NumEventsOrderBookSwap,
		summary.NumEventsAutomaticPause,
		summary.NumEventsAutomaticBeginUnstaking,
		summary.NumEventsAutomaticFinishUnstaking,
		summary.NumOrders,
		summary.NumOrdersNew,
		summary.NumOrdersOpen,
		summary.NumOrdersFilled,
		summary.NumOrdersCancelled,
		summary.NumOrdersExpired,
		summary.NumPools,
		summary.NumPoolsNew,
		summary.NumDexPrices,
		summary.NumDexOrders,
		summary.NumDexOrdersFuture,
		summary.NumDexOrdersLocked,
		summary.NumDexOrdersComplete,
		summary.NumDexOrdersSuccess,
		summary.NumDexOrdersFailed,
		summary.NumDexDeposits,
		summary.NumDexDepositsPending,
		summary.NumDexDepositsComplete,
		summary.NumDexWithdrawals,
		summary.NumDexWithdrawalsPending,
		summary.NumDexWithdrawalsComplete,
		summary.NumDexPoolPointsHolders,
		summary.NumDexPoolPointsHoldersNew,
		summary.ParamsChanged,
		summary.NumValidators,
		summary.NumValidatorsNew,
		summary.NumValidatorsActive,
		summary.NumValidatorsPaused,
		summary.NumValidatorsUnstaking,
		summary.NumValidatorSigningInfo,
		summary.NumValidatorSigningInfoNew,
		summary.NumCommittees,
		summary.NumCommitteesNew,
		summary.NumCommitteesSubsidized,
		summary.NumCommitteesRetired,
		summary.NumCommitteeValidators,
		summary.NumPollSnapshots,
	)
	if err != nil {
		return err
	}

	return batch.Send()
}

// GetBlockSummary retrieves a block summary by height from the production table.
func (db *DB) GetBlockSummary(ctx context.Context, height uint64) (*indexermodels.BlockSummary, error) {
	var bs indexermodels.BlockSummary

	query := `
		SELECT
			height,
			height_time,
			num_txs,
			num_txs_send,
			num_txs_delegate,
			num_txs_undelegate,
			num_txs_stake,
			num_txs_unstake,
			num_txs_edit_stake,
			num_txs_vote,
			num_txs_proposal,
			num_txs_contract,
			num_txs_system,
			num_txs_unknown,
			num_txs_pause,
			num_txs_unpause,
			num_txs_change_parameter,
			num_txs_dao_transfer,
			num_txs_certificate_results,
			num_txs_subsidy,
			num_txs_create_order,
			num_txs_edit_order,
			num_txs_delete_order,
			num_txs_dex_limit_order,
			num_txs_dex_liquidity_deposit,
			num_txs_dex_liquidity_withdraw,
			num_accounts,
			num_accounts_new,
			num_events,
			num_events_reward,
			num_events_slash,
			num_events_dex_liquidity_deposit,
			num_events_dex_liquidity_withdraw,
			num_events_dex_swap,
			num_events_order_book_swap,
			num_events_automatic_pause,
			num_events_automatic_begin_unstaking,
			num_events_automatic_finish_unstaking,
			num_orders,
			num_orders_new,
			num_orders_open,
			num_orders_filled,
			num_orders_cancelled,
			num_orders_expired,
			num_pools,
			num_pools_new,
			num_dex_prices,
			num_dex_orders,
			num_dex_orders_future,
			num_dex_orders_locked,
			num_dex_orders_complete,
			num_dex_orders_success,
			num_dex_orders_failed,
			num_dex_deposits,
			num_dex_deposits_pending,
			num_dex_deposits_complete,
			num_dex_withdrawals,
			num_dex_withdrawals_pending,
			num_dex_withdrawals_complete,
			num_dex_pool_points_holders,
			num_dex_pool_points_holders_new,
			params_changed,
			num_validators,
			num_validators_new,
			num_validators_active,
			num_validators_paused,
			num_validators_unstaking,
			num_validator_signing_info,
			num_validator_signing_info_new,
			num_committees,
			num_committees_new,
			num_committees_subsidized,
			num_committees_retired,
			num_committee_validators,
			num_poll_snapshots
		FROM block_summaries FINAL
		WHERE height = ?
		LIMIT 1
	`

	err := db.Db.QueryRow(ctx, query, height).Scan(
		&bs.Height,
		&bs.HeightTime,
		&bs.NumTxs,
		&bs.NumTxsSend,
		&bs.NumTxsDelegate,
		&bs.NumTxsUndelegate,
		&bs.NumTxsStake,
		&bs.NumTxsUnstake,
		&bs.NumTxsEditStake,
		&bs.NumTxsVote,
		&bs.NumTxsProposal,
		&bs.NumTxsContract,
		&bs.NumTxsSystem,
		&bs.NumTxsUnknown,
		&bs.NumTxsPause,
		&bs.NumTxsUnpause,
		&bs.NumTxsChangeParameter,
		&bs.NumTxsDaoTransfer,
		&bs.NumTxsCertificateResults,
		&bs.NumTxsSubsidy,
		&bs.NumTxsCreateOrder,
		&bs.NumTxsEditOrder,
		&bs.NumTxsDeleteOrder,
		&bs.NumTxsDexLimitOrder,
		&bs.NumTxsDexLiquidityDeposit,
		&bs.NumTxsDexLiquidityWithdraw,
		&bs.NumAccounts,
		&bs.NumAccountsNew,
		&bs.NumEvents,
		&bs.NumEventsReward,
		&bs.NumEventsSlash,
		&bs.NumEventsDexLiquidityDeposit,
		&bs.NumEventsDexLiquidityWithdraw,
		&bs.NumEventsDexSwap,
		&bs.NumEventsOrderBookSwap,
		&bs.NumEventsAutomaticPause,
		&bs.NumEventsAutomaticBeginUnstaking,
		&bs.NumEventsAutomaticFinishUnstaking,
		&bs.NumOrders,
		&bs.NumOrdersNew,
		&bs.NumOrdersOpen,
		&bs.NumOrdersFilled,
		&bs.NumOrdersCancelled,
		&bs.NumOrdersExpired,
		&bs.NumPools,
		&bs.NumPoolsNew,
		&bs.NumDexPrices,
		&bs.NumDexOrders,
		&bs.NumDexOrdersFuture,
		&bs.NumDexOrdersLocked,
		&bs.NumDexOrdersComplete,
		&bs.NumDexOrdersSuccess,
		&bs.NumDexOrdersFailed,
		&bs.NumDexDeposits,
		&bs.NumDexDepositsPending,
		&bs.NumDexDepositsComplete,
		&bs.NumDexWithdrawals,
		&bs.NumDexWithdrawalsPending,
		&bs.NumDexWithdrawalsComplete,
		&bs.NumDexPoolPointsHolders,
		&bs.NumDexPoolPointsHoldersNew,
		&bs.ParamsChanged,
		&bs.NumValidators,
		&bs.NumValidatorsNew,
		&bs.NumValidatorsActive,
		&bs.NumValidatorsPaused,
		&bs.NumValidatorsUnstaking,
		&bs.NumValidatorSigningInfo,
		&bs.NumValidatorSigningInfoNew,
		&bs.NumCommittees,
		&bs.NumCommitteesNew,
		&bs.NumCommitteesSubsidized,
		&bs.NumCommitteesRetired,
		&bs.NumCommitteeValidators,
		&bs.NumPollSnapshots,
	)

	return &bs, err
}

// QueryBlockSummaries retrieves a paginated list of block summaries ordered by height.
// If sortDesc is true, orders by height DESC (newest first), otherwise ASC (oldest first).
// If cursor > 0 and sortDesc is true, only summaries with height < cursor are returned.
// If cursor > 0 and sortDesc is false, only summaries with height > cursor are returned.
// The limit parameter controls the maximum number of rows returned (+1 for pagination detection).
func (db *DB) QueryBlockSummaries(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]*indexermodels.BlockSummary, error) {
	query := `
		SELECT
			height,
			height_time,
			num_txs,
			num_txs_send,
			num_txs_delegate,
			num_txs_undelegate,
			num_txs_stake,
			num_txs_unstake,
			num_txs_edit_stake,
			num_txs_vote,
			num_txs_proposal,
			num_txs_contract,
			num_txs_system,
			num_txs_unknown,
			num_txs_pause,
			num_txs_unpause,
			num_txs_change_parameter,
			num_txs_dao_transfer,
			num_txs_certificate_results,
			num_txs_subsidy,
			num_txs_create_order,
			num_txs_edit_order,
			num_txs_delete_order,
			num_txs_dex_limit_order,
			num_txs_dex_liquidity_deposit,
			num_txs_dex_liquidity_withdraw,
			num_accounts,
			num_accounts_new,
			num_events,
			num_events_reward,
			num_events_slash,
			num_events_dex_liquidity_deposit,
			num_events_dex_liquidity_withdraw,
			num_events_dex_swap,
			num_events_order_book_swap,
			num_events_automatic_pause,
			num_events_automatic_begin_unstaking,
			num_events_automatic_finish_unstaking,
			num_orders,
			num_orders_new,
			num_orders_open,
			num_orders_filled,
			num_orders_cancelled,
			num_orders_expired,
			num_pools,
			num_pools_new,
			num_dex_prices,
			num_dex_orders,
			num_dex_orders_future,
			num_dex_orders_locked,
			num_dex_orders_complete,
			num_dex_orders_success,
			num_dex_orders_failed,
			num_dex_deposits,
			num_dex_deposits_pending,
			num_dex_deposits_complete,
			num_dex_withdrawals,
			num_dex_withdrawals_pending,
			num_dex_withdrawals_complete,
			num_dex_pool_points_holders,
			num_dex_pool_points_holders_new,
			params_changed,
			num_validators,
			num_validators_new,
			num_validators_active,
			num_validators_paused,
			num_validators_unstaking,
			num_validator_signing_info,
			num_validator_signing_info_new,
			num_committees,
			num_committees_new,
			num_committees_subsidized,
			num_committees_retired,
			num_committee_validators,
			num_poll_snapshots
		FROM block_summaries FINAL
		WHERE 1=1
	`

	// Add cursor condition if provided
	if cursor > 0 {
		if sortDesc {
			query += fmt.Sprintf(" AND height < %d", cursor)
		} else {
			query += fmt.Sprintf(" AND height > %d", cursor)
		}
	}

	// Add ordering
	if sortDesc {
		query += " ORDER BY height DESC"
	} else {
		query += " ORDER BY height ASC"
	}

	// Add limit
	query += fmt.Sprintf(" LIMIT %d", limit)

	rows, err := db.Db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var summaries []*indexermodels.BlockSummary
	for rows.Next() {
		var bs indexermodels.BlockSummary
		err := rows.Scan(
			&bs.Height,
			&bs.HeightTime,
			&bs.NumTxs,
			&bs.NumTxsSend,
			&bs.NumTxsDelegate,
			&bs.NumTxsUndelegate,
			&bs.NumTxsStake,
			&bs.NumTxsUnstake,
			&bs.NumTxsEditStake,
			&bs.NumTxsVote,
			&bs.NumTxsProposal,
			&bs.NumTxsContract,
			&bs.NumTxsSystem,
			&bs.NumTxsUnknown,
			&bs.NumTxsPause,
			&bs.NumTxsUnpause,
			&bs.NumTxsChangeParameter,
			&bs.NumTxsDaoTransfer,
			&bs.NumTxsCertificateResults,
			&bs.NumTxsSubsidy,
			&bs.NumTxsCreateOrder,
			&bs.NumTxsEditOrder,
			&bs.NumTxsDeleteOrder,
			&bs.NumTxsDexLimitOrder,
			&bs.NumTxsDexLiquidityDeposit,
			&bs.NumTxsDexLiquidityWithdraw,
			&bs.NumAccounts,
			&bs.NumAccountsNew,
			&bs.NumEvents,
			&bs.NumEventsReward,
			&bs.NumEventsSlash,
			&bs.NumEventsDexLiquidityDeposit,
			&bs.NumEventsDexLiquidityWithdraw,
			&bs.NumEventsDexSwap,
			&bs.NumEventsOrderBookSwap,
			&bs.NumEventsAutomaticPause,
			&bs.NumEventsAutomaticBeginUnstaking,
			&bs.NumEventsAutomaticFinishUnstaking,
			&bs.NumOrders,
			&bs.NumOrdersNew,
			&bs.NumOrdersOpen,
			&bs.NumOrdersFilled,
			&bs.NumOrdersCancelled,
			&bs.NumOrdersExpired,
			&bs.NumPools,
			&bs.NumPoolsNew,
			&bs.NumDexPrices,
			&bs.NumDexOrders,
			&bs.NumDexOrdersFuture,
			&bs.NumDexOrdersLocked,
			&bs.NumDexOrdersComplete,
			&bs.NumDexOrdersSuccess,
			&bs.NumDexOrdersFailed,
			&bs.NumDexDeposits,
			&bs.NumDexDepositsPending,
			&bs.NumDexDepositsComplete,
			&bs.NumDexWithdrawals,
			&bs.NumDexWithdrawalsPending,
			&bs.NumDexWithdrawalsComplete,
			&bs.NumDexPoolPointsHolders,
			&bs.NumDexPoolPointsHoldersNew,
			&bs.ParamsChanged,
			&bs.NumValidators,
			&bs.NumValidatorsNew,
			&bs.NumValidatorsActive,
			&bs.NumValidatorsPaused,
			&bs.NumValidatorsUnstaking,
			&bs.NumValidatorSigningInfo,
			&bs.NumValidatorSigningInfoNew,
			&bs.NumCommittees,
			&bs.NumCommitteesNew,
			&bs.NumCommitteesSubsidized,
			&bs.NumCommitteesRetired,
			&bs.NumCommitteeValidators,
			&bs.NumPollSnapshots,
		)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, &bs)
	}

	return summaries, rows.Err()
}

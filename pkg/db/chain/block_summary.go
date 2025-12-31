package chain

import (
	"context"
	"fmt"

	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initBlockSummaries initializes the block_summaries table and its staging table.
// This table tracks all 16 indexed entities with comprehensive field coverage (90+ fields).
func (db *DB) initBlockSummaries(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToSchemaSQL(indexermodels.BlockSummaryColumns)

	// Create production table with ReplacingMergeTree
	productionQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" %s (
			%s
		) ENGINE = %s
		ORDER BY (height)
	`, db.Name, indexermodels.BlockSummariesProductionTableName, db.OnCluster(), schemaSQL, db.Engine(indexermodels.BlockSummariesProductionTableName, "ReplacingMergeTree", "height"))

	if err := db.Exec(ctx, productionQuery); err != nil {
		return fmt.Errorf("create %s: %w", indexermodels.BlockSummariesProductionTableName, err)
	}

	// Create staging table with MergeTree
	stagingQuery := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" %s (
			%s
		) ENGINE = %s
		ORDER BY (height)
	`, db.Name, indexermodels.BlockSummariesStagingTableName, db.OnCluster(), schemaSQL, db.Engine(indexermodels.BlockSummariesStagingTableName, "ReplacingMergeTree", "height"))

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
		INSERT INTO "%s"."block_summaries_staging" (
			height,
			height_time,
			num_txs,
			num_txs_send,
			num_txs_stake,
			num_txs_unstake,
			num_txs_edit_stake,
			num_txs_start_poll,
			num_txs_vote_poll,
			num_txs_lock_order,
			num_txs_close_order,
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
			num_pool_points_holders,
			num_pool_points_holders_new,
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
			num_committee_payments,
			supply_changed,
			supply_total,
			supply_staked,
			supply_delegated_only
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
		summary.NumTxsStake,
		summary.NumTxsUnstake,
		summary.NumTxsEditStake,
		summary.NumTxsStartPoll,
		summary.NumTxsVotePoll,
		summary.NumTxsLockOrder,
		summary.NumTxsCloseOrder,
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
		summary.NumCommitteePayments,
		summary.SupplyChanged,
		summary.SupplyTotal,
		summary.SupplyStaked,
		summary.SupplyDelegatedOnly,
	)
	if err != nil {
		return err
	}

	return batch.Send()
}

// GetBlockSummary retrieves a block summary by height from the production table.
func (db *DB) GetBlockSummary(ctx context.Context, height uint64, staging bool) (*indexermodels.BlockSummary, error) {
	var bs indexermodels.BlockSummary

	tableName := indexermodels.BlockSummariesStagingTableName
	if !staging {
		tableName = indexermodels.BlockSummariesProductionTableName
	}

	query := fmt.Sprintf(`
		SELECT *
		FROM "%s"."%s" FINAL
		WHERE height = ?
		LIMIT 1
	`, db.Name, tableName)

	err := db.Db.QueryRow(ctx, query, height).ScanStruct(&bs)

	return &bs, err
}

// QueryBlockSummaries retrieves a paginated list of block summaries ordered by height.
// If sortDesc is true, orders by height DESC (newest first), otherwise ASC (oldest first).
// If cursor > 0 and sortDesc is true, only summaries with height < cursor are returned.
// If cursor > 0 and sortDesc is false, only summaries with height > cursor are returned.
// The limit parameter controls the maximum number of rows returned (+1 for pagination detection).
func (db *DB) QueryBlockSummaries(ctx context.Context, cursor uint64, limit int, sortDesc bool) ([]*indexermodels.BlockSummary, error) {
	query := fmt.Sprintf(`
		SELECT *
		FROM "%s"."block_summaries" FINAL
		WHERE 1=1
	`, db.Name)

	// Build query with parameterized values for security
	var args []interface{}

	// Add cursor condition if provided
	if cursor > 0 {
		if sortDesc {
			query += " AND height < ?"
		} else {
			query += " AND height > ?"
		}
		args = append(args, cursor)
	}

	// Add ordering
	if sortDesc {
		query += " ORDER BY height DESC"
	} else {
		query += " ORDER BY height ASC"
	}

	// Add limit (parameterized for best practices)
	query += " LIMIT ?"
	args = append(args, limit)

	rows, err := db.Db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var summaries []*indexermodels.BlockSummary
	for rows.Next() {
		var bs indexermodels.BlockSummary
		err := rows.ScanStruct(&bs)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, &bs)
	}

	return summaries, rows.Err()
}

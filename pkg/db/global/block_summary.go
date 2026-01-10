package global

import (
	"context"
	"fmt"

	"github.com/canopy-network/canopyx/pkg/db/clickhouse"

	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// initBlockSummaries initializes the block_summaries table.
// This table tracks all 16 indexed entities with comprehensive field coverage (90+ fields).
func (db *DB) initBlockSummaries(ctx context.Context) error {
	schemaSQL := indexermodels.ColumnsToGlobalSchemaSQL(indexermodels.BlockSummaryColumns)

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s"."%s" (
			%s
		) ENGINE = %s
		ORDER BY (chain_id, height)
	`, db.Name, indexermodels.BlockSummariesProductionTableName, schemaSQL, clickhouse.ReplicatedEngine(clickhouse.ReplacingMergeTree, "height"))

	return db.Exec(ctx, query)
}

// InsertBlockSummaries persists block summary data into the block_summaries table.
// All 16 entities are tracked with comprehensive field coverage.
func (db *DB) InsertBlockSummaries(ctx context.Context, summary *indexermodels.BlockSummary) error {
	if summary == nil {
		return nil
	}

	query := fmt.Sprintf(`
		INSERT INTO "%s"."%s" (
			chain_id,
			height,
			height_time,
			total_transactions,
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
			num_dex_deposits_locked,
			num_dex_deposits_complete,
			num_dex_withdrawals,
			num_dex_withdrawals_pending,
			num_dex_withdrawals_locked,
			num_dex_withdrawals_complete,
			num_pool_points_holders,
			num_pool_points_holders_new,
			params_changed,
			num_validators,
			num_validators_new,
			num_validators_active,
			num_validators_paused,
			num_validators_unstaking,
			num_validator_non_signing_info,
			num_validator_non_signing_info_new,
			num_validator_double_signing_info,
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
		) VALUES`, db.Name, indexermodels.BlockSummariesProductionTableName)

	batch, err := db.PrepareBatch(ctx, query)
	if err != nil {
		return err
	}
	// Ensure the batch is closed, especially if not all data is sent immediately
	defer func() { _ = batch.Close() }()

	err = batch.Append(
		db.ChainID,
		summary.Height,
		summary.HeightTime,
		summary.TotalTransactions,
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
		summary.NumDexDepositsLocked,
		summary.NumDexDepositsComplete,
		summary.NumDexWithdrawals,
		summary.NumDexWithdrawalsPending,
		summary.NumDexWithdrawalsLocked,
		summary.NumDexWithdrawalsComplete,
		summary.NumDexPoolPointsHolders,
		summary.NumDexPoolPointsHoldersNew,
		summary.ParamsChanged,
		summary.NumValidators,
		summary.NumValidatorsNew,
		summary.NumValidatorsActive,
		summary.NumValidatorsPaused,
		summary.NumValidatorsUnstaking,
		summary.NumValidatorNonSigningInfo,
		summary.NumValidatorNonSigningInfoNew,
		summary.NumValidatorDoubleSigningInfo,
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
		_ = batch.Abort()
		return err
	}

	return batch.Send()
}

// GetBlockSummary retrieves a block summary by height.
// Does not use FINAL since we query by exact (chain_id, height) which is unique.
func (db *DB) GetBlockSummary(ctx context.Context, height uint64) (*indexermodels.BlockSummary, error) {
	var bs indexermodels.BlockSummary

	query := fmt.Sprintf(`
		SELECT *
		FROM "%s"."%s"
		WHERE chain_id = ? AND height = ?
		LIMIT 1
	`, db.Name, indexermodels.BlockSummariesProductionTableName)

	err := db.Db.QueryRow(ctx, query, db.ChainID, height).ScanStruct(&bs)

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
		FROM "%s"."%s" FINAL
		WHERE chain_id = ?
	`, db.Name, indexermodels.BlockSummariesProductionTableName)

	// Build query with parameterized values for security
	args := []interface{}{db.ChainID}

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

package activity

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"go.temporal.io/sdk/temporal"
)

// SaveBlockSummary saves aggregated entity counts for a block after all entities have been indexed.
// This activity should run after all entity indexing activities (IndexTransactions, IndexEvents, etc.) complete.
// Returns output containing execution duration in milliseconds.
func (c *Context) SaveBlockSummary(ctx context.Context, in types.SaveBlockSummaryInput) (types.SaveBlockSummaryOutput, error) {
	start := time.Now()

	// Acquire (or ping) the chain DB
	chainDb, chainDbErr := c.NewChainDb(ctx, in.ChainID)
	if chainDbErr != nil {
		return types.SaveBlockSummaryOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	// Convert BlockSummaries (with maps) to BlockSummary model (with individual fields)
	summary := convertToBlockSummaryModel(in.Height, in.BlockTime, in.Summaries)

	// Insert block summary to staging table (two-phase commit pattern)
	if err := chainDb.InsertBlockSummariesStaging(ctx, summary); err != nil {
		return types.SaveBlockSummaryOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.SaveBlockSummaryOutput{DurationMs: durationMs}, nil
}

// convertToBlockSummaryModel converts the workflow's BlockSummaries type (which uses maps for
// dynamic type counting) into the database BlockSummary model (which uses individual typed fields).
// This follows the pattern established in params.go - individual fields instead of maps for better
// queryability and type safety in ClickHouse.
func convertToBlockSummaryModel(height uint64, blockTime time.Time, summaries types.BlockSummaries) *indexermodels.BlockSummary {
	summary := &indexermodels.BlockSummary{
		Height:     height,
		HeightTime: blockTime,

		// Transactions - convert map to individual fields
		NumTxs:                     summaries.NumTxs,
		NumTxsSend:                 summaries.TxCountsByType["send"],
		NumTxsDelegate:             summaries.TxCountsByType["delegate"],
		NumTxsUndelegate:           summaries.TxCountsByType["undelegate"],
		NumTxsStake:                summaries.TxCountsByType["stake"],
		NumTxsUnstake:              summaries.TxCountsByType["unstake"],
		NumTxsEditStake:            summaries.TxCountsByType["edit_stake"],
		NumTxsVote:                 summaries.TxCountsByType["vote"],
		NumTxsProposal:             summaries.TxCountsByType["proposal"],
		NumTxsContract:             summaries.TxCountsByType["contract"],
		NumTxsSystem:               summaries.TxCountsByType["system"],
		NumTxsUnknown:              summaries.TxCountsByType["unknown"],
		NumTxsPause:                summaries.TxCountsByType["pause"],
		NumTxsUnpause:              summaries.TxCountsByType["unpause"],
		NumTxsChangeParameter:      summaries.TxCountsByType["change_parameter"],
		NumTxsDaoTransfer:          summaries.TxCountsByType["dao_transfer"],
		NumTxsCertificateResults:   summaries.TxCountsByType["certificate_results"],
		NumTxsSubsidy:              summaries.TxCountsByType["subsidy"],
		NumTxsCreateOrder:          summaries.TxCountsByType["create_order"],
		NumTxsEditOrder:            summaries.TxCountsByType["edit_order"],
		NumTxsDeleteOrder:          summaries.TxCountsByType["delete_order"],
		NumTxsDexLimitOrder:        summaries.TxCountsByType["dex_limit_order"],
		NumTxsDexLiquidityDeposit:  summaries.TxCountsByType["dex_liquidity_deposit"],
		NumTxsDexLiquidityWithdraw: summaries.TxCountsByType["dex_liquidity_withdraw"],

		// Accounts
		NumAccounts:    summaries.NumAccounts,
		NumAccountsNew: summaries.NumAccountsNew,

		// Events - convert map to individual fields
		NumEvents:                         summaries.NumEvents,
		NumEventsReward:                   summaries.EventCountsByType["reward"],
		NumEventsSlash:                    summaries.EventCountsByType["slash"],
		NumEventsDexLiquidityDeposit:      summaries.EventCountsByType["dex_liquidity_deposit"],
		NumEventsDexLiquidityWithdraw:     summaries.EventCountsByType["dex_liquidity_withdraw"],
		NumEventsDexSwap:                  summaries.EventCountsByType["dex_swap"],
		NumEventsOrderBookSwap:            summaries.EventCountsByType["order_book_swap"],
		NumEventsAutomaticPause:           summaries.EventCountsByType["automatic_pause"],
		NumEventsAutomaticBeginUnstaking:  summaries.EventCountsByType["automatic_begin_unstaking"],
		NumEventsAutomaticFinishUnstaking: summaries.EventCountsByType["automatic_finish_unstaking"],

		// Orders
		NumOrders:          summaries.NumOrders,
		NumOrdersNew:       summaries.NumOrdersNew,
		NumOrdersOpen:      summaries.NumOrdersOpen,
		NumOrdersFilled:    summaries.NumOrdersFilled,
		NumOrdersCancelled: summaries.NumOrdersCancelled,
		NumOrdersExpired:   summaries.NumOrdersExpired,

		// Pools
		NumPools:    summaries.NumPools,
		NumPoolsNew: summaries.NumPoolsNew,

		// DexPrices
		NumDexPrices: summaries.NumDexPrices,

		// DexOrders
		NumDexOrders:         summaries.NumDexOrders,
		NumDexOrdersFuture:   summaries.NumDexOrdersFuture,
		NumDexOrdersLocked:   summaries.NumDexOrdersLocked,
		NumDexOrdersComplete: summaries.NumDexOrdersComplete,
		NumDexOrdersSuccess:  summaries.NumDexOrdersSuccess,
		NumDexOrdersFailed:   summaries.NumDexOrdersFailed,

		// DexDeposits
		NumDexDeposits:         summaries.NumDexDeposits,
		NumDexDepositsPending:  summaries.NumDexDepositsPending,
		NumDexDepositsComplete: summaries.NumDexDepositsComplete,

		// DexWithdrawals
		NumDexWithdrawals:         summaries.NumDexWithdrawals,
		NumDexWithdrawalsPending:  summaries.NumDexWithdrawalsPending,
		NumDexWithdrawalsComplete: summaries.NumDexWithdrawalsComplete,

		// DexPoolPointsByHolder
		NumDexPoolPointsHolders:    summaries.NumDexPoolPointsHolders,
		NumDexPoolPointsHoldersNew: summaries.NumDexPoolPointsHoldersNew,

		// Params
		ParamsChanged: summaries.ParamsChanged,

		// Validators
		NumValidators:          summaries.NumValidators,
		NumValidatorsNew:       summaries.NumValidatorsNew,
		NumValidatorsActive:    summaries.NumValidatorsActive,
		NumValidatorsPaused:    summaries.NumValidatorsPaused,
		NumValidatorsUnstaking: summaries.NumValidatorsUnstaking,

		// ValidatorSigningInfo
		NumValidatorSigningInfo:    summaries.NumValidatorSigningInfo,
		NumValidatorSigningInfoNew: summaries.NumValidatorSigningInfoNew,

		// Committees
		NumCommittees:           summaries.NumCommittees,
		NumCommitteesNew:        summaries.NumCommitteesNew,
		NumCommitteesSubsidized: summaries.NumCommitteesSubsidized,
		NumCommitteesRetired:    summaries.NumCommitteesRetired,

		// CommitteeValidators
		NumCommitteeValidators: summaries.NumCommitteeValidators,

		// PollSnapshots
		NumPollSnapshots: summaries.NumPollSnapshots,
	}

	return summary
}

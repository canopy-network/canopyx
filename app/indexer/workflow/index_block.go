package workflow

import (
	"encoding/json"
	"fmt"
	"time"

	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// stagingEntities is the list of entities that use the two-phase commit pattern
// (staging table -> promotion -> production table). These entities need both
// promotion during indexing and cleanup after promotion.
//
// Note: poll_snapshots is NOT in this list because it uses direct inserts
// to production via a scheduled workflow (not per-block indexing).
var stagingEntities = []string{
	"blocks",
	"txs",
	"block_summaries",
	"accounts",
	"events",
	"pools",
	"orders",
	"dex_prices",
	"params",
	"validators",
	"validator_non_signing_info",
	"validator_double_signing_info",
	"committees",
	"committee_validators",
	"committee_payments",
	"dex_orders",
	"dex_deposits",
	"dex_withdrawals",
	"pool_points_by_holder",
	"supply",
}

// IndexBlockWorkflow kicks off indexing of a block for a given chain in a separate workflow at `index:<chain>` queue.
// It tracks the execution time of each activity and records detailed timing metrics.
func (wc *Context) IndexBlockWorkflow(ctx workflow.Context, in types.WorkflowIndexBlockInput) error {
	//  check if the current options are good enough for indexing the block
	retry := &temporal.RetryPolicy{
		InitialInterval:    100 * time.Millisecond, // Start quickly for block propagation delays
		BackoffCoefficient: 1.2,                    // Grow moderately (200ms, 300ms, 450ms, 675ms, 1s...)
		MaximumInterval:    2 * time.Second,        // Cap at 2s for faster retries when block isn't ready
		MaximumAttempts:    0,                      // zero means unlimited - we want it keeps trying to index a block until it succeeds
	}

	// Extract the task queue from workflow info - activities must run on the same queue as the workflow
	// This ensures activities run on the correct queue (live or historical) that the workflow was scheduled to
	info := workflow.GetInfo(ctx)
	taskQueue := info.TaskQueueName

	ao := workflow.ActivityOptions{
		// NOTE: this should never reach 2 minutes to index a block, but WHO knows...
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy:         retry,
		// Use the same task queue as the workflow (live or historical)
		TaskQueue: taskQueue,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Collect timing data for each activity
	timings := make(map[string]float64)

	// 1. PrepareIndexBlock - check if block needs indexing
	var prepareOut types.ActivityPrepareIndexBlockOutput
	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.PrepareIndexBlock, in).Get(ctx, &prepareOut); err != nil {
		return err
	}
	timings["prepare_index_ms"] = prepareOut.DurationMs

	// If block should be skipped, record timing and exit early
	if prepareOut.Skip {
		// Even for skipped blocks, we record that we checked them
		detailBytes, _ := json.Marshal(timings)
		recordInput := types.ActivityRecordIndexedInput{
			Height:         in.Height,
			IndexingTimeMs: prepareOut.DurationMs,
			IndexingDetail: string(detailBytes),
		}
		return workflow.ExecuteActivity(ctx, wc.ActivityContext.RecordIndexed, recordInput).Get(ctx, nil)
	}

	// 2. FetchBlock - fetch block from RPC (local activity for fast retries)
	// This uses a local activity to retry quickly when waiting for block propagation
	// NO TIMEOUT: We want unlimited retries since blocks MUST eventually be indexed
	// The retry policy handles backoff, we just need to be patient
	var fetchOut types.ActivityFetchBlockOutput
	localActivityOpts := workflow.LocalActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		RetryPolicy:         retry,
	}
	localCtx := workflow.WithLocalActivityOptions(ctx, localActivityOpts)
	if err := workflow.ExecuteLocalActivity(localCtx, wc.ActivityContext.FetchBlockFromRPC, types.ActivityFetchBlockInput{
		Height: in.Height,
	}).Get(localCtx, &fetchOut); err != nil {
		return err
	}
	timings["fetch_block_ms"] = fetchOut.DurationMs

	heightTime := time.UnixMicro(int64(fetchOut.Block.BlockHeader.Time))

	var eventsOut types.ActivityIndexEventsOutput
	indexAtHeight := types.ActivityIndexAtHeight{
		Height:    in.Height,
		BlockTime: heightTime,
	}

	// save events first so the other indexing process can list and filter by type
	eventsFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexEvents, indexAtHeight)
	if err := eventsFuture.Get(ctx, &eventsOut); err != nil {
		return err
	}
	timings["index_events_ms"] = eventsOut.DurationMs

	// All activities run concurrently for maximum throughput
	// Each activity writes to its own staging table to avoid conflicts
	var (
		saveBlockOut  types.ActivitySaveBlockOutput
		txOut         types.ActivityIndexTransactionsOutput
		accountsOut   types.ActivityIndexAccountsOutput
		poolsOut      types.ActivityIndexPoolsOutput
		ordersOut     types.ActivityIndexOrdersOutput
		pricesOut     types.ActivityIndexDexPricesOutput
		paramsOut     types.ActivityIndexParamsOutput
		validatorsOut types.ActivityIndexValidatorsOutput
		committeesOut types.ActivityIndexCommitteesOutput
		dexBatchOut   types.ActivityIndexDexBatchOutput
		supplyOut     types.ActivityIndexSupplyOutput
	)

	// Create futures for all parallel operations
	saveBlockInput := types.ActivitySaveBlockInput{
		Height: in.Height,
		Block:  fetchOut.Block,
	}

	// Capture start time for parallel data indexing (wall-clock measurement)
	dataIndexingStart := workflow.Now(ctx)

	saveBlockFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.SaveBlock, saveBlockInput)
	txFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexTransactions, indexAtHeight)
	accountsFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexAccounts, indexAtHeight)
	poolsFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexPools, indexAtHeight)
	ordersFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexOrders, indexAtHeight)
	pricesFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexDexPrices, indexAtHeight)
	paramsFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexParams, indexAtHeight)
	validatorsFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexValidators, indexAtHeight)
	committeesFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexCommittees, indexAtHeight)
	dexBatchFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexDexBatch, indexAtHeight)
	supplyFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexSupply, indexAtHeight)

	// Wait for all parallel operations to complete
	if err := saveBlockFuture.Get(ctx, &saveBlockOut); err != nil {
		return err
	}
	timings["save_block_ms"] = saveBlockOut.DurationMs

	if err := txFuture.Get(ctx, &txOut); err != nil {
		return err
	}
	timings["index_transactions_ms"] = txOut.DurationMs

	if err := accountsFuture.Get(ctx, &accountsOut); err != nil {
		return err
	}
	timings["index_accounts_ms"] = accountsOut.DurationMs

	if err := eventsFuture.Get(ctx, &eventsOut); err != nil {
		return err
	}
	timings["index_events_ms"] = eventsOut.DurationMs

	if err := poolsFuture.Get(ctx, &poolsOut); err != nil {
		return err
	}
	timings["index_pools_ms"] = poolsOut.DurationMs

	if err := ordersFuture.Get(ctx, &ordersOut); err != nil {
		return err
	}
	timings["index_orders_ms"] = ordersOut.DurationMs

	if err := pricesFuture.Get(ctx, &pricesOut); err != nil {
		return err
	}
	timings["index_dex_prices_ms"] = pricesOut.DurationMs

	if err := paramsFuture.Get(ctx, &paramsOut); err != nil {
		return err
	}
	timings["index_params_ms"] = paramsOut.DurationMs

	if err := validatorsFuture.Get(ctx, &validatorsOut); err != nil {
		return err
	}
	timings["index_validators_ms"] = validatorsOut.DurationMs

	if err := committeesFuture.Get(ctx, &committeesOut); err != nil {
		return err
	}
	timings["index_committees_ms"] = committeesOut.DurationMs

	if err := dexBatchFuture.Get(ctx, &dexBatchOut); err != nil {
		return err
	}
	timings["index_dex_batch_ms"] = dexBatchOut.DurationMs

	if err := supplyFuture.Get(ctx, &supplyOut); err != nil {
		return err
	}
	timings["index_supply_ms"] = supplyOut.DurationMs

	// Capture end time for parallel data indexing and calculate wall-clock duration
	dataIndexingEnd := workflow.Now(ctx)
	dataIndexingElapsed := dataIndexingEnd.Sub(dataIndexingStart)
	timings["data_indexing"] = float64(dataIndexingElapsed.Milliseconds())

	// Collect all summaries for aggregation from all indexed entities
	// Note: Some detailed breakdowns (e.g., NumOrdersNew, NumOrdersOpen) are not yet
	// returned by activity outputs and will remain zero until those activities are enhanced.
	summary := &indexermodels.BlockSummary{
		Height:            in.Height,
		HeightTime:        heightTime,
		TotalTransactions: fetchOut.Block.BlockHeader.TotalTxs, // Lifetime number of transactions across all blocks

		// Transaction counts by type (populated from TxCountsByType map)
		NumTxs:                     txOut.NumTxs,
		NumTxsSend:                 txOut.TxCountsByType["send"],
		NumTxsStake:                txOut.TxCountsByType["stake"],
		NumTxsUnstake:              txOut.TxCountsByType["unstake"],
		NumTxsEditStake:            txOut.TxCountsByType["edit_stake"],
		NumTxsStartPoll:            txOut.TxCountsByType["startPoll"],
		NumTxsVotePoll:             txOut.TxCountsByType["votePoll"],
		NumTxsLockOrder:            txOut.TxCountsByType["lockOrder"],
		NumTxsCloseOrder:           txOut.TxCountsByType["closeOrder"],
		NumTxsUnknown:              txOut.TxCountsByType["unknown"],
		NumTxsPause:                txOut.TxCountsByType["pause"],
		NumTxsUnpause:              txOut.TxCountsByType["unpause"],
		NumTxsChangeParameter:      txOut.TxCountsByType["changeParameter"],
		NumTxsDaoTransfer:          txOut.TxCountsByType["daoTransfer"],
		NumTxsCertificateResults:   txOut.TxCountsByType["certificateResults"],
		NumTxsSubsidy:              txOut.TxCountsByType["subsidy"],
		NumTxsCreateOrder:          txOut.TxCountsByType["createOrder"],
		NumTxsEditOrder:            txOut.TxCountsByType["editOrder"],
		NumTxsDeleteOrder:          txOut.TxCountsByType["deleteOrder"],
		NumTxsDexLimitOrder:        txOut.TxCountsByType["dexLimitOrder"],
		NumTxsDexLiquidityDeposit:  txOut.TxCountsByType["dexLiquidityDeposit"],
		NumTxsDexLiquidityWithdraw: txOut.TxCountsByType["dexLiquidityWithdraw"],

		// Account counts (from accounts activity)
		NumAccounts:    accountsOut.NumAccounts,
		NumAccountsNew: accountsOut.NumAccountsNew,

		// Event counts by type (populated from EventCountsByType map)
		NumEvents:                         eventsOut.NumEvents,
		NumEventsReward:                   eventsOut.EventCountsByType["reward"],
		NumEventsSlash:                    eventsOut.EventCountsByType["slash"],
		NumEventsDexLiquidityDeposit:      eventsOut.EventCountsByType["dex-liquidity-deposit"],
		NumEventsDexLiquidityWithdraw:     eventsOut.EventCountsByType["dex-liquidity-withdraw"],
		NumEventsDexSwap:                  eventsOut.EventCountsByType["dex-swap"],
		NumEventsOrderBookSwap:            eventsOut.EventCountsByType["order-book-swap"],
		NumEventsAutomaticPause:           eventsOut.EventCountsByType["automatic-pause"],
		NumEventsAutomaticBeginUnstaking:  eventsOut.EventCountsByType["automatic-begin-unstaking"],
		NumEventsAutomaticFinishUnstaking: eventsOut.EventCountsByType["automatic-finish-unstaking"],
		// Order counts (from orders activity)
		NumOrders:          ordersOut.NumOrders,
		NumOrdersNew:       ordersOut.NumOrdersNew,
		NumOrdersOpen:      ordersOut.NumOrdersOpen,
		NumOrdersFilled:    ordersOut.NumOrdersFilled,
		NumOrdersCancelled: ordersOut.NumOrdersCancelled,

		// Pool counts (from pools activity)
		NumPools:    poolsOut.NumPools,
		NumPoolsNew: poolsOut.NumPoolsNew,

		// DEX price counts
		NumDexPrices: pricesOut.NumPrices,

		// DEX order counts (from dex_batch activity)
		NumDexOrders:         dexBatchOut.NumOrders,
		NumDexOrdersFuture:   dexBatchOut.NumOrdersFuture,
		NumDexOrdersLocked:   dexBatchOut.NumOrdersLocked,
		NumDexOrdersComplete: dexBatchOut.NumOrdersComplete,
		NumDexOrdersSuccess:  dexBatchOut.NumOrdersSuccess,
		NumDexOrdersFailed:   dexBatchOut.NumOrdersFailed,

		// DEX deposit counts (from dex_batch activity)
		NumDexDeposits:         dexBatchOut.NumDeposits,
		NumDexDepositsPending:  dexBatchOut.NumDepositsPending,
		NumDexDepositsLocked:   dexBatchOut.NumDepositsLocked,
		NumDexDepositsComplete: dexBatchOut.NumDepositsComplete,

		// DEX withdrawal counts (from dex_batch activity)
		NumDexWithdrawals:         dexBatchOut.NumWithdrawals,
		NumDexWithdrawalsPending:  dexBatchOut.NumWithdrawalsPending,
		NumDexWithdrawalsLocked:   dexBatchOut.NumWithdrawalsLocked,
		NumDexWithdrawalsComplete: dexBatchOut.NumWithdrawalsComplete,

		// DEX pool points (from pools activity)
		NumDexPoolPointsHolders:    poolsOut.NumPoolHolders,
		NumDexPoolPointsHoldersNew: poolsOut.NumPoolHoldersNew,

		// Params
		ParamsChanged: paramsOut.ParamsChanged,

		// Validator counts (from validators activity)
		NumValidators:                 validatorsOut.NumValidators,
		NumValidatorsNew:              validatorsOut.NumValidatorsNew,
		NumValidatorsActive:           validatorsOut.NumValidatorsActive,
		NumValidatorsPaused:           validatorsOut.NumValidatorsPaused,
		NumValidatorsUnstaking:        validatorsOut.NumValidatorsUnstaking,
		NumValidatorNonSigningInfo:    validatorsOut.NumNonSigningInfos,
		NumValidatorNonSigningInfoNew: validatorsOut.NumNonSigningInfosNew,
		NumValidatorDoubleSigningInfo: validatorsOut.NumDoubleSigningInfos,

		// Committee counts (populated from activity output)
		NumCommittees:           committeesOut.NumCommittees,
		NumCommitteesNew:        committeesOut.NumCommitteesNew,
		NumCommitteesSubsidized: committeesOut.NumCommitteesSubsidized,
		NumCommitteesRetired:    committeesOut.NumCommitteesRetired,
		NumCommitteeValidators:  validatorsOut.NumCommitteeValidators,
		NumCommitteePayments:    committeesOut.NumCommitteePayments,

		// Supply metrics (from supply activity)
		SupplyChanged:       supplyOut.Changed,
		SupplyTotal:         supplyOut.Total,
		SupplyStaked:        supplyOut.Staked,
		SupplyDelegatedOnly: supplyOut.DelegatedOnly,
	}

	// Phase 2: SaveBlockSummary - aggregate and save all entity summaries
	saveInput := types.ActivitySaveBlockSummaryInput{
		Height:    in.Height,
		BlockTime: heightTime,
		Summary:   summary,
	}
	var saveSummaryOut types.ActivitySaveBlockSummaryOutput
	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.SaveBlockSummary, saveInput).Get(ctx, &saveSummaryOut); err != nil {
		return err
	}
	timings["save_block_summary_ms"] = saveSummaryOut.DurationMs

	// Phase 3: Promote all entities in parallel from staging to production
	// This follows the two-phase commit pattern for data consistency
	// Uses stagingEntities which is the list of entities that use staging tables

	// Capture start time for parallel data release (wall-clock measurement)
	dataReleaseStart := workflow.Now(ctx)

	promoteFutures := make([]workflow.Future, 0, len(stagingEntities))
	promoteEntities := make([]string, 0, len(stagingEntities))

	for _, entity := range stagingEntities {
		f := workflow.ExecuteActivity(ctx, wc.ActivityContext.PromoteData, types.ActivityPromoteDataInput{
			Entity: entity,
			Height: in.Height,
		})
		promoteFutures = append(promoteFutures, f)
		promoteEntities = append(promoteEntities, entity)
	}

	// Wait for all promotions to complete and collect individual timings for debugging
	for i, f := range promoteFutures {
		var out types.ActivityPromoteDataOutput
		if err := f.Get(ctx, &out); err != nil {
			return err
		}
		// Store individual promotion times for debugging (not added to total)
		timings[fmt.Sprintf("promote_%s", promoteEntities[i])] = out.DurationMs
	}

	// Capture end time for parallel data release and calculate wall-clock duration
	dataReleaseEnd := workflow.Now(ctx)
	dataReleaseElapsed := dataReleaseEnd.Sub(dataReleaseStart)
	timings["data_release"] = float64(dataReleaseElapsed.Milliseconds())

	// NOTE: Per-block cleanup removed - staging cleanup now runs as a periodic batch workflow
	// (CleanupStagingWorkflow in cleanup_staging.go) that runs hourly and cleans all
	// heights indexed more than 1 hour ago. This reduces mutation count from
	// ~20 per block to ~20 per hour (one batch DELETE per table).

	// Phase 4: Update TVL snapshot for the current hour
	// This runs after promotions so it captures the just-indexed data.
	// ReplacingMergeTree handles repeated updates to the same hour.
	var tvlSnapshotOut types.ActivityUpdateTVLSnapshotOutput
	tvlInput := types.ActivityUpdateTVLSnapshotInput{
		Height:    in.Height,
		BlockTime: heightTime,
	}
	if err := workflow.ExecuteActivity(ctx, wc.ActivityContext.UpdateTVLSnapshot, tvlInput).Get(ctx, &tvlSnapshotOut); err != nil {
		// Log but don't fail the workflow - TVL snapshot is non-critical
		// The next block will update the snapshot anyway
		timings["tvl_snapshot_error"] = 1
	} else {
		timings["tvl_snapshot_ms"] = tvlSnapshotOut.DurationMs
	}

	// Calculate total indexing time (sum of critical path activities only)
	// Only sum wall-clock measurements, not individual parallel activity times
	totalMs := timings["prepare_index_ms"] +
		timings["fetch_block_ms"] +
		timings["index_events_ms"] +
		timings["data_indexing"] + // Wall-clock time for parallel indexing
		timings["data_release"] + // Wall-clock time for parallel promotions
		timings["save_block_summary_ms"]

	// Marshal timing details to JSON
	detailBytes, err := json.Marshal(timings)
	if err != nil {
		// If marshaling fails, use empty string but still record the total
		detailBytes = []byte("{}")
	}

	// Phase 5: RecordIndexed - persist indexing progress with timing metrics
	recordInput := types.ActivityRecordIndexedInput{
		Height:         in.Height,
		IndexingTimeMs: totalMs,
		IndexingDetail: string(detailBytes),
	}

	return workflow.ExecuteActivity(ctx, wc.ActivityContext.RecordIndexed, recordInput).Get(ctx, nil)
}

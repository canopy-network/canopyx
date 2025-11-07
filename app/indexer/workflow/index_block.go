package workflow

import (
	"encoding/json"
	"time"

	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"github.com/canopy-network/canopyx/pkg/db/entities"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// IndexBlockWorkflow kicks off indexing of a block for a given chain in a separate workflow at `index:<chain>` queue.
// It tracks the execution time of each activity and records detailed timing metrics.
func (wc *Context) IndexBlockWorkflow(ctx workflow.Context, in types.WorkflowIndexBlockInput) error {
	// TODO: add checks for chainId and height values at `in` param, also verify database exists (it should)
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
	// Timeout: 5 minutes to allow ~150 retry attempts (2s interval) for blocks not ready yet
	// With 20s block time, worst case is waiting ~60s for a block to become available
	var fetchOut types.ActivityFetchBlockOutput
	localActivityOpts := workflow.LocalActivityOptions{
		StartToCloseTimeout: 5 * time.Minute, // Allow enough time for blocks to become available
		RetryPolicy:         retry,           // Use the same fast retry policy
	}
	localCtx := workflow.WithLocalActivityOptions(ctx, localActivityOpts)
	if err := workflow.ExecuteLocalActivity(localCtx, wc.ActivityContext.FetchBlockFromRPC, types.ActivityFetchBlockInput{
		Height: in.Height,
	}).Get(localCtx, &fetchOut); err != nil {
		return err
	}
	timings["fetch_block_ms"] = fetchOut.DurationMs

	// 3. EnsureGenesisCached - cache genesis state BEFORE indexing height 1
	// Height 1 requires comparing RPC(1) vs Genesis(0), so genesis must be cached first
	if in.Height == 1 {
		genesisErr := workflow.ExecuteActivity(ctx, wc.ActivityContext.EnsureGenesisCached).Get(ctx, nil)
		if genesisErr != nil {
			return genesisErr
		}
	}

	heightTime := time.UnixMicro(fetchOut.Block.BlockHeader.Time)

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
		pollOut       types.ActivityIndexPollOutput
		dexBatchOut   types.ActivityIndexDexBatchOutput
	)

	// Create futures for all parallel operations
	saveBlockInput := types.ActivitySaveBlockInput{
		Height: in.Height,
		Block:  fetchOut.Block,
	}

	saveBlockFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.SaveBlock, saveBlockInput)
	txFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexTransactions, indexAtHeight)
	accountsFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexAccounts, indexAtHeight)
	poolsFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexPools, indexAtHeight)
	ordersFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexOrders, indexAtHeight)
	pricesFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexDexPrices, indexAtHeight)
	paramsFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexParams, indexAtHeight)
	validatorsFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexValidators, indexAtHeight)
	committeesFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexCommittees, indexAtHeight)
	pollFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexPoll, indexAtHeight)
	dexBatchFuture := workflow.ExecuteActivity(ctx, wc.ActivityContext.IndexDexBatch, indexAtHeight)

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

	if err := pollFuture.Get(ctx, &pollOut); err != nil {
		return err
	}
	timings["index_poll_ms"] = pollOut.DurationMs

	if err := dexBatchFuture.Get(ctx, &dexBatchOut); err != nil {
		return err
	}
	timings["index_dex_batch_ms"] = dexBatchOut.DurationMs

	// Collect all summaries for aggregation from all indexed entities
	// Note: Some detailed breakdowns (e.g., NumOrdersNew, NumOrdersOpen) are not yet
	// returned by activity outputs and will remain zero until those activities are enhanced.
	summary := &indexermodels.BlockSummary{
		Height:     in.Height,
		HeightTime: heightTime,
		// @TODO: map all this, add more, review everyone
		NumTxs:                            txOut.NumTxs,
		NumTxsSend:                        txOut.TxCountsByType["send"], // repeat...
		NumTxsDelegate:                    0,
		NumTxsUndelegate:                  0,
		NumTxsStake:                       0,
		NumTxsUnstake:                     0,
		NumTxsEditStake:                   0,
		NumTxsVote:                        0,
		NumTxsProposal:                    0,
		NumTxsContract:                    0,
		NumTxsSystem:                      0,
		NumTxsUnknown:                     0,
		NumTxsPause:                       0,
		NumTxsUnpause:                     0,
		NumTxsChangeParameter:             0,
		NumTxsDaoTransfer:                 0,
		NumTxsCertificateResults:          0,
		NumTxsSubsidy:                     0,
		NumTxsCreateOrder:                 0,
		NumTxsEditOrder:                   0,
		NumTxsDeleteOrder:                 0,
		NumTxsDexLimitOrder:               0,
		NumTxsDexLiquidityDeposit:         0,
		NumTxsDexLiquidityWithdraw:        0,
		NumAccounts:                       accountsOut.NumAccounts,
		NumAccountsNew:                    0,
		NumEvents:                         eventsOut.NumEvents,
		NumEventsReward:                   eventsOut.EventCountsByType["reward"],
		NumEventsSlash:                    0,
		NumEventsDexLiquidityDeposit:      0,
		NumEventsDexLiquidityWithdraw:     0,
		NumEventsDexSwap:                  0,
		NumEventsOrderBookSwap:            0,
		NumEventsAutomaticPause:           0,
		NumEventsAutomaticBeginUnstaking:  0,
		NumEventsAutomaticFinishUnstaking: 0,
		NumOrders:                         ordersOut.NumOrders,
		NumOrdersNew:                      0,
		NumOrdersOpen:                     0,
		NumOrdersFilled:                   0,
		NumOrdersCancelled:                0,
		NumOrdersExpired:                  0,
		NumPools:                          poolsOut.NumPools,
		NumPoolsNew:                       0,
		NumDexPrices:                      pricesOut.NumPrices,
		NumDexOrders:                      dexBatchOut.NumOrders,
		NumDexOrdersFuture:                0,
		NumDexOrdersLocked:                0,
		NumDexOrdersComplete:              0,
		NumDexOrdersSuccess:               0,
		NumDexOrdersFailed:                0,
		NumDexDeposits:                    dexBatchOut.NumDeposits,
		NumDexDepositsPending:             0,
		NumDexDepositsComplete:            0,
		NumDexWithdrawals:                 dexBatchOut.NumWithdrawals,
		NumDexWithdrawalsPending:          0,
		NumDexWithdrawalsComplete:         0,
		NumDexPoolPointsHolders:           0,
		NumDexPoolPointsHoldersNew:        0,
		ParamsChanged:                     paramsOut.ParamsChanged,
		NumValidators:                     validatorsOut.NumValidators,
		NumValidatorsNew:                  0,
		NumValidatorsActive:               0,
		NumValidatorsPaused:               0,
		NumValidatorsUnstaking:            0,
		NumValidatorSigningInfo:           validatorsOut.NumSigningInfos,
		NumValidatorSigningInfoNew:        0,
		NumCommittees:                     committeesOut.NumCommittees,
		NumCommitteesNew:                  0,
		NumCommitteesSubsidized:           0,
		NumCommitteesRetired:              0,
		NumCommitteeValidators:            0,
		NumPollSnapshots:                  pollOut.NumProposals,
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
	promoteEntities := []string{
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
		"validator_signing_info",
		"committees",
		"committee_validators",
		"poll_snapshots",
		"dex_orders",
		"dex_deposits",
		"dex_withdrawals",
		"pool_points_by_holder",
	}
	promoteFutures := make([]workflow.Future, 0, len(promoteEntities))

	for _, entity := range promoteEntities {
		f := workflow.ExecuteActivity(ctx, wc.ActivityContext.PromoteData, types.ActivityPromoteDataInput{
			Entity: entity,
			Height: in.Height,
		})
		promoteFutures = append(promoteFutures, f)
	}

	// Wait for all promotions to complete
	var promoteTimingsTotal float64
	for _, f := range promoteFutures {
		var out types.ActivityPromoteDataOutput
		if err := f.Get(ctx, &out); err != nil {
			return err
		}
		promoteTimingsTotal += out.DurationMs
	}
	timings["promote_all_ms"] = promoteTimingsTotal

	// Phase 4: Trigger async cleanup workflow (fire-and-forget)
	// Cleanup runs on ops queue to avoid blocking indexing queue
	cleanupWorkflowID := wc.TemporalClient.GetCleanupStagingWorkflowID(wc.ChainID, in.Height)
	opsQueue := wc.TemporalClient.GetIndexerOpsQueue(wc.ChainID)

	childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
		WorkflowID:        cleanupWorkflowID,
		TaskQueue:         opsQueue,
		ParentClosePolicy: enums.PARENT_CLOSE_POLICY_ABANDON, // Let cleanup continue after parent completes
	})

	// Start cleanup workflow and wait only for it to start, not complete
	cleanupFuture := workflow.ExecuteChildWorkflow(childCtx, wc.CleanupStagingWorkflow, types.WorkflowCleanupStagingInput{
		Height: in.Height,
	})

	// Wait for cleanup workflow to start (not complete)
	if err := cleanupFuture.GetChildWorkflowExecution().Get(ctx, nil); err != nil {
		// Log warning but don't fail - cleanup is non-critical
		workflow.GetLogger(ctx).Warn("Failed to start cleanup workflow", "error", err)
	}

	// Calculate total indexing time (sum of all activity durations)
	var totalMs float64
	for _, duration := range timings {
		totalMs += duration
	}

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

// CleanupStagingWorkflow asynchronously cleans up staging data for all entities after promotion.
// This workflow runs on the ops queue to avoid blocking the indexing queue.
// It iterates through all entities and removes promoted data from their staging tables.
//
// Design Philosophy:
// - Fire-and-forget: Parent workflow doesn't wait for cleanup to complete
// - Non-critical: Failures are logged but don't affect indexing
// - Idempotent: Safe to retry or run multiple times
// - Async: Runs on ops queue to avoid blocking high-throughput indexing queue
func (wc *Context) CleanupStagingWorkflow(ctx workflow.Context, input types.WorkflowCleanupStagingInput) error {
	// Configure activity options for cleanup operations
	// Cleanup is non-critical, so we use shorter timeouts and limited retries
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    1 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3, // Limited retries for cleanup
		},
	})

	// Get all entities that need cleanup
	// Using entities.All() ensures we clean all defined entities
	allEntities := entities.All()

	workflow.GetLogger(ctx).Info("Starting staging cleanup",
		"chainId", wc.ChainID,
		"height", input.Height,
		"numEntities", len(allEntities))

	// Clean each entity's staging table
	// We don't fail on individual entity cleanup failures - just log and continue
	var successCount, failureCount int

	for _, entity := range allEntities {
		var output types.ActivityCleanPromotedDataOutput
		err := workflow.ExecuteActivity(ctx, wc.ActivityContext.CleanPromotedData, types.ActivityCleanPromotedDataInput{
			Entity: entity.String(),
			Height: input.Height,
		}).Get(ctx, &output)

		if err != nil {
			workflow.GetLogger(ctx).Warn("Failed to clean staging data for entity",
				"entity", entity.String(),
				"chainId", wc.ChainID,
				"height", input.Height,
				"error", err)
			failureCount++
			// Continue cleaning other entities despite failure
		} else {
			workflow.GetLogger(ctx).Debug("Successfully cleaned staging data",
				"entity", entity.String(),
				"chainId", wc.ChainID,
				"height", input.Height,
				"durationMs", output.DurationMs)
			successCount++
		}
	}

	workflow.GetLogger(ctx).Info("Completed staging cleanup",
		"chainId", wc.ChainID,
		"height", input.Height,
		"success", successCount,
		"failures", failureCount)

	// Always return nil - cleanup failures are non-critical
	// They're logged above and can be investigated if needed
	return nil
}

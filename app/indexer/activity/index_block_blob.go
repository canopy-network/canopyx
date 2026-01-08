package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/canopy-network/canopy/lib"
	"github.com/canopy-network/canopyx/app/indexer/types"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/rpc"
)

var blobStagingEntities = []string{
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

// IndexBlockFromBlob indexes a block sequentially using the pre-fetched blob data.
func (ac *Context) IndexBlockFromBlob(ctx context.Context, in types.ActivityIndexBlockFromBlobInput) error {
	if in.Blobs == nil || in.Blobs.Current == nil {
		return fmt.Errorf("missing indexer blobs for height %d", in.Height)
	}

	timings := map[string]float64{
		"prepare_index_ms": in.PrepareDurationMs,
		"fetch_block_ms":   in.FetchDurationMs,
	}

	var block lib.BlockResult
	if err := lib.Unmarshal(in.Blobs.Current.Block, &block); err != nil {
		return fmt.Errorf("unmarshal current block: %w", err)
	}

	heightTime := time.UnixMicro(int64(block.BlockHeader.Time))
	indexAtHeight := types.ActivityIndexAtHeight{
		Height:    in.Height,
		BlockTime: heightTime,
	}

	blobCtx := &Context{
		ChainID:              ac.ChainID,
		Logger:               ac.Logger,
		AdminDB:              ac.AdminDB,
		ChainDB:              ac.ChainDB,
		CrossChainDB:         ac.CrossChainDB,
		RPCFactory:           rpc.NewBlobFactory(in.Height, in.Blobs),
		RPCOpts:              ac.RPCOpts,
		ChainClient:          ac.ChainClient,
		RedisClient:          ac.RedisClient,
		WorkerMaxParallelism: ac.WorkerMaxParallelism,
	}

	saveBlockOut, err := blobCtx.SaveBlock(ctx, types.ActivitySaveBlockInput{
		Height: in.Height,
		Block:  &block,
	})
	if err != nil {
		return err
	}
	timings["save_block_ms"] = saveBlockOut.DurationMs

	eventsOut, err := blobCtx.IndexEvents(ctx, indexAtHeight)
	if err != nil {
		return err
	}
	timings["index_events_ms"] = eventsOut.DurationMs

	dataIndexingStart := time.Now()

	txOut, err := blobCtx.IndexTransactions(ctx, indexAtHeight)
	if err != nil {
		return err
	}
	timings["index_transactions_ms"] = txOut.DurationMs

	accountsOut, err := blobCtx.IndexAccounts(ctx, indexAtHeight)
	if err != nil {
		return err
	}
	timings["index_accounts_ms"] = accountsOut.DurationMs

	poolsOut, err := blobCtx.IndexPools(ctx, indexAtHeight)
	if err != nil {
		return err
	}
	timings["index_pools_ms"] = poolsOut.DurationMs

	ordersOut, err := blobCtx.IndexOrders(ctx, indexAtHeight)
	if err != nil {
		return err
	}
	timings["index_orders_ms"] = ordersOut.DurationMs

	pricesOut, err := blobCtx.IndexDexPrices(ctx, indexAtHeight)
	if err != nil {
		return err
	}
	timings["index_dex_prices_ms"] = pricesOut.DurationMs

	paramsOut, err := blobCtx.IndexParams(ctx, indexAtHeight)
	if err != nil {
		return err
	}
	timings["index_params_ms"] = paramsOut.DurationMs

	validatorsOut, err := blobCtx.IndexValidators(ctx, indexAtHeight)
	if err != nil {
		return err
	}
	timings["index_validators_ms"] = validatorsOut.DurationMs

	committeesOut, err := blobCtx.IndexCommittees(ctx, indexAtHeight)
	if err != nil {
		return err
	}
	timings["index_committees_ms"] = committeesOut.DurationMs

	dexBatchOut, err := blobCtx.IndexDexBatch(ctx, indexAtHeight)
	if err != nil {
		return err
	}
	timings["index_dex_batch_ms"] = dexBatchOut.DurationMs

	supplyOut, err := blobCtx.IndexSupply(ctx, indexAtHeight)
	if err != nil {
		return err
	}
	timings["index_supply_ms"] = supplyOut.DurationMs

	timings["data_indexing"] = float64(time.Since(dataIndexingStart).Milliseconds())

	summary := &indexermodels.BlockSummary{
		Height:            in.Height,
		HeightTime:        heightTime,
		TotalTransactions: block.BlockHeader.TotalTxs,

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

		NumAccounts:    accountsOut.NumAccounts,
		NumAccountsNew: accountsOut.NumAccountsNew,

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

		NumOrders:          ordersOut.NumOrders,
		NumOrdersNew:       ordersOut.NumOrdersNew,
		NumOrdersOpen:      ordersOut.NumOrdersOpen,
		NumOrdersFilled:    ordersOut.NumOrdersFilled,
		NumOrdersCancelled: ordersOut.NumOrdersCancelled,

		NumPools:    poolsOut.NumPools,
		NumPoolsNew: poolsOut.NumPoolsNew,

		NumDexPrices: pricesOut.NumPrices,

		NumDexOrders:         dexBatchOut.NumOrders,
		NumDexOrdersFuture:   dexBatchOut.NumOrdersFuture,
		NumDexOrdersLocked:   dexBatchOut.NumOrdersLocked,
		NumDexOrdersComplete: dexBatchOut.NumOrdersComplete,
		NumDexOrdersSuccess:  dexBatchOut.NumOrdersSuccess,
		NumDexOrdersFailed:   dexBatchOut.NumOrdersFailed,

		NumDexDeposits:         dexBatchOut.NumDeposits,
		NumDexDepositsPending:  dexBatchOut.NumDepositsPending,
		NumDexDepositsLocked:   dexBatchOut.NumDepositsLocked,
		NumDexDepositsComplete: dexBatchOut.NumDepositsComplete,

		NumDexWithdrawals:         dexBatchOut.NumWithdrawals,
		NumDexWithdrawalsPending:  dexBatchOut.NumWithdrawalsPending,
		NumDexWithdrawalsLocked:   dexBatchOut.NumWithdrawalsLocked,
		NumDexWithdrawalsComplete: dexBatchOut.NumWithdrawalsComplete,

		NumDexPoolPointsHolders:    poolsOut.NumPoolHolders,
		NumDexPoolPointsHoldersNew: poolsOut.NumPoolHoldersNew,

		ParamsChanged: paramsOut.ParamsChanged,

		NumValidators:                 validatorsOut.NumValidators,
		NumValidatorsNew:              validatorsOut.NumValidatorsNew,
		NumValidatorsActive:           validatorsOut.NumValidatorsActive,
		NumValidatorsPaused:           validatorsOut.NumValidatorsPaused,
		NumValidatorsUnstaking:        validatorsOut.NumValidatorsUnstaking,
		NumValidatorNonSigningInfo:    validatorsOut.NumNonSigningInfos,
		NumValidatorNonSigningInfoNew: validatorsOut.NumNonSigningInfosNew,
		NumValidatorDoubleSigningInfo: validatorsOut.NumDoubleSigningInfos,

		NumCommittees:           committeesOut.NumCommittees,
		NumCommitteesNew:        committeesOut.NumCommitteesNew,
		NumCommitteesSubsidized: committeesOut.NumCommitteesSubsidized,
		NumCommitteesRetired:    committeesOut.NumCommitteesRetired,
		NumCommitteeValidators:  validatorsOut.NumCommitteeValidators,
		NumCommitteePayments:    committeesOut.NumCommitteePayments,

		SupplyChanged:       supplyOut.Changed,
		SupplyTotal:         supplyOut.Total,
		SupplyStaked:        supplyOut.Staked,
		SupplyDelegatedOnly: supplyOut.DelegatedOnly,
	}

	saveSummaryOut, err := blobCtx.SaveBlockSummary(ctx, types.ActivitySaveBlockSummaryInput{
		Height:    in.Height,
		BlockTime: heightTime,
		Summary:   summary,
	})
	if err != nil {
		return err
	}
	timings["save_block_summary_ms"] = saveSummaryOut.DurationMs

	dataReleaseStart := time.Now()
	for _, entity := range blobStagingEntities {
		out, err := blobCtx.PromoteData(ctx, types.ActivityPromoteDataInput{
			Entity: entity,
			Height: in.Height,
		})
		if err != nil {
			return err
		}
		timings[fmt.Sprintf("promote_%s", entity)] = out.DurationMs
	}
	timings["data_release"] = float64(time.Since(dataReleaseStart).Milliseconds())

	totalMs := timings["prepare_index_ms"] +
		timings["fetch_block_ms"] +
		timings["index_events_ms"] +
		timings["data_indexing"] +
		timings["data_release"] +
		timings["save_block_summary_ms"]

	detailBytes, err := json.Marshal(timings)
	if err != nil {
		detailBytes = []byte("{}")
	}

	recordInput := types.ActivityRecordIndexedInput{
		Height:         in.Height,
		IndexingTimeMs: totalMs,
		IndexingDetail: string(detailBytes),
	}
	return blobCtx.RecordIndexed(ctx, recordInput)
}

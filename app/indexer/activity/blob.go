package activity

import (
    "context"
    "encoding/hex"
    "fmt"
    "sync"
    "time"

    "github.com/canopy-network/canopy/fsm"
    "github.com/canopy-network/canopy/lib"
    "github.com/canopy-network/canopyx/app/indexer/types"
    indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
    "go.temporal.io/sdk/activity"
    "golang.org/x/sync/errgroup"
)

// IndexBlockProgress represents the progress of the IndexBlockFromBlob activity.
// This is sent via heartbeat to allow monitoring and resumption on retry.
type IndexBlockProgress struct {
    Height          uint64             `json:"height"`
    Stage           string             `json:"stage"`
    StageNum        int                `json:"stage_num"`
    TotalStages     int                `json:"total_stages"`
    PercentComplete float64            `json:"percent_complete"`
    ElapsedMs       float64            `json:"elapsed_ms"`
    Message         string             `json:"message"`
    ParallelTasks   map[string]float64 `json:"parallel_tasks,omitempty"` // task name -> duration in ms (0 if in progress, >0 if complete)
}

// FetchBlobFromRPC fetches the indexer blob payload from the RPC endpoint.
func (ac *Context) FetchBlobFromRPC(ctx context.Context, in types.ActivityFetchBlockInput) (types.ActivityFetchBlobOutput, error) {
    start := time.Now()

    cli, err := ac.rpcClientForHeight(ctx, in.Height)
    if err != nil {
        return types.ActivityFetchBlobOutput{}, err
    }

    blobs, err := cli.Blob(ctx, in.Height)
    if err != nil {
        return types.ActivityFetchBlobOutput{}, err
    }

    durationMs := float64(time.Since(start).Microseconds()) / 1000.0
    return types.ActivityFetchBlobOutput{Blobs: blobs, DurationMs: durationMs}, nil
}

// IndexBlockFromBlob indexes a block sequentially using the pre-fetched blob data.
// This activity reports progress via heartbeat at each stage to enable monitoring
// and allow resumption from the last checkpoint on retry.
func (ac *Context) IndexBlockFromBlob(ctx context.Context, in types.ActivityIndexBlockFromBlobInput) (types.ActivityIndexBlockFromBlobOutput, error) {
    start := time.Now()
    const totalStages = 6

    // Helper to record heartbeat progress
    recordProgress := func(stageNum int, stage, message string) {
        activity.RecordHeartbeat(ctx, IndexBlockProgress{
            Height:          in.Height,
            Stage:           stage,
            StageNum:        stageNum,
            TotalStages:     totalStages,
            PercentComplete: float64(stageNum) / float64(totalStages) * 100,
            ElapsedMs:       float64(time.Since(start).Milliseconds()),
            Message:         message,
        })
    }

    // Stage 1: Fetch blob from RPC
    recordProgress(1, "fetch_blob", "Fetching blob from RPC")

    cli, err := ac.rpcClientForHeight(ctx, in.Height)
    if err != nil {
        return types.ActivityIndexBlockFromBlobOutput{}, err
    }

    blobs, err := cli.Blob(ctx, in.Height)
    if err != nil {
        return types.ActivityIndexBlockFromBlobOutput{}, fmt.Errorf("error reading blob for height %d: %v", in.Height, err)
    }

    if blobs.Current == nil {
        return types.ActivityIndexBlockFromBlobOutput{}, fmt.Errorf("missing indexer blobs for height %d", in.Height)
    }

    timings := map[string]float64{
        "fetch_block_ms":   float64(time.Since(start).Microseconds()) / 1000.0,
        "prepare_index_ms": in.PrepareDurationMs,
    }

    // Stage 2: Decode blobs
    recordProgress(2, "decode_blobs", "Decoding current and previous blobs")

    currentData, err := decodeBlob(blobs.Current)
    if err != nil {
        return types.ActivityIndexBlockFromBlobOutput{}, err
    }
    if currentData.block == nil {
        return types.ActivityIndexBlockFromBlobOutput{}, fmt.Errorf("missing block data for height %d", in.Height)
    }

    var previousData *blobData
    if in.Height > 1 {
        if blobs.Previous == nil {
            return types.ActivityIndexBlockFromBlobOutput{}, fmt.Errorf("missing previous blob for height %d", in.Height)
        }
        previousData, err = decodeBlob(blobs.Previous)
        if err != nil {
            return types.ActivityIndexBlockFromBlobOutput{}, err
        }
    } else {
        previousData = &blobData{}
    }

    heightTime := time.UnixMicro(int64(currentData.block.BlockHeader.Time))

    chainDb, err := ac.GetGlobalDb(ctx)
    if err != nil {
        return types.ActivityIndexBlockFromBlobOutput{}, err
    }

    // Stage 3: Index block result
    recordProgress(3, "index_block", "Saving block result to database")

    saveBlockDuration, err := ac.indexBlockResultFromBlob(ctx, chainDb, currentData)
    if err != nil {
        return types.ActivityIndexBlockFromBlobOutput{}, err
    }
    timings["save_block_ms"] = saveBlockDuration

    // Stage 4: Index events
    recordProgress(4, "index_events", "Indexing block events")

    eventsOut, eventMaps, eventsDuration, err := ac.indexEventsFromBlob(ctx, chainDb, in.Height, heightTime, currentData)
    if err != nil {
        return types.ActivityIndexBlockFromBlobOutput{}, err
    }
    timings["index_events_ms"] = eventsDuration

    // Stage 5: Parallel data indexing with per-task progress tracking
    dataIndexingStart := time.Now()

    // Track parallel task progress
    var tasksMu sync.Mutex
    parallelTasks := make(map[string]float64)
    const totalParallelTasks = 10

    // Helper to mark task as started and report progress
    markTaskStarted := func(taskName string) {
        tasksMu.Lock()
        parallelTasks[taskName] = 0 // 0 = in progress
        completedCount := 0
        for _, dur := range parallelTasks {
            if dur > 0 {
                completedCount++
            }
        }
        tasksCopy := make(map[string]float64, len(parallelTasks))
        for k, v := range parallelTasks {
            tasksCopy[k] = v
        }
        tasksMu.Unlock()

        activity.RecordHeartbeat(ctx, IndexBlockProgress{
            Height:          in.Height,
            Stage:           "parallel_indexing",
            StageNum:        5,
            TotalStages:     totalStages,
            PercentComplete: (4.0 + float64(completedCount)/float64(totalParallelTasks)) / float64(totalStages) * 100,
            ElapsedMs:       float64(time.Since(start).Milliseconds()),
            Message:         fmt.Sprintf("Started %s (%d/%d tasks running)", taskName, len(tasksCopy)-completedCount, totalParallelTasks),
            ParallelTasks:   tasksCopy,
        })
    }

    // Helper to mark task as completed and report progress
    markTaskCompleted := func(taskName string, durationMs float64) {
        tasksMu.Lock()
        parallelTasks[taskName] = durationMs
        completedCount := 0
        for _, dur := range parallelTasks {
            if dur > 0 {
                completedCount++
            }
        }
        tasksCopy := make(map[string]float64, len(parallelTasks))
        for k, v := range parallelTasks {
            tasksCopy[k] = v
        }
        tasksMu.Unlock()

        activity.RecordHeartbeat(ctx, IndexBlockProgress{
            Height:          in.Height,
            Stage:           "parallel_indexing",
            StageNum:        5,
            TotalStages:     totalStages,
            PercentComplete: (4.0 + float64(completedCount)/float64(totalParallelTasks)) / float64(totalStages) * 100,
            ElapsedMs:       float64(time.Since(start).Milliseconds()),
            Message:         fmt.Sprintf("Completed %s in %.1fms (%d/%d done)", taskName, durationMs, completedCount, totalParallelTasks),
            ParallelTasks:   tasksCopy,
        })
    }

    var (
        txOut         types.ActivityIndexTransactionsOutput
        txDuration    float64
        accountsOut   types.ActivityIndexAccountsOutput
        accountsDur   float64
        poolsOut      types.ActivityIndexPoolsOutput
        poolsDur      float64
        ordersOut     types.ActivityIndexOrdersOutput
        ordersDur     float64
        pricesOut     types.ActivityIndexDexPricesOutput
        pricesDur     float64
        paramsOut     types.ActivityIndexParamsOutput
        paramsDur     float64
        validatorsOut types.ActivityIndexValidatorsOutput
        validatorsDur float64
        committeesOut types.ActivityIndexCommitteesOutput
        committeesDur float64
        dexBatchOut   types.ActivityIndexDexBatchOutput
        dexBatchDur   float64
        supplyOut     types.ActivityIndexSupplyOutput
        supplyDur     float64
    )

    group, groupCtx := errgroup.WithContext(ctx)

    group.Go(func() error {
        markTaskStarted("transactions")
        var err error
        txOut, txDuration, err = ac.indexTransactionsFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData)
        if err == nil {
            markTaskCompleted("transactions", txDuration)
        }
        return err
    })
    group.Go(func() error {
        markTaskStarted("accounts")
        var err error
        accountsOut, accountsDur, err = ac.indexAccountsFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData, previousData, eventMaps)
        if err == nil {
            markTaskCompleted("accounts", accountsDur)
        }
        return err
    })
    group.Go(func() error {
        markTaskStarted("pools")
        var err error
        poolsOut, poolsDur, err = ac.indexPoolsFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData, previousData)
        if err == nil {
            markTaskCompleted("pools", poolsDur)
        }
        return err
    })
    group.Go(func() error {
        markTaskStarted("orders")
        var err error
        ordersOut, ordersDur, err = ac.indexOrdersFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData, previousData, eventMaps)
        if err == nil {
            markTaskCompleted("orders", ordersDur)
        }
        return err
    })
    group.Go(func() error {
        markTaskStarted("dex_prices")
        var err error
        pricesOut, pricesDur, err = ac.indexDexPricesFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData, previousData)
        if err == nil {
            markTaskCompleted("dex_prices", pricesDur)
        }
        return err
    })
    group.Go(func() error {
        markTaskStarted("params")
        var err error
        paramsOut, paramsDur, err = ac.indexParamsFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData, previousData)
        if err == nil {
            markTaskCompleted("params", paramsDur)
        }
        return err
    })
    group.Go(func() error {
        markTaskStarted("validators")
        var err error
        validatorsOut, validatorsDur, err = ac.indexValidatorsFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData, previousData, eventMaps)
        if err == nil {
            markTaskCompleted("validators", validatorsDur)
        }
        return err
    })
    group.Go(func() error {
        markTaskStarted("committees")
        var err error
        committeesOut, committeesDur, err = ac.indexCommitteesFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData, previousData)
        if err == nil {
            markTaskCompleted("committees", committeesDur)
        }
        return err
    })
    group.Go(func() error {
        markTaskStarted("dex_batch")
        var err error
        dexBatchOut, dexBatchDur, err = ac.indexDexBatchFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData, previousData, eventMaps)
        if err == nil {
            markTaskCompleted("dex_batch", dexBatchDur)
        }
        return err
    })
    group.Go(func() error {
        markTaskStarted("supply")
        var err error
        supplyOut, supplyDur, err = ac.indexSupplyFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData, previousData)
        if err == nil {
            markTaskCompleted("supply", supplyDur)
        }
        return err
    })

    if err := group.Wait(); err != nil {
        return types.ActivityIndexBlockFromBlobOutput{}, err
    }

    timings["index_transactions_ms"] = txDuration
    timings["index_accounts_ms"] = accountsDur
    timings["index_pools_ms"] = poolsDur
    timings["index_orders_ms"] = ordersDur
    timings["index_dex_prices_ms"] = pricesDur
    timings["index_params_ms"] = paramsDur
    timings["index_validators_ms"] = validatorsDur
    timings["index_committees_ms"] = committeesDur
    timings["index_dex_batch_ms"] = dexBatchDur
    timings["index_supply_ms"] = supplyDur

    timings["data_indexing"] = float64(time.Since(dataIndexingStart).Milliseconds())

    // Stage 6: Save block summary
    recordProgress(6, "save_summary", "Building and saving block summary")

    summary := buildBlockSummary(in.Height, heightTime, currentData.block.BlockHeader.TotalTxs, txOut, accountsOut, eventsOut, ordersOut, poolsOut, pricesOut, paramsOut, validatorsOut, committeesOut, dexBatchOut, supplyOut)

    summaryDuration, err := ac.saveBlockSummaryFromBlob(ctx, chainDb, summary)
    if err != nil {
        return types.ActivityIndexBlockFromBlobOutput{}, err
    }
    timings["save_block_summary_ms"] = summaryDuration

    totalMs := timings["prepare_index_ms"] +
            timings["fetch_block_ms"] +
            timings["index_events_ms"] +
            timings["data_indexing"] +
            timings["save_block_summary_ms"]

    return types.ActivityIndexBlockFromBlobOutput{
        Height:         in.Height,
        IndexingTimeMs: totalMs,
        IndexingDetail: timings,
        // Pass block info to avoid re-querying ClickHouse in RecordIndexed
        BlockHash:            hex.EncodeToString(currentData.block.BlockHeader.Hash),
        BlockTime:            heightTime,
        BlockProposerAddress: hex.EncodeToString(currentData.block.BlockHeader.ProposerAddress),
    }, nil
}

func buildBlockSummary(
        height uint64,
        heightTime time.Time,
        totalTxs uint64,
        txOut types.ActivityIndexTransactionsOutput,
        accountsOut types.ActivityIndexAccountsOutput,
        eventsOut types.ActivityIndexEventsOutput,
        ordersOut types.ActivityIndexOrdersOutput,
        poolsOut types.ActivityIndexPoolsOutput,
        pricesOut types.ActivityIndexDexPricesOutput,
        paramsOut types.ActivityIndexParamsOutput,
        validatorsOut types.ActivityIndexValidatorsOutput,
        committeesOut types.ActivityIndexCommitteesOutput,
        dexBatchOut types.ActivityIndexDexBatchOutput,
        supplyOut types.ActivityIndexSupplyOutput,
) *indexermodels.BlockSummary {
    return &indexermodels.BlockSummary{
        Height:            height,
        HeightTime:        heightTime,
        TotalTransactions: totalTxs,

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

        NumEvents: eventsOut.NumEvents,
        // todo: use string(lib.<eventType>)
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
}

type blobData struct {
    block                *lib.BlockResult
    accounts             []*fsm.Account
    pools                []*fsm.Pool
    validators           []*fsm.Validator
    dexPrices            []*lib.DexPrice
    nonSigners           []*fsm.NonSigner
    doubleSigners        []*lib.DoubleSigner
    orders               []*lib.SellOrder
    params               *fsm.Params
    dexBatches           []*lib.DexBatch
    nextDexBatches       []*lib.DexBatch
    committeesData       []*lib.CommitteeData
    subsidizedCommittees []uint64
    retiredCommittees    []uint64
    supply               *fsm.Supply
}

func decodeBlob(blob *fsm.IndexerBlob) (*blobData, error) {
    if blob == nil {
        return nil, fmt.Errorf("blob is nil")
    }

    out := &blobData{
        subsidizedCommittees: blob.SubsidizedCommittees,
        retiredCommittees:    blob.RetiredCommittees,
    }

    if len(blob.Block) > 0 {
        var block lib.BlockResult
        if err := lib.Unmarshal(blob.Block, &block); err != nil {
            return nil, fmt.Errorf("unmarshal block: %w", err)
        }
        out.block = &block
    }

    if len(blob.Params) > 0 {
        var params fsm.Params
        if err := lib.Unmarshal(blob.Params, &params); err != nil {
            return nil, fmt.Errorf("unmarshal params: %w", err)
        }
        out.params = &params
    }

    if len(blob.Supply) > 0 {
        var supply fsm.Supply
        if err := lib.Unmarshal(blob.Supply, &supply); err != nil {
            return nil, fmt.Errorf("unmarshal supply: %w", err)
        }
        out.supply = &supply
    }

    if len(blob.CommitteesData) > 0 {
        var committees lib.CommitteesData
        if err := lib.Unmarshal(blob.CommitteesData, &committees); err != nil {
            return nil, fmt.Errorf("unmarshal committees data: %w", err)
        }
        out.committeesData = committees.List
    }

    if blob.Accounts != nil {
        out.accounts = make([]*fsm.Account, 0, len(blob.Accounts))
        for _, raw := range blob.Accounts {
            var acc fsm.Account
            if err := lib.Unmarshal(raw, &acc); err != nil {
                return nil, fmt.Errorf("unmarshal account: %w", err)
            }
            out.accounts = append(out.accounts, &acc)
        }
    }

    if blob.Pools != nil {
        out.pools = make([]*fsm.Pool, 0, len(blob.Pools))
        for _, raw := range blob.Pools {
            var pool fsm.Pool
            if err := lib.Unmarshal(raw, &pool); err != nil {
                return nil, fmt.Errorf("unmarshal pool: %w", err)
            }
            out.pools = append(out.pools, &pool)
        }
    }

    if blob.Validators != nil {
        out.validators = make([]*fsm.Validator, 0, len(blob.Validators))
        for _, raw := range blob.Validators {
            var validator fsm.Validator
            if err := lib.Unmarshal(raw, &validator); err != nil {
                return nil, fmt.Errorf("unmarshal validator: %w", err)
            }
            out.validators = append(out.validators, &validator)
        }
    }

    if blob.DexPrices != nil {
        out.dexPrices = make([]*lib.DexPrice, 0, len(blob.DexPrices))
        for _, raw := range blob.DexPrices {
            var price lib.DexPrice
            if err := lib.Unmarshal(raw, &price); err != nil {
                return nil, fmt.Errorf("unmarshal dex price: %w", err)
            }
            out.dexPrices = append(out.dexPrices, &price)
        }
    }

    if blob.NonSigners != nil {
        out.nonSigners = make([]*fsm.NonSigner, 0, len(blob.NonSigners))
        for _, raw := range blob.NonSigners {
            var nonSigner fsm.NonSigner
            if err := lib.Unmarshal(raw, &nonSigner); err != nil {
                return nil, fmt.Errorf("unmarshal non-signer: %w", err)
            }
            out.nonSigners = append(out.nonSigners, &nonSigner)
        }
    }

    if blob.DoubleSigners != nil {
        out.doubleSigners = make([]*lib.DoubleSigner, 0, len(blob.DoubleSigners))
        for _, raw := range blob.DoubleSigners {
            var doubleSigner lib.DoubleSigner
            if err := lib.Unmarshal(raw, &doubleSigner); err != nil {
                return nil, fmt.Errorf("unmarshal double-signer: %w", err)
            }
            out.doubleSigners = append(out.doubleSigners, &doubleSigner)
        }
    }

    if len(blob.Orders) > 0 {
        var orderBooks lib.OrderBooks
        if err := lib.Unmarshal(blob.Orders, &orderBooks); err != nil {
            return nil, fmt.Errorf("unmarshal orders: %w", err)
        }
        var orders []*lib.SellOrder
        for _, book := range orderBooks.OrderBooks {
            orders = append(orders, book.Orders...)
        }
        out.orders = orders
    }

    if blob.DexBatches != nil {
        out.dexBatches = make([]*lib.DexBatch, 0, len(blob.DexBatches))
        for _, raw := range blob.DexBatches {
            var batch lib.DexBatch
            if err := lib.Unmarshal(raw, &batch); err != nil {
                return nil, fmt.Errorf("unmarshal dex batch: %w", err)
            }
            out.dexBatches = append(out.dexBatches, &batch)
        }
    }

    if blob.NextDexBatches != nil {
        out.nextDexBatches = make([]*lib.DexBatch, 0, len(blob.NextDexBatches))
        for _, raw := range blob.NextDexBatches {
            var batch lib.DexBatch
            if err := lib.Unmarshal(raw, &batch); err != nil {
                return nil, fmt.Errorf("unmarshal next dex batch: %w", err)
            }
            out.nextDexBatches = append(out.nextDexBatches, &batch)
        }
    }

    return out, nil
}

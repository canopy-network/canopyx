package activity

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/canopy-network/canopy/fsm"
	"github.com/canopy-network/canopy/lib"
	"github.com/canopy-network/canopyx/app/indexer/types"
	chainstore "github.com/canopy-network/canopyx/pkg/db/chain"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/db/transform"
	"golang.org/x/sync/errgroup"
)

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
func (ac *Context) IndexBlockFromBlob(ctx context.Context, in types.ActivityIndexBlockFromBlobInput) error {
	if in.Blobs == nil || in.Blobs.Current == nil {
		return fmt.Errorf("missing indexer blobs for height %d", in.Height)
	}

	timings := map[string]float64{
		"prepare_index_ms": in.PrepareDurationMs,
		"fetch_block_ms":   in.FetchDurationMs,
	}

	currentData, err := decodeBlob(in.Blobs.Current)
	if err != nil {
		return err
	}
	if currentData.block == nil {
		return fmt.Errorf("missing block data for height %d", in.Height)
	}

	var previousData *blobData
	if in.Height > 1 {
		if in.Blobs.Previous == nil {
			return fmt.Errorf("missing previous blob for height %d", in.Height)
		}
		previousData, err = decodeBlob(in.Blobs.Previous)
		if err != nil {
			return err
		}
	} else {
		previousData = &blobData{}
	}

	heightTime := time.UnixMicro(int64(currentData.block.BlockHeader.Time))

	chainDb, err := ac.GetChainDb(ctx, ac.ChainID)
	if err != nil {
		return err
	}

	saveBlockDuration, err := ac.indexBlockResultFromBlob(ctx, chainDb, currentData)
	if err != nil {
		return err
	}
	timings["save_block_ms"] = saveBlockDuration

	eventsOut, eventMaps, eventsDuration, err := ac.indexEventsFromBlob(ctx, chainDb, in.Height, heightTime, currentData)
	if err != nil {
		return err
	}
	timings["index_events_ms"] = eventsDuration

	dataIndexingStart := time.Now()

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
		var err error
		txOut, txDuration, err = ac.indexTransactionsFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData)
		return err
	})
	group.Go(func() error {
		var err error
		accountsOut, accountsDur, err = ac.indexAccountsFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData, previousData, eventMaps)
		return err
	})
	group.Go(func() error {
		var err error
		poolsOut, poolsDur, err = ac.indexPoolsFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData, previousData)
		return err
	})
	group.Go(func() error {
		var err error
		ordersOut, ordersDur, err = ac.indexOrdersFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData, previousData, eventMaps)
		return err
	})
	group.Go(func() error {
		var err error
		pricesOut, pricesDur, err = ac.indexDexPricesFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData, previousData)
		return err
	})
	group.Go(func() error {
		var err error
		paramsOut, paramsDur, err = ac.indexParamsFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData, previousData)
		return err
	})
	group.Go(func() error {
		var err error
		validatorsOut, validatorsDur, err = ac.indexValidatorsFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData, previousData, eventMaps)
		return err
	})
	group.Go(func() error {
		var err error
		committeesOut, committeesDur, err = ac.indexCommitteesFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData, previousData)
		return err
	})
	group.Go(func() error {
		var err error
		dexBatchOut, dexBatchDur, err = ac.indexDexBatchFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData, previousData, eventMaps)
		return err
	})
	group.Go(func() error {
		var err error
		supplyOut, supplyDur, err = ac.indexSupplyFromBlob(groupCtx, chainDb, in.Height, heightTime, currentData, previousData)
		return err
	})

	if err := group.Wait(); err != nil {
		return err
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

	summary := buildBlockSummary(in.Height, heightTime, currentData.block.BlockHeader.TotalTxs, txOut, accountsOut, eventsOut, ordersOut, poolsOut, pricesOut, paramsOut, validatorsOut, committeesOut, dexBatchOut, supplyOut)

	summaryDuration, err := ac.saveBlockSummaryFromBlob(ctx, chainDb, summary)
	if err != nil {
		return err
	}
	timings["save_block_summary_ms"] = summaryDuration

	totalMs := timings["prepare_index_ms"] +
		timings["fetch_block_ms"] +
		timings["index_events_ms"] +
		timings["data_indexing"] +
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
	return ac.RecordIndexed(ctx, recordInput)
}

type blobEventMaps struct {
	reward            map[string]uint64
	slash             map[string]uint64
	pause             map[string]struct{}
	beginUnstaking    map[string]struct{}
	validatorReward   map[string]struct{}
	validatorSlash    map[string]struct{}
	orderBookSwap     map[string]struct{}
	dexSwap           map[string]*chainstore.EventDexBatch
	dexDeposit        map[string]*chainstore.EventDexBatch
	dexWithdrawal     map[string]*chainstore.EventDexBatch
	eventCountsByType map[string]uint32
}

func (ac *Context) indexBlockResultFromBlob(ctx context.Context, chainDb chainstore.Store, currentData *blobData) (float64, error) {
	start := time.Now()
	block := transform.Block(currentData.block)
	if err := chainDb.InsertBlocksProduction(ctx, block); err != nil {
		return 0, err
	}
	return float64(time.Since(start).Microseconds()) / 1000.0, nil
}

func (ac *Context) indexEventsFromBlob(ctx context.Context, chainDb chainstore.Store, height uint64, heightTime time.Time, currentData *blobData) (types.ActivityIndexEventsOutput, *blobEventMaps, float64, error) {
	start := time.Now()
	events := make([]*indexermodels.Event, 0, len(currentData.block.Events))
	eventCountsByType := make(map[string]uint32)
	rewardEvents := make(map[string]uint64)
	slashEvents := make(map[string]uint64)
	pauseEvents := make(map[string]struct{})
	beginUnstakingEvents := make(map[string]struct{})
	validatorRewardEvents := make(map[string]struct{})
	validatorSlashEvents := make(map[string]struct{})
	orderBookSwapEvents := make(map[string]struct{})
	dexSwapEvents := make(map[string]*chainstore.EventDexBatch)
	dexDepositEvents := make(map[string]*chainstore.EventDexBatch)
	dexWithdrawalEvents := make(map[string]*chainstore.EventDexBatch)

	for _, rpcEvent := range currentData.block.Events {
		event, err := transform.Event(rpcEvent)
		if err != nil {
			return types.ActivityIndexEventsOutput{}, nil, 0, fmt.Errorf("convert event at height %d, type %s: %w", height, rpcEvent.EventType, err)
		}
		event.HeightTime = heightTime
		events = append(events, event)
		eventCountsByType[event.EventType]++

		switch event.EventType {
		case string(lib.EventTypeReward):
			rewardEvents[event.Address] = event.Amount
			validatorRewardEvents[event.Address] = struct{}{}
		case string(lib.EventTypeSlash):
			slashEvents[event.Address] = event.Amount
			validatorSlashEvents[event.Address] = struct{}{}
		case string(lib.EventTypeAutoPause):
			pauseEvents[event.Address] = struct{}{}
		case string(lib.EventTypeAutoBeginUnstaking):
			beginUnstakingEvents[event.Address] = struct{}{}
		case string(lib.EventTypeOrderBookSwap):
			if event.OrderID != "" {
				orderBookSwapEvents[event.OrderID] = struct{}{}
			}
		case string(lib.EventTypeDexSwap):
			if event.OrderID != "" {
				dexSwapEvents[event.OrderID] = &chainstore.EventDexBatch{
					Height:       height,
					EventType:    event.EventType,
					OrderID:      event.OrderID,
					Success:      event.Success,
					SoldAmount:   event.SoldAmount,
					BoughtAmount: event.BoughtAmount,
					LocalOrigin:  event.LocalOrigin,
				}
			}
		case string(lib.EventTypeDexLiquidityDeposit):
			if event.OrderID != "" {
				dexDepositEvents[event.OrderID] = &chainstore.EventDexBatch{
					Height:         height,
					EventType:      event.EventType,
					OrderID:        event.OrderID,
					LocalOrigin:    event.LocalOrigin,
					PointsReceived: event.PointsReceived,
				}
			}
		case string(lib.EventTypeDexLiquidityWithdraw):
			if event.OrderID != "" {
				dexWithdrawalEvents[event.OrderID] = &chainstore.EventDexBatch{
					Height:       height,
					EventType:    event.EventType,
					OrderID:      event.OrderID,
					LocalAmount:  event.LocalAmount,
					RemoteAmount: event.RemoteAmount,
					PointsBurned: event.PointsBurned,
				}
			}
		}
	}

	if len(events) > 0 {
		if err := chainDb.InsertEventsProduction(ctx, events); err != nil {
			return types.ActivityIndexEventsOutput{}, nil, 0, err
		}
	}

	eventsOut := types.ActivityIndexEventsOutput{
		NumEvents:         uint32(len(events)),
		EventCountsByType: eventCountsByType,
	}
	eventMaps := &blobEventMaps{
		reward:            rewardEvents,
		slash:             slashEvents,
		pause:             pauseEvents,
		beginUnstaking:    beginUnstakingEvents,
		validatorReward:   validatorRewardEvents,
		validatorSlash:    validatorSlashEvents,
		orderBookSwap:     orderBookSwapEvents,
		dexSwap:           dexSwapEvents,
		dexDeposit:        dexDepositEvents,
		dexWithdrawal:     dexWithdrawalEvents,
		eventCountsByType: eventCountsByType,
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return eventsOut, eventMaps, durationMs, nil
}

func (ac *Context) indexTransactionsFromBlob(ctx context.Context, chainDb chainstore.Store, height uint64, heightTime time.Time, currentData *blobData) (types.ActivityIndexTransactionsOutput, float64, error) {
	start := time.Now()
	txCountsByType := make(map[string]uint32)
	txs := make([]*indexermodels.Transaction, 0, len(currentData.block.Transactions))
	for _, rpcTx := range currentData.block.Transactions {
		tx, err := transform.Transaction(rpcTx)
		if err != nil {
			return types.ActivityIndexTransactionsOutput{}, 0, fmt.Errorf("convert transaction at height %d, hash %s: %w", height, rpcTx.TxHash, err)
		}
		tx.HeightTime = heightTime
		txs = append(txs, tx)
		txCountsByType[tx.MessageType]++
	}
	if len(txs) > 0 {
		if err := chainDb.InsertTransactionsProduction(ctx, txs); err != nil {
			return types.ActivityIndexTransactionsOutput{}, 0, err
		}
	}
	out := types.ActivityIndexTransactionsOutput{
		NumTxs:         uint32(len(txs)),
		TxCountsByType: txCountsByType,
	}
	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return out, durationMs, nil
}

func (ac *Context) indexAccountsFromBlob(ctx context.Context, chainDb chainstore.Store, height uint64, heightTime time.Time, currentData *blobData, previousData *blobData, events *blobEventMaps) (types.ActivityIndexAccountsOutput, float64, error) {
	start := time.Now()
	prevAccountMap := make(map[string]uint64, len(previousData.accounts))
	for _, acc := range previousData.accounts {
		prevAccountMap[hex.EncodeToString(acc.Address)] = acc.Amount
	}
	changedAccounts := make([]*indexermodels.Account, 0)
	var numAccountsNew uint32
	for _, curr := range currentData.accounts {
		addrHex := hex.EncodeToString(curr.Address)
		prevAmount, existed := prevAccountMap[addrHex]
		rewardAmount := events.reward[addrHex]
		slashAmount := events.slash[addrHex]

		if curr.Amount != prevAmount || rewardAmount > 0 || slashAmount > 0 {
			changedAccounts = append(changedAccounts, &indexermodels.Account{
				Address:    addrHex,
				Amount:     curr.Amount,
				Rewards:    rewardAmount,
				Slashes:    slashAmount,
				Height:     height,
				HeightTime: heightTime,
			})
			if !existed {
				numAccountsNew++
			}
		}
	}
	if len(changedAccounts) > 0 {
		if err := chainDb.InsertAccountsProduction(ctx, changedAccounts); err != nil {
			return types.ActivityIndexAccountsOutput{}, 0, err
		}
	}
	out := types.ActivityIndexAccountsOutput{
		NumAccounts:    uint32(len(changedAccounts)),
		NumAccountsNew: numAccountsNew,
	}
	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return out, durationMs, nil
}

func (ac *Context) indexPoolsFromBlob(ctx context.Context, chainDb chainstore.Store, height uint64, heightTime time.Time, currentData *blobData, previousData *blobData) (types.ActivityIndexPoolsOutput, float64, error) {
	start := time.Now()
	prevPoolsMap := make(map[uint32]*indexermodels.Pool, len(previousData.pools))
	for _, prevPool := range previousData.pools {
		dbPool := transform.Pool(prevPool, height-1)
		prevPoolsMap[dbPool.PoolID] = dbPool
	}

	pools := make([]*indexermodels.Pool, 0, len(currentData.pools))
	var numPoolsNew uint32
	for _, rpcPool := range currentData.pools {
		pool := transform.Pool(rpcPool, height)
		pool.HeightTime = heightTime
		pool.CalculatePoolIDs()

		if prevPool, exists := prevPoolsMap[pool.PoolID]; exists {
			pool.AmountDelta = int64(pool.Amount) - int64(prevPool.Amount)
			pool.TotalPointsDelta = int64(pool.TotalPoints) - int64(prevPool.TotalPoints)
			pool.LPCountDelta = int16(pool.LPCount) - int16(prevPool.LPCount)
		} else {
			pool.AmountDelta = 0
			pool.TotalPointsDelta = 0
			pool.LPCountDelta = 0
			numPoolsNew++
		}
		pools = append(pools, pool)
	}

	prevHolderMap := make(map[string]uint64)
	prevPoolTotalPoints := make(map[uint64]uint64)
	for _, pool := range previousData.pools {
		prevPoolTotalPoints[pool.Id] = pool.TotalPoolPoints
		for _, pointEntry := range pool.Points {
			address := hex.EncodeToString(pointEntry.Address)
			key := fmt.Sprintf("%d:%s", pool.Id, address)
			prevHolderMap[key] = pointEntry.Points
		}
	}

	changedHolders := make([]*indexermodels.PoolPointsByHolder, 0)
	var numPoolHoldersNew uint32
	for _, rpcPool := range currentData.pools {
		if len(rpcPool.Points) == 0 {
			continue
		}
		chainID := indexermodels.ExtractChainIDFromPoolID(rpcPool.Id)
		liquidityPoolID := indexermodels.LiquidityPoolAddend + chainID

		prevTotalPoints, poolExisted := prevPoolTotalPoints[rpcPool.Id]
		poolTotalPointsChanged := !poolExisted || rpcPool.TotalPoolPoints != prevTotalPoints

		for _, pointEntry := range rpcPool.Points {
			address := hex.EncodeToString(pointEntry.Address)
			key := fmt.Sprintf("%d:%s", rpcPool.Id, address)
			prevPoints, holderExisted := prevHolderMap[key]

			shouldSnapshot := !holderExisted || pointEntry.Points != prevPoints || poolTotalPointsChanged
			if shouldSnapshot {
				if !holderExisted {
					numPoolHoldersNew++
				}
				holder := &indexermodels.PoolPointsByHolder{
					Address:             address,
					PoolID:              uint32(rpcPool.Id),
					Height:              height,
					HeightTime:          heightTime,
					Committee:           uint16(chainID),
					Points:              pointEntry.Points,
					LiquidityPoolPoints: rpcPool.TotalPoolPoints,
					LiquidityPoolID:     uint32(liquidityPoolID),
					PoolAmount:          rpcPool.Amount,
				}
				changedHolders = append(changedHolders, holder)
			}
		}
	}

	if len(pools) > 0 {
		if err := chainDb.InsertPoolsProduction(ctx, pools); err != nil {
			return types.ActivityIndexPoolsOutput{}, 0, err
		}
	}
	if len(changedHolders) > 0 {
		if err := chainDb.InsertPoolPointsByHolderProduction(ctx, changedHolders); err != nil {
			return types.ActivityIndexPoolsOutput{}, 0, err
		}
	}
	out := types.ActivityIndexPoolsOutput{
		NumPools:          uint32(len(pools)),
		NumPoolsNew:       numPoolsNew,
		NumPoolHolders:    uint32(len(changedHolders)),
		NumPoolHoldersNew: numPoolHoldersNew,
	}
	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return out, durationMs, nil
}

func (ac *Context) indexOrdersFromBlob(ctx context.Context, chainDb chainstore.Store, height uint64, heightTime time.Time, currentData *blobData, previousData *blobData, events *blobEventMaps) (types.ActivityIndexOrdersOutput, float64, error) {
	start := time.Now()
	prevOrdersMap := make(map[string]*lib.SellOrder, len(previousData.orders))
	for _, order := range previousData.orders {
		prevOrdersMap[hex.EncodeToString(order.Id)] = order
	}
	currentOrderIDs := make(map[string]struct{}, len(currentData.orders))
	changedOrders := make([]*indexermodels.Order, 0)
	var numOrdersNew, numOrdersOpen, numOrdersFilled uint32

	for _, curr := range currentData.orders {
		currID := hex.EncodeToString(curr.Id)
		currentOrderIDs[currID] = struct{}{}
		prev := prevOrdersMap[currID]
		_, hasSwapEvent := events.orderBookSwap[currID]

		if prev == nil {
			numOrdersNew++
		}

		if hasSwapEvent {
			numOrdersFilled++
		} else {
			numOrdersOpen++
		}

		hasChanged := prev == nil || hasSwapEvent || orderStateChanged(prev, curr)
		if hasChanged {
			status := indexermodels.OrderStatusOpen
			if hasSwapEvent {
				status = indexermodels.OrderStatusComplete
			}
			changedOrders = append(changedOrders, &indexermodels.Order{
				OrderID:              currID,
				Height:               height,
				HeightTime:           heightTime,
				Committee:            uint16(curr.Committee),
				Data:                 hex.EncodeToString(curr.Data),
				AmountForSale:        curr.AmountForSale,
				RequestedAmount:      curr.RequestedAmount,
				SellerReceiveAddress: hex.EncodeToString(curr.SellerReceiveAddress),
				BuyerSendAddress:     hex.EncodeToString(curr.BuyerSendAddress),
				BuyerReceiveAddress:  hex.EncodeToString(curr.BuyerReceiveAddress),
				BuyerChainDeadline:   curr.BuyerChainDeadline,
				SellersSendAddress:   hex.EncodeToString(curr.SellersSendAddress),
				Status:               status,
			})
		}
	}

	var numOrdersCancelled uint32
	for prevID, prevOrder := range prevOrdersMap {
		if _, existsNow := currentOrderIDs[prevID]; existsNow {
			continue
		}
		if _, hasSwapEvent := events.orderBookSwap[prevID]; hasSwapEvent {
			continue
		}
		numOrdersCancelled++
		changedOrders = append(changedOrders, &indexermodels.Order{
			OrderID:              prevID,
			Height:               height,
			HeightTime:           heightTime,
			Committee:            uint16(prevOrder.Committee),
			Data:                 hex.EncodeToString(prevOrder.Data),
			AmountForSale:        prevOrder.AmountForSale,
			RequestedAmount:      prevOrder.RequestedAmount,
			SellerReceiveAddress: hex.EncodeToString(prevOrder.SellerReceiveAddress),
			BuyerSendAddress:     hex.EncodeToString(prevOrder.BuyerSendAddress),
			BuyerReceiveAddress:  hex.EncodeToString(prevOrder.BuyerReceiveAddress),
			BuyerChainDeadline:   prevOrder.BuyerChainDeadline,
			SellersSendAddress:   hex.EncodeToString(prevOrder.SellersSendAddress),
			Status:               indexermodels.OrderStatusCanceled,
		})
	}

	if len(changedOrders) > 0 {
		if err := chainDb.InsertOrdersProduction(ctx, changedOrders); err != nil {
			return types.ActivityIndexOrdersOutput{}, 0, err
		}
	}
	out := types.ActivityIndexOrdersOutput{
		NumOrders:          uint32(len(changedOrders)),
		NumOrdersNew:       numOrdersNew,
		NumOrdersOpen:      numOrdersOpen,
		NumOrdersFilled:    numOrdersFilled,
		NumOrdersCancelled: numOrdersCancelled,
	}
	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return out, durationMs, nil
}

func (ac *Context) indexDexPricesFromBlob(ctx context.Context, chainDb chainstore.Store, height uint64, heightTime time.Time, currentData *blobData, previousData *blobData) (types.ActivityIndexDexPricesOutput, float64, error) {
	start := time.Now()
	prevDexPrices := make(map[string]*indexermodels.DexPrice, len(previousData.dexPrices))
	for _, price := range previousData.dexPrices {
		dbPrice := transform.DexPrice(price)
		key := transform.DexPriceKey(dbPrice.LocalChainID, dbPrice.RemoteChainID)
		prevDexPrices[key] = dbPrice
	}
	dbPrices := make([]*indexermodels.DexPrice, 0, len(currentData.dexPrices))
	for _, rpcPrice := range currentData.dexPrices {
		dbPrice := transform.DexPrice(rpcPrice)
		dbPrice.Height = height
		dbPrice.HeightTime = heightTime
		key := transform.DexPriceKey(dbPrice.LocalChainID, dbPrice.RemoteChainID)

		priceChanged := true
		if priceH1, exists := prevDexPrices[key]; exists {
			dbPrice.PriceDelta = int64(dbPrice.PriceE6) - int64(priceH1.PriceE6)
			dbPrice.LocalPoolDelta = int64(dbPrice.LocalPool) - int64(priceH1.LocalPool)
			dbPrice.RemotePoolDelta = int64(dbPrice.RemotePool) - int64(priceH1.RemotePool)

			if dbPrice.PriceE6 == priceH1.PriceE6 &&
				dbPrice.LocalPool == priceH1.LocalPool &&
				dbPrice.RemotePool == priceH1.RemotePool {
				priceChanged = false
			}
		} else {
			dbPrice.PriceDelta = 0
			dbPrice.LocalPoolDelta = 0
			dbPrice.RemotePoolDelta = 0
		}

		if priceChanged {
			dbPrices = append(dbPrices, dbPrice)
		}
	}
	if len(dbPrices) > 0 {
		if err := chainDb.InsertDexPricesProduction(ctx, dbPrices); err != nil {
			return types.ActivityIndexDexPricesOutput{}, 0, err
		}
	}
	out := types.ActivityIndexDexPricesOutput{
		NumPrices: uint32(len(dbPrices)),
	}
	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return out, durationMs, nil
}

func (ac *Context) indexParamsFromBlob(ctx context.Context, chainDb chainstore.Store, height uint64, heightTime time.Time, currentData *blobData, previousData *blobData) (types.ActivityIndexParamsOutput, float64, error) {
	start := time.Now()
	currentParams := currentData.params
	if currentParams == nil {
		currentParams = &fsm.Params{}
	}
	currentParamsEntity := convertRpcParamsToEntity(currentParams, height, heightTime)
	var paramsChanged bool
	if height == 1 {
		paramsChanged = true
	} else {
		prevParams := previousData.params
		if prevParams == nil {
			prevParams = &fsm.Params{}
		}
		prevParamsEntity := convertRpcParamsToEntity(prevParams, height-1, time.Time{})
		paramsChanged = !paramsEqual(prevParamsEntity, currentParamsEntity)
	}
	if paramsChanged {
		if err := chainDb.InsertParamsProduction(ctx, currentParamsEntity); err != nil {
			return types.ActivityIndexParamsOutput{}, 0, err
		}
	}
	out := types.ActivityIndexParamsOutput{
		ParamsChanged: paramsChanged,
	}
	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return out, durationMs, nil
}

func (ac *Context) indexValidatorsFromBlob(ctx context.Context, chainDb chainstore.Store, height uint64, heightTime time.Time, currentData *blobData, previousData *blobData, events *blobEventMaps) (types.ActivityIndexValidatorsOutput, float64, error) {
	start := time.Now()
	subsidizedMap := make(map[uint64]bool, len(currentData.subsidizedCommittees))
	for _, id := range currentData.subsidizedCommittees {
		subsidizedMap[id] = true
	}
	retiredMap := make(map[uint64]bool, len(currentData.retiredCommittees))
	for _, id := range currentData.retiredCommittees {
		retiredMap[id] = true
	}

	prevValidatorMap := make(map[string]*fsm.Validator, len(previousData.validators))
	for _, val := range previousData.validators {
		prevValidatorMap[hex.EncodeToString(val.Address)] = val
	}

	prevNonSignerMap := make(map[string]*fsm.NonSigner, len(previousData.nonSigners))
	for _, ns := range previousData.nonSigners {
		addrHex := hex.EncodeToString(ns.Address)
		prevNonSignerMap[addrHex] = ns
	}

	currentNonSignerMap := make(map[string]*fsm.NonSigner, len(currentData.nonSigners))
	for _, ns := range currentData.nonSigners {
		addrHex := hex.EncodeToString(ns.Address)
		currentNonSignerMap[addrHex] = ns
	}

	changedValidators := make([]*indexermodels.Validator, 0)
	var numValidatorsNew, numValidatorsActive, numValidatorsPaused, numValidatorsUnstaking uint32

	for _, curr := range currentData.validators {
		addrHex := hex.EncodeToString(curr.Address)
		prev := prevValidatorMap[addrHex]

		changed := false
		hasEvent := false

		if _, ok := events.pause[addrHex]; ok {
			changed = true
			hasEvent = true
		}
		if _, ok := events.beginUnstaking[addrHex]; ok {
			changed = true
			hasEvent = true
		}
		if _, ok := events.validatorSlash[addrHex]; ok {
			changed = true
			hasEvent = true
		}
		if _, ok := events.validatorReward[addrHex]; ok {
			changed = true
			hasEvent = true
		}

		if prev == nil {
			changed = true
			numValidatorsNew++
		} else if !hasEvent {
			if curr.StakedAmount != prev.StakedAmount ||
				hex.EncodeToString(curr.PublicKey) != hex.EncodeToString(prev.PublicKey) ||
				curr.NetAddress != prev.NetAddress ||
				curr.MaxPausedHeight != prev.MaxPausedHeight ||
				curr.UnstakingHeight != prev.UnstakingHeight ||
				hex.EncodeToString(curr.Output) != hex.EncodeToString(prev.Output) ||
				curr.Delegate != prev.Delegate ||
				curr.Compound != prev.Compound ||
				!equalCommittees(curr.Committees, prev.Committees) {
				changed = true
			}
		}

		if changed {
			val := &indexermodels.Validator{
				Address:         addrHex,
				PublicKey:       hex.EncodeToString(curr.PublicKey),
				NetAddress:      curr.NetAddress,
				StakedAmount:    curr.StakedAmount,
				Output:          hex.EncodeToString(curr.Output),
				Committees:      curr.Committees,
				MaxPausedHeight: curr.MaxPausedHeight,
				UnstakingHeight: curr.UnstakingHeight,
				Delegate:        curr.Delegate,
				Compound:        curr.Compound,
				Height:          height,
				HeightTime:      heightTime,
			}
			val.Status = val.DeriveStatus()
			changedValidators = append(changedValidators, val)
		}

		if curr.UnstakingHeight > 0 {
			numValidatorsUnstaking++
		} else if curr.MaxPausedHeight > 0 {
			numValidatorsPaused++
		} else {
			numValidatorsActive++
		}
	}

	changedNonSigningInfos := make([]*indexermodels.ValidatorNonSigningInfo, 0)
	var numNonSigningInfosNew uint32
	for _, curr := range currentData.nonSigners {
		addrHex := hex.EncodeToString(curr.Address)
		prev := prevNonSignerMap[addrHex]
		changed := false
		if prev == nil {
			changed = true
			numNonSigningInfosNew++
		} else if curr.Counter != prev.Counter {
			changed = true
		}

		if changed {
			nonSigningInfo := &indexermodels.ValidatorNonSigningInfo{
				Address:           addrHex,
				MissedBlocksCount: curr.Counter,
				LastSignedHeight:  height,
				Height:            height,
				HeightTime:        heightTime,
			}
			changedNonSigningInfos = append(changedNonSigningInfos, nonSigningInfo)
		}
	}

	for _, prev := range previousData.nonSigners {
		addrHex := hex.EncodeToString(prev.Address)
		if _, exists := currentNonSignerMap[addrHex]; !exists {
			resetInfo := &indexermodels.ValidatorNonSigningInfo{
				Address:           addrHex,
				MissedBlocksCount: 0,
				LastSignedHeight:  height,
				Height:            height,
				HeightTime:        heightTime,
			}
			changedNonSigningInfos = append(changedNonSigningInfos, resetInfo)
		}
	}

	prevDoubleSignersMap := make(map[string]uint64, len(previousData.doubleSigners))
	for _, ds := range previousData.doubleSigners {
		addrHex := hex.EncodeToString(ds.Id)
		prevDoubleSignersMap[addrHex] = uint64(len(ds.Heights))
	}

	currentDoubleSignersMap := make(map[string]*lib.DoubleSigner, len(currentData.doubleSigners))
	for _, ds := range currentData.doubleSigners {
		addrHex := hex.EncodeToString(ds.Id)
		currentDoubleSignersMap[addrHex] = ds
	}

	changedDoubleSigningInfos := make([]*indexermodels.ValidatorDoubleSigningInfo, 0)
	for _, curr := range currentData.doubleSigners {
		addrHex := hex.EncodeToString(curr.Id)
		currentCount := uint64(len(curr.Heights))
		prevCount := prevDoubleSignersMap[addrHex]

		if currentCount != prevCount {
			var firstHeight, lastHeight uint64
			if len(curr.Heights) > 0 {
				firstHeight = curr.Heights[0]
				lastHeight = curr.Heights[len(curr.Heights)-1]
			}
			info := &indexermodels.ValidatorDoubleSigningInfo{
				Address:             addrHex,
				EvidenceCount:       currentCount,
				FirstEvidenceHeight: firstHeight,
				LastEvidenceHeight:  lastHeight,
				Height:              height,
				HeightTime:          heightTime,
			}
			changedDoubleSigningInfos = append(changedDoubleSigningInfos, info)
		}
	}

	for _, prev := range previousData.doubleSigners {
		addrHex := hex.EncodeToString(prev.Id)
		if _, exists := currentDoubleSignersMap[addrHex]; !exists {
			resetInfo := &indexermodels.ValidatorDoubleSigningInfo{
				Address:             addrHex,
				EvidenceCount:       0,
				FirstEvidenceHeight: 0,
				LastEvidenceHeight:  0,
				Height:              height,
				HeightTime:          heightTime,
			}
			changedDoubleSigningInfos = append(changedDoubleSigningInfos, resetInfo)
		}
	}

	var committeeValidators []*indexermodels.CommitteeValidator
	for _, v := range changedValidators {
		for _, committeeID := range v.Committees {
			cv := &indexermodels.CommitteeValidator{
				CommitteeID:      committeeID,
				ValidatorAddress: v.Address,
				StakedAmount:     v.StakedAmount,
				Status:           v.Status,
				Delegate:         v.Delegate,
				Compound:         v.Compound,
				Height:           v.Height,
				HeightTime:       v.HeightTime,
				Subsidized:       subsidizedMap[committeeID],
				Retired:          retiredMap[committeeID],
			}
			committeeValidators = append(committeeValidators, cv)
		}
	}

	if len(changedValidators) > 0 {
		if err := chainDb.InsertValidatorsProduction(ctx, changedValidators); err != nil {
			return types.ActivityIndexValidatorsOutput{}, 0, err
		}
	}
	if len(changedNonSigningInfos) > 0 {
		if err := chainDb.InsertValidatorNonSigningInfoProduction(ctx, changedNonSigningInfos); err != nil {
			return types.ActivityIndexValidatorsOutput{}, 0, err
		}
	}
	if len(committeeValidators) > 0 {
		if err := chainDb.InsertCommitteeValidatorsProduction(ctx, committeeValidators); err != nil {
			return types.ActivityIndexValidatorsOutput{}, 0, err
		}
	}
	if len(changedDoubleSigningInfos) > 0 {
		if err := chainDb.InsertValidatorDoubleSigningInfoProduction(ctx, changedDoubleSigningInfos); err != nil {
			return types.ActivityIndexValidatorsOutput{}, 0, err
		}
	}
	out := types.ActivityIndexValidatorsOutput{
		NumValidators:          uint32(len(changedValidators)),
		NumValidatorsNew:       numValidatorsNew,
		NumValidatorsActive:    numValidatorsActive,
		NumValidatorsPaused:    numValidatorsPaused,
		NumValidatorsUnstaking: numValidatorsUnstaking,
		NumNonSigningInfos:     uint32(len(changedNonSigningInfos)),
		NumNonSigningInfosNew:  numNonSigningInfosNew,
		NumDoubleSigningInfos:  uint32(len(changedDoubleSigningInfos)),
		NumCommitteeValidators: uint32(len(committeeValidators)),
	}
	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return out, durationMs, nil
}

func (ac *Context) indexCommitteesFromBlob(ctx context.Context, chainDb chainstore.Store, height uint64, heightTime time.Time, currentData *blobData, previousData *blobData) (types.ActivityIndexCommitteesOutput, float64, error) {
	start := time.Now()
	committeesAtH := currentData.committeesData
	committeesAtH1 := previousData.committeesData
	numSubsidizedTotal := uint8(len(currentData.subsidizedCommittees))
	numRetiredTotal := uint8(len(currentData.retiredCommittees))

	currentCommittees := make(map[uint64]*indexermodels.Committee)
	for _, rpcCommittee := range committeesAtH {
		currentCommittees[rpcCommittee.ChainId] = &indexermodels.Committee{
			ChainID:                uint16(rpcCommittee.ChainId),
			LastRootHeightUpdated:  rpcCommittee.LastRootHeightUpdated,
			LastChainHeightUpdated: rpcCommittee.LastChainHeightUpdated,
			NumberOfSamples:        rpcCommittee.NumberOfSamples,
			Subsidized:             numSubsidizedTotal,
			Retired:                numRetiredTotal,
			Height:                 height,
			HeightTime:             heightTime,
		}
	}

	var changedCommittees []*indexermodels.Committee
	var numCommitteesNew uint32
	if height == 1 {
		for _, committee := range currentCommittees {
			changedCommittees = append(changedCommittees, committee)
		}
		numCommitteesNew = uint32(len(currentCommittees))
	} else {
		numSubsidizedTotalH1 := uint8(len(previousData.subsidizedCommittees))
		numRetiredTotalH1 := uint8(len(previousData.retiredCommittees))
		prevMap := make(map[uint64]*indexermodels.Committee)
		for _, rpcCommittee := range committeesAtH1 {
			prevMap[rpcCommittee.ChainId] = &indexermodels.Committee{
				ChainID:                uint16(rpcCommittee.ChainId),
				LastRootHeightUpdated:  rpcCommittee.LastRootHeightUpdated,
				LastChainHeightUpdated: rpcCommittee.LastChainHeightUpdated,
				NumberOfSamples:        rpcCommittee.NumberOfSamples,
				Subsidized:             numSubsidizedTotalH1,
				Retired:                numRetiredTotalH1,
			}
		}

		for chainID, currentCommittee := range currentCommittees {
			prevCommittee, existed := prevMap[chainID]
			if !existed {
				changedCommittees = append(changedCommittees, currentCommittee)
				numCommitteesNew++
				continue
			}
			if !committeesEqual(prevCommittee, currentCommittee) {
				changedCommittees = append(changedCommittees, currentCommittee)
			}
		}
	}

	var payments []*indexermodels.CommitteePayment
	for _, rpcCommittee := range committeesAtH {
		for _, pp := range rpcCommittee.PaymentPercents {
			payments = append(payments, &indexermodels.CommitteePayment{
				CommitteeID: rpcCommittee.ChainId,
				Address:     bytesToHex(pp.Address),
				Percent:     pp.Percent,
				Height:      height,
				HeightTime:  heightTime,
			})
		}
	}

	if len(changedCommittees) > 0 {
		if err := chainDb.InsertCommitteesProduction(ctx, changedCommittees); err != nil {
			return types.ActivityIndexCommitteesOutput{}, 0, err
		}
	}
	if len(payments) > 0 {
		if err := chainDb.InsertCommitteePaymentsProduction(ctx, payments); err != nil {
			return types.ActivityIndexCommitteesOutput{}, 0, err
		}
	}
	out := types.ActivityIndexCommitteesOutput{
		NumCommittees:           uint32(len(changedCommittees)),
		NumCommitteesNew:        numCommitteesNew,
		NumCommitteesSubsidized: uint32(len(currentData.subsidizedCommittees)),
		NumCommitteesRetired:    uint32(len(currentData.retiredCommittees)),
		NumCommitteePayments:    uint32(len(payments)),
	}
	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return out, durationMs, nil
}

func (ac *Context) indexDexBatchFromBlob(ctx context.Context, chainDb chainstore.Store, height uint64, heightTime time.Time, currentData *blobData, previousData *blobData, events *blobEventMaps) (types.ActivityIndexDexBatchOutput, float64, error) {
	start := time.Now()
	h1Maps := ac.buildH1ComparisonMaps(previousData.dexBatches, previousData.nextDexBatches)
	orders := make([]*indexermodels.DexOrder, 0)
	deposits := make([]*indexermodels.DexDeposit, 0)
	withdrawals := make([]*indexermodels.DexWithdrawal, 0)
	counters := &dexBatchCounters{}

	ac.processCompleteItems(previousData.dexBatches, events.dexSwap, events.dexDeposit, events.dexWithdrawal, height, heightTime, &orders, &deposits, &withdrawals, counters)
	ac.processLockedItems(currentData.dexBatches, h1Maps, height, heightTime, &orders, &deposits, &withdrawals, counters)
	ac.processPendingItems(currentData.nextDexBatches, h1Maps, height, heightTime, &orders, &deposits, &withdrawals, counters)

	if len(orders) > 0 {
		if err := chainDb.InsertDexOrdersProduction(ctx, orders); err != nil {
			return types.ActivityIndexDexBatchOutput{}, 0, err
		}
	}
	if len(deposits) > 0 {
		if err := chainDb.InsertDexDepositsProduction(ctx, deposits); err != nil {
			return types.ActivityIndexDexBatchOutput{}, 0, err
		}
	}
	if len(withdrawals) > 0 {
		if err := chainDb.InsertDexWithdrawalsProduction(ctx, withdrawals); err != nil {
			return types.ActivityIndexDexBatchOutput{}, 0, err
		}
	}

	out := types.ActivityIndexDexBatchOutput{
		NumOrders:              uint32(len(orders)),
		NumOrdersFuture:        counters.NumOrdersFuture,
		NumOrdersLocked:        counters.NumOrdersLocked,
		NumOrdersComplete:      counters.NumOrdersComplete,
		NumOrdersSuccess:       counters.NumOrdersSuccess,
		NumOrdersFailed:        counters.NumOrdersFailed,
		NumDeposits:            uint32(len(deposits)),
		NumDepositsPending:     counters.NumDepositsPending,
		NumDepositsLocked:      counters.NumDepositsLocked,
		NumDepositsComplete:    counters.NumDepositsComplete,
		NumWithdrawals:         uint32(len(withdrawals)),
		NumWithdrawalsPending:  counters.NumWithdrawalsPending,
		NumWithdrawalsLocked:   counters.NumWithdrawalsLocked,
		NumWithdrawalsComplete: counters.NumWithdrawalsComplete,
	}
	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return out, durationMs, nil
}

func (ac *Context) indexSupplyFromBlob(ctx context.Context, chainDb chainstore.Store, height uint64, heightTime time.Time, currentData *blobData, previousData *blobData) (types.ActivityIndexSupplyOutput, float64, error) {
	start := time.Now()
	var supplyOut types.ActivityIndexSupplyOutput
	if currentData.supply != nil {
		currentSupply := &indexermodels.Supply{
			Total:         currentData.supply.Total,
			Staked:        currentData.supply.Staked,
			DelegatedOnly: currentData.supply.DelegatedOnly,
			Height:        height,
			HeightTime:    heightTime,
		}

		var previousSupply *indexermodels.Supply
		if previousData.supply != nil {
			previousSupply = &indexermodels.Supply{
				Total:         previousData.supply.Total,
				Staked:        previousData.supply.Staked,
				DelegatedOnly: previousData.supply.DelegatedOnly,
			}
		}

		changed := previousSupply == nil ||
			currentSupply.Total != previousSupply.Total ||
			currentSupply.Staked != previousSupply.Staked ||
			currentSupply.DelegatedOnly != previousSupply.DelegatedOnly

		if changed {
			if err := chainDb.InsertSupplyProduction(ctx, []*indexermodels.Supply{currentSupply}); err != nil {
				return types.ActivityIndexSupplyOutput{}, 0, err
			}
		}

		supplyOut = types.ActivityIndexSupplyOutput{
			Changed:       changed,
			Total:         currentSupply.Total,
			Staked:        currentSupply.Staked,
			DelegatedOnly: currentSupply.DelegatedOnly,
		}
	} else {
		supplyOut = types.ActivityIndexSupplyOutput{}
	}
	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return supplyOut, durationMs, nil
}

func (ac *Context) saveBlockSummaryFromBlob(ctx context.Context, chainDb chainstore.Store, summary *indexermodels.BlockSummary) (float64, error) {
	start := time.Now()
	if err := chainDb.InsertBlockSummariesProduction(ctx, summary); err != nil {
		return 0, err
	}
	return float64(time.Since(start).Microseconds()) / 1000.0, nil
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

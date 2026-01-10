package activity

import (
	"context"
	"fmt"
	"github.com/canopy-network/canopy/lib"
	globalstore "github.com/canopy-network/canopyx/pkg/db/global"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"github.com/canopy-network/canopyx/pkg/db/transform"
)

type blobEventMaps struct {
	reward            map[string]uint64
	slash             map[string]uint64
	pause             map[string]struct{}
	beginUnstaking    map[string]struct{}
	validatorReward   map[string]struct{}
	validatorSlash    map[string]struct{}
	orderBookSwap     map[string]struct{}
	dexSwap           map[string]*globalstore.EventDexBatch
	dexDeposit        map[string]*globalstore.EventDexBatch
	dexWithdrawal     map[string]*globalstore.EventDexBatch
	eventCountsByType map[string]uint32
}

// indexEventsFromBlob indexes events for a given block.
func (ac *Context) indexEventsFromBlob(ctx context.Context, chainDb globalstore.Store, height uint64, heightTime time.Time, currentData *blobData) (types.ActivityIndexEventsOutput, *blobEventMaps, float64, error) {
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
	dexSwapEvents := make(map[string]*globalstore.EventDexBatch)
	dexDepositEvents := make(map[string]*globalstore.EventDexBatch)
	dexWithdrawalEvents := make(map[string]*globalstore.EventDexBatch)

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
				dexSwapEvents[event.OrderID] = &globalstore.EventDexBatch{
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
				dexDepositEvents[event.OrderID] = &globalstore.EventDexBatch{
					Height:         height,
					EventType:      event.EventType,
					OrderID:        event.OrderID,
					LocalOrigin:    event.LocalOrigin,
					PointsReceived: event.PointsReceived,
				}
			}
		case string(lib.EventTypeDexLiquidityWithdraw):
			if event.OrderID != "" {
				dexWithdrawalEvents[event.OrderID] = &globalstore.EventDexBatch{
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
		if err := chainDb.InsertEvents(ctx, events); err != nil {
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

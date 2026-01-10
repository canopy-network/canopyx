package activity

import (
    "bytes"
    "context"
    "encoding/hex"
    indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
    "time"

    "github.com/canopy-network/canopy/lib"
    "github.com/canopy-network/canopyx/app/indexer/types"
    globalstore "github.com/canopy-network/canopyx/pkg/db/global"
)

// IndexOrders indexes order state changes for a given block using the snapshot-on-change pattern.
func (ac *Context) indexOrdersFromBlob(ctx context.Context, chainDb globalstore.Store, height uint64, heightTime time.Time, currentData *blobData, previousData *blobData, events *blobEventMaps) (types.ActivityIndexOrdersOutput, float64, error) {
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
        if err := chainDb.InsertOrders(ctx, changedOrders); err != nil {
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

// orderStateChanged compares two order states to detect changes.
// Returns true if any significant field has changed.
func orderStateChanged(prev, curr *lib.SellOrder) bool {
    // Check all significant fields (use bytes.Equal for []byte fields)
    if !bytes.Equal(prev.Data, curr.Data) {
        return true
    }
    if prev.AmountForSale != curr.AmountForSale {
        return true
    }
    if prev.RequestedAmount != curr.RequestedAmount {
        return true
    }
    if !bytes.Equal(prev.SellerReceiveAddress, curr.SellerReceiveAddress) {
        return true
    }
    if !bytes.Equal(prev.BuyerSendAddress, curr.BuyerSendAddress) {
        return true
    }
    if !bytes.Equal(prev.BuyerReceiveAddress, curr.BuyerReceiveAddress) {
        return true
    }
    if prev.BuyerChainDeadline != curr.BuyerChainDeadline {
        return true
    }
    if !bytes.Equal(prev.SellersSendAddress, curr.SellersSendAddress) {
        return true
    }

    return false
}

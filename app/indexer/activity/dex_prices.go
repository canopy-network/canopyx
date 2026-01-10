package activity

import (
    "context"
    globalstore "github.com/canopy-network/canopyx/pkg/db/global"
    indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
    "time"

    "github.com/canopy-network/canopyx/app/indexer/types"
    "github.com/canopy-network/canopyx/pkg/db/transform"
)

// IndexDexPrices indexes DEX price information for a given block.
// Returns output containing the number of indexed price records and execution duration in milliseconds.
func (ac *Context) indexDexPricesFromBlob(ctx context.Context, chainDb globalstore.Store, height uint64, heightTime time.Time, currentData *blobData, previousData *blobData) (types.ActivityIndexDexPricesOutput, float64, error) {
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
        if err := chainDb.InsertDexPrices(ctx, dbPrices); err != nil {
            return types.ActivityIndexDexPricesOutput{}, 0, err
        }
    }
    out := types.ActivityIndexDexPricesOutput{
        NumPrices: uint32(len(dbPrices)),
    }
    durationMs := float64(time.Since(start).Microseconds()) / 1000.0
    return out, durationMs, nil
}

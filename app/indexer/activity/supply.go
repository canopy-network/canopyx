package activity

import (
    "context"
    globalstore "github.com/canopy-network/canopyx/pkg/db/global"
    indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
    "time"

    "github.com/canopy-network/canopyx/app/indexer/types"
)

// indexSupplyFromBlob indexes token supply metrics for a given block using the snapshot-on-change pattern.
func (ac *Context) indexSupplyFromBlob(ctx context.Context, chainDb globalstore.Store, height uint64, heightTime time.Time, currentData *blobData, previousData *blobData) (types.ActivityIndexSupplyOutput, float64, error) {
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
            if err := chainDb.InsertSupply(ctx, []*indexermodels.Supply{currentSupply}); err != nil {
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

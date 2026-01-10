package activity

import (
    "context"
    "encoding/hex"
    "fmt"
    globalstore "github.com/canopy-network/canopyx/pkg/db/global"
    indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
    "time"

    "github.com/canopy-network/canopyx/app/indexer/types"
    "github.com/canopy-network/canopyx/pkg/db/transform"
)

// indexPoolsFromBlob indexes pools for a given block height.
func (ac *Context) indexPoolsFromBlob(ctx context.Context, chainDb globalstore.Store, height uint64, heightTime time.Time, currentData *blobData, previousData *blobData) (types.ActivityIndexPoolsOutput, float64, error) {
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
        if err := chainDb.InsertPools(ctx, pools); err != nil {
            return types.ActivityIndexPoolsOutput{}, 0, err
        }
    }
    if len(changedHolders) > 0 {
        if err := chainDb.InsertPoolPointsByHolder(ctx, changedHolders); err != nil {
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

package activity

import (
    "context"
    globalstore "github.com/canopy-network/canopyx/pkg/db/global"
    "time"

    "github.com/canopy-network/canopyx/pkg/db/transform"
)

// indexBlockResultFromBlob indexes a block from the provided blob data and stores it in the database, returning the duration in ms.
func (ac *Context) indexBlockResultFromBlob(ctx context.Context, chainDb globalstore.Store, currentData *blobData) (float64, error) {
    start := time.Now()
    block := transform.Block(currentData.block)
    if err := chainDb.InsertBlocks(ctx, block); err != nil {
        return 0, err
    }
    return float64(time.Since(start).Microseconds()) / 1000.0, nil
}

package activity

import (
	"context"
	globalstore "github.com/canopy-network/canopyx/pkg/db/global"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"time"
)

// saveBlockSummaryFromBlob indexes a block summary from the provided block summary and stores it in the database, returning the duration in ms.
func (ac *Context) saveBlockSummaryFromBlob(ctx context.Context, chainDb globalstore.Store, summary *indexermodels.BlockSummary) (float64, error) {
	start := time.Now()
	if err := chainDb.InsertBlockSummaries(ctx, summary); err != nil {
		return 0, err
	}
	return float64(time.Since(start).Microseconds()) / 1000.0, nil
}

package activity

import (
	"context"

	"github.com/canopy-network/canopyx/pkg/indexer/types"
)

// RecordIndexed records the height of the last block indexed for a given chain along with timing metrics.
func (c *Context) RecordIndexed(ctx context.Context, in types.RecordIndexedInput) error {
	if err := c.IndexerDB.RecordIndexed(ctx, in.ChainID, in.Height, in.IndexingTimeMs, in.IndexingDetail); err != nil {
		return err
	}
	return nil
}

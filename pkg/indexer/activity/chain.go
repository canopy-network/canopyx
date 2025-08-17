package activity

import (
	"context"

	"github.com/canopy-network/canopyx/pkg/indexer/types"
)

// RecordIndexed records the height of the last block indexed for a given chain.
func (c *Context) RecordIndexed(ctx context.Context, in types.IndexBlockInput) error {
	if err := c.IndexerDB.RecordIndexed(ctx, in.ChainID, in.Height); err != nil {
		return err
	}
	return nil
}

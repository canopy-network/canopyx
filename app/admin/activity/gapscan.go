package activity

import (
	"context"

	"github.com/canopy-network/canopyx/pkg/db"
	"github.com/canopy-network/canopyx/pkg/indexer/types"
)

// FindGaps identifies and retrieves missing height ranges (gaps) in the indexing progress for a specific blockchain.
func (c *Context) FindGaps(ctx context.Context, in types.ChainIdInput) ([]db.Gap, error) {
	return c.IndexerDB.FindGaps(ctx, in.ChainID)
}

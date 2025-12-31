package transform

import (
	"fmt"

	"github.com/canopy-network/canopy/lib"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// DexPrice converts a lib.DexPrice (from Canopy lib/dex.go) to the database model.
// The height and height_time fields must be populated by the caller (activity layer).
func DexPrice(r *lib.DexPrice) *indexer.DexPrice {
	return &indexer.DexPrice{
		LocalChainID:  r.LocalChainId,
		RemoteChainID: r.RemoteChainId,
		LocalPool:     r.LocalPool,
		RemotePool:    r.RemotePool,
		PriceE6:       r.E6ScaledPrice,
		// Height and HeightTime will be set by the activity layer
	}
}

// DexPriceKey creates a unique key for DexPrice lookups based on chain pair.
// Used for efficient map-based lookups when calculating H-1 deltas.
func DexPriceKey(localChainID, remoteChainID uint64) string {
	return fmt.Sprintf("%d:%d", localChainID, remoteChainID)
}

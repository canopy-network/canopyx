package transform

import (
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/rpc"
)

// DexPrice converts an RPC dex price response to the database model.
// The height and height_time fields must be populated by the caller (activity layer).
func DexPrice(r *rpc.RpcDexPrice) *indexer.DexPrice {
	return &indexer.DexPrice{
		LocalChainID:  r.LocalChainID,
		RemoteChainID: r.RemoteChainID,
		LocalPool:     r.LocalPool,
		RemotePool:    r.RemotePool,
		PriceE6:       r.E6ScaledPrice,
		// Height and HeightTime will be set by the activity layer
	}
}

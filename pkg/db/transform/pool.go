package transform

import (
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/rpc"
)

// Pool converts an RpcPool to the database Pool model.
// The points array is used only to calculate lp_count.
// Individual points allocations are not stored in the pool table.
// The chain_id is extracted from the pool_id encoding (since RPC doesn't return it separately).
// Pool IDs are encoded as: TypeAddend + ChainID, where TypeAddend is 0 (reward), 16384 (holding), 32768 (liquidity), or 65536 (escrow).
// The height parameter must be provided by the caller (activity layer).
func Pool(rp *rpc.RpcPool, height uint64) *indexer.Pool {
	// Extract chain_id from pool_id if RPC doesn't provide it.
	// Canopy blockchain encodes chain_id directly in pool IDs (fsm/key.go:16-28).
	// Pool structure has no separate chain_id field (fsm/account.pb.go:100-114).
	chainID := rp.ChainID
	if chainID == 0 {
		chainID = indexer.ExtractChainIDFromPoolID(rp.ID)
	}

	return &indexer.Pool{
		PoolID:      rp.ID,
		Height:      height,
		ChainID:     chainID,
		Amount:      rp.Amount,
		TotalPoints: rp.TotalPoolPoints,
		LPCount:     uint32(len(rp.Points)),
	}
}

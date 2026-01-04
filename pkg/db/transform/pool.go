package transform

import (
	"github.com/canopy-network/canopy/fsm"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// Pool converts a fsm.Pool (protobuf type from Canopy) to the database Pool model.
// The points array is used only to calculate lp_count.
// Individual points allocations are not stored in the pool table.
// The chain_id is always extracted from the pool_id encoding (fsm.Pool doesn't have a separate ChainID field).
// Pool IDs are encoded as: TypeAddend + ChainID, where TypeAddend is 0 (reward), 16384 (holding), 32768 (liquidity), or 65536 (escrow).
// The height parameter must be provided by the caller (activity layer).
func Pool(rp *fsm.Pool, height uint64) *indexer.Pool {
	// Extract chain_id from pool_id.
	// Canopy blockchain encodes chain_id directly in pool IDs (fsm/key.go:16-28).
	// Pool structure has no separate chain_id field (fsm/account.pb.go:100-114).
	chainID := indexer.ExtractChainIDFromPoolID(rp.Id)

	return &indexer.Pool{
		PoolID:      uint32(rp.Id), // Pool IDs fit in uint32 (max ~65536)
		Height:      height,
		ChainID:     uint16(chainID),
		Amount:      rp.Amount,
		TotalPoints: rp.TotalPoolPoints,
		LPCount:     uint16(len(rp.Points)),
	}
}

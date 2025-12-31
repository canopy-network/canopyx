package transform

import (
	"time"

	"github.com/canopy-network/canopy/lib"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// Block converts a Canopy lib.BlockResult to the database Block model.
// The BlockResult contains the block header with all consensus fields,
// including merkle roots for state, transactions, and validators.
func Block(result *lib.BlockResult) *indexer.Block {
	h := result.BlockHeader

	return &indexer.Block{
		// Core block identification
		Height:          h.Height,
		Hash:            bytesToHex(h.Hash),
		Time:            time.UnixMicro(int64(h.Time)),
		NetworkID:       uint64(h.NetworkId),
		LastBlockHash:   bytesToHex(h.LastBlockHash),
		ProposerAddress: bytesToHex(h.ProposerAddress),
		Size:            int32(result.Meta.Size),
		// Block metrics (transaction counts and VDF progress)
		NumTxs:             h.NumTxs,
		TotalTxs:           h.TotalTxs,
		TotalVDFIterations: int32(h.TotalVdfIterations),
		// Merkle roots for verification (enables light client operation and state proofs)
		StateRoot:         bytesToHex(h.StateRoot),
		TransactionRoot:   bytesToHex(h.TransactionRoot),
		ValidatorRoot:     bytesToHex(h.ValidatorRoot),
		NextValidatorRoot: bytesToHex(h.NextValidatorRoot),
	}
}

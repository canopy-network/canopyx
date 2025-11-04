package transform

import (
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/rpc"
)

// Block converts a BlockByHeight RPC response to the database Block model.
func Block(bbh *rpc.BlockByHeight) *indexer.Block {
	return &indexer.Block{
		Height:          bbh.BlockHeader.Height,
		Hash:            bbh.BlockHeader.Hash,
		Time:            time.UnixMicro(bbh.BlockHeader.Time),
		LastBlockHash:   bbh.BlockHeader.LastBlockHash,
		ProposerAddress: bbh.BlockHeader.ProposerAddress,
		Size:            int32(bbh.Meta.Size),
	}
}

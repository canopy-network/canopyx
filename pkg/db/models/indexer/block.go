package indexer

import (
	"time"
)

const BlocksProductionTableName = "blocks"
const BlocksStagingTableName = BlocksProductionTableName + "_staging"

type Block struct {
	Height          uint64    `ch:"height" json:"height"`
	Hash            string    `ch:"hash" json:"hash"`
	Time            time.Time `ch:"time" json:"time"` // stored as DateTime64(6)
	LastBlockHash   string    `ch:"parent_hash" json:"parent_hash"`
	ProposerAddress string    `ch:"proposer_address" json:"proposer_address"`
	Size            int32     `ch:"size" json:"size"`
}

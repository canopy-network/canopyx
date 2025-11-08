package indexer

import (
	"time"

	"github.com/canopy-network/canopyx/pkg/db/entities"
)

const BlocksProductionTableName = "blocks"
const BlocksStagingTableName = BlocksProductionTableName + entities.StagingSuffix

// BlockColumns defines the schema for the blocks table.
var BlockColumns = []ColumnDef{
	{Name: "height", Type: "UInt64"},
	{Name: "hash", Type: "String"},
	{Name: "time", Type: "DateTime64(6)"},
	{Name: "network_id", Type: "UInt64"},
	{Name: "parent_hash", Type: "String"},
	{Name: "proposer_address", Type: "String"},
	{Name: "size", Type: "Int32"},
}

type Block struct {
	Height          uint64    `ch:"height" json:"height"`
	Hash            string    `ch:"hash" json:"hash"`
	Time            time.Time `ch:"time" json:"time"` // stored as DateTime64(6)
	NetworkID       uint64    `ch:"network_id" json:"network_id"`
	LastBlockHash   string    `ch:"parent_hash" json:"parent_hash"`
	ProposerAddress string    `ch:"proposer_address" json:"proposer_address"`
	Size            int32     `ch:"size" json:"size"`
}

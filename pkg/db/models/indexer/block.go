package indexer

import (
	"context"
	"time"

	"github.com/uptrace/go-clickhouse/ch"
)

type Block struct {
	ch.CHModel `ch:"table:blocks"`

	Height          uint64    `ch:"height,pk"`
	Hash            string    `ch:"hash"`
	Time            time.Time `ch:"time,type:DateTime64(6)"` // stored as a Unix timestamp
	LastBlockHash   string    `ch:"parent_hash"`
	ProposerAddress string    `ch:"proposer_address"`
	Size            int       `ch:"size"`
	NumTxs          uint32    `ch:"num_txs,default:0"`
}

// InitBlocks initializes the blocks table.
func InitBlocks(ctx context.Context, db *ch.DB) error {
	_, err := db.NewCreateTable().
		Model((*Block)(nil)).
		IfNotExists().
		Engine("MergeTree").
		Order("height").
		Exec(ctx)
	return err
}

// GetBlock returns the latest (deduped) row for the given height.
func GetBlock(ctx context.Context, db *ch.DB, height uint64) (*Block, error) {
	var b Block

	err := db.NewSelect().
		Model(&b).
		Where("height = ?", height).
		Limit(1).
		Scan(ctx)

	return &b, err
}

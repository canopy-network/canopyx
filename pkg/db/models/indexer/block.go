package indexer

import (
	"context"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type Block struct {
	Height          uint64    `ch:"height" json:"height"`
	Hash            string    `ch:"hash" json:"hash"`
	Time            time.Time `ch:"time" json:"time"` // stored as DateTime64(6)
	LastBlockHash   string    `ch:"parent_hash" json:"parent_hash"`
	ProposerAddress string    `ch:"proposer_address" json:"proposer_address"`
	Size            int32     `ch:"size" json:"size"`
}

// InitBlocks initializes the blocks table.
func InitBlocks(ctx context.Context, db driver.Conn) error {
	query := `
		CREATE TABLE IF NOT EXISTS blocks (
			height UInt64,
			hash String,
			time DateTime64(6),
			parent_hash String,
			proposer_address String,
			size Int32
		) ENGINE = ReplacingMergeTree(height)
		ORDER BY (height)
	`
	return db.Exec(ctx, query)
}

// GetBlock returns the latest (deduped) row for the given height.
func GetBlock(ctx context.Context, db driver.Conn, height uint64) (*Block, error) {
	var b Block
	query := `
		SELECT height, hash, time, parent_hash, proposer_address, size
		FROM blocks FINAL
		WHERE height = ?
		LIMIT 1
	`
	err := db.QueryRow(ctx, query, height).Scan(
		&b.Height,
		&b.Hash,
		&b.Time,
		&b.LastBlockHash,
		&b.ProposerAddress,
		&b.Size,
	)
	return &b, err
}

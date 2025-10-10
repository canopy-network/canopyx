package admin

import (
	"context"
	"time"

	"github.com/uptrace/go-clickhouse/ch"
)

type Chain struct {
	ch.CHModel `ch:"table:chains"`

	ChainID      string    `json:"chain_id" ch:"chain_id"` // ORDER BY set via builder
	ChainName    string    `json:"chain_name" ch:"chain_name"`
	RPCEndpoints []string  `json:"rpc_endpoints" ch:"rpc_endpoints"` // []string -> Array(String)
	Paused       uint8     `json:"paused" ch:"paused,default:0"`     // defaults are applied by CreateTable
	Deleted      uint8     `json:"deleted" ch:"deleted,default:0"`
	Image        string    `json:"image" ch:"image,default:''"`
	MinReplicas  uint16    `json:"min_replicas" ch:"min_replicas,default:1"`
	MaxReplicas  uint16    `json:"max_replicas" ch:"max_replicas,default:1"`
	Notes        string    `json:"notes,omitempty" ch:"notes,default:''"`
	CreatedAt    time.Time `json:"created_at" ch:"created_at,default:now()"`
	UpdatedAt    time.Time `json:"updated_at" ch:"updated_at,default:now()"`
}

// InitChains creates the chain table using the models.Chain definition.
// Table: ReplacingMergeTree(updated_at) ORDER BY (chain_id)
func InitChains(ctx context.Context, db *ch.DB) error {
	_, err := db.NewCreateTable().
		Model((*Chain)(nil)).
		IfNotExists().
		Engine("ReplacingMergeTree(updated_at)").
		// explicit ORDER BY to match your prior SQL
		Order("(chain_id)").
		Exec(ctx)
	return err
}

// GetChain returns the latest (deduped) row for the given chain_id.
func GetChain(ctx context.Context, db *ch.DB, id string) (*Chain, error) {
	var c Chain

	// Use FINAL to collapse versions from ReplacingMergeTree(updated_at)
	err := db.NewSelect().
		Model(&c).
		Final().
		Where("chain_id = ?", id).
		Limit(1).
		Scan(ctx)

	return &c, err
}

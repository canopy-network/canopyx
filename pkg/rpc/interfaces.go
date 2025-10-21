package rpc

import (
	"context"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// Client captures the RPC calls used by activities when indexing blocks and transactions.
type Client interface {
	ChainHead(ctx context.Context) (uint64, error)
	BlockByHeight(ctx context.Context, height uint64) (*indexer.Block, error)
	TxsByHeight(ctx context.Context, height uint64) ([]*indexer.Transaction, error)
	EventsByHeight(ctx context.Context, height uint64) ([]*indexer.Event, error)
	AccountsByHeight(ctx context.Context, height uint64) ([]*Account, error)
	OrdersByHeight(ctx context.Context, height uint64, chainID uint64) ([]*RpcOrder, error)
	GetGenesisState(ctx context.Context, height uint64) (*GenesisState, error)
	DexPrice(ctx context.Context, chainID uint64) (*indexer.DexPrice, error)
	DexPrices(ctx context.Context) ([]*indexer.DexPrice, error)
	PoolByID(ctx context.Context, id uint64) (*RpcPool, error)
	Pools(ctx context.Context) ([]*RpcPool, error)
}

// Factory produces RPC clients for a given set of endpoints.
type Factory interface {
	NewClient(endpoints []string) Client
}

type httpFactory struct {
	opts Opts
}

// NewHTTPFactory returns a factory that builds HTTP clients with shared defaults.
func NewHTTPFactory(opts Opts) Factory {
	return &httpFactory{opts: opts}
}

func (f *httpFactory) NewClient(endpoints []string) Client {
	o := f.opts
	o.Endpoints = endpoints
	return NewHTTPWithOpts(o)
}

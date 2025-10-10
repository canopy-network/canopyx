package rpc

import (
	"context"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// Client captures the RPC calls used by activities when indexing blocks and transactions.
type Client interface {
	ChainHead(ctx context.Context) (uint64, error)
	BlockByHeight(ctx context.Context, height uint64) (*indexer.Block, error)
	TxsByHeight(ctx context.Context, height uint64) ([]*indexer.Transaction, []*indexer.TransactionRaw, error)
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

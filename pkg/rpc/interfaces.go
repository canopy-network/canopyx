package rpc

import (
	"context"
)

// Client captures the RPC calls used by activities when indexing blocks and transactions.
// Methods return raw RPC types that should be converted to indexer models at the activity layer.
type Client interface {
	ChainHead(ctx context.Context) (uint64, error)
	BlockByHeight(ctx context.Context, height uint64) (*BlockByHeight, error)
	TxsByHeight(ctx context.Context, height uint64) ([]*Transaction, error)
	EventsByHeight(ctx context.Context, height uint64) ([]*RpcEvent, error)
	AccountsByHeight(ctx context.Context, height uint64) ([]*Account, error)
	OrdersByHeight(ctx context.Context, height uint64, chainID uint64) ([]*RpcOrder, error)
	StateByHeight(ctx context.Context, height uint64) (*StateResponse, error)
	DexPricesByHeight(ctx context.Context, height uint64) ([]*RpcDexPrice, error)
	PoolsByHeight(ctx context.Context, height uint64) ([]*RpcPool, error)
	AllDexBatchesByHeight(ctx context.Context, height uint64) ([]*RpcDexBatch, error)
	AllNextDexBatchesByHeight(ctx context.Context, height uint64) ([]*RpcDexBatch, error)
	AllParamsByHeight(ctx context.Context, height uint64) (*RpcAllParams, error)
	ValParamsByHeight(ctx context.Context, height uint64) (*ValidatorParams, error)
	ValidatorsByHeight(ctx context.Context, height uint64) ([]*RpcValidator, error)
	NonSignersByHeight(ctx context.Context, height uint64) ([]*RpcNonSigner, error)
	DoubleSignersByHeight(ctx context.Context, height uint64) ([]*RpcDoubleSigner, error)
	CommitteesDataByHeight(ctx context.Context, height uint64) ([]*RpcCommitteeData, error)
	SubsidizedCommitteesByHeight(ctx context.Context, height uint64) ([]uint64, error)
	RetiredCommitteesByHeight(ctx context.Context, height uint64) ([]uint64, error)
	// Poll does not support historical query (aka by Height)
	Poll(ctx context.Context) (RpcPoll, error)
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

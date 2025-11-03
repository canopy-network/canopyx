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
	State(ctx context.Context) (*StateResponse, error)
	DexPrice(ctx context.Context, chainID uint64) (*indexer.DexPrice, error)
	DexPrices(ctx context.Context) ([]*indexer.DexPrice, error)
	PoolByID(ctx context.Context, id uint64) (*RpcPool, error)
	Pools(ctx context.Context) ([]*RpcPool, error)
	DexBatchByHeight(ctx context.Context, height uint64, committee uint64) (*RpcDexBatch, error)
	NextDexBatchByHeight(ctx context.Context, height uint64, committee uint64) (*RpcDexBatch, error)
	AllParams(ctx context.Context, height uint64) (*RpcAllParams, error)
	FeeParams(ctx context.Context, height uint64) (*FeeParams, error)
	ConParams(ctx context.Context, height uint64) (*ConsensusParams, error)
	ValParams(ctx context.Context, height uint64) (*ValidatorParams, error)
	GovParams(ctx context.Context, height uint64) (*GovParams, error)
	Validators(ctx context.Context, height uint64) ([]*RpcValidator, error)
	NonSigners(ctx context.Context, height uint64) ([]*RpcNonSigner, error)
	CommitteeData(ctx context.Context, chainID uint64, height uint64) (*RpcCommitteeData, error)
	CommitteesData(ctx context.Context, height uint64) ([]*RpcCommitteeData, error)
	SubsidizedCommittees(ctx context.Context, height uint64) ([]uint64, error)
	RetiredCommittees(ctx context.Context, height uint64) ([]uint64, error)
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

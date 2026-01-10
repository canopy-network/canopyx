package activity

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/alitto/pond/v2"
	"go.uber.org/zap"

	adminstore "github.com/canopy-network/canopyx/pkg/db/admin"
	globalstore "github.com/canopy-network/canopyx/pkg/db/global"
	"github.com/canopy-network/canopyx/pkg/redis"
	"github.com/canopy-network/canopyx/pkg/rpc"
	temporalclient "github.com/canopy-network/canopyx/pkg/temporal"
)

type Context struct {
	ChainID uint64
	Logger  *zap.Logger
	// Database stores
	AdminDB  adminstore.Store
	GlobalDB globalstore.Store // New single-DB architecture (replaces per-chain DB)
	// For RPC calls to the blockchain
	RPCFactory rpc.Factory
	RPCOpts    rpc.Opts
	// For scheduling workflows
	ChainClient *temporalclient.ChainClient
	// For publishing real-time events
	RedisClient *redis.Client
	// WorkerMaxParallelism allows overriding the default worker pool size.
	WorkerMaxParallelism int
	workerPoolOnce       sync.Once
	workerPool           pond.Pool
	workerPoolSize       int
}

// GetGlobalDb returns the global database (must be initialized at startup).
// This replaces the old GetChainDb method as part of the single-DB architecture migration.
func (ac *Context) GetGlobalDb(ctx context.Context) (globalstore.Store, error) {
	if ac.GlobalDB == nil {
		return nil, fmt.Errorf("GlobalDB not initialized - this is a programming error")
	}
	return ac.GlobalDB, nil
}

// rpcClient creates and returns an RPC client using the provided endpoints and the context's RPCFactory or default factory.
// Uses lightweight GetChainRPCEndpoints query (2 columns instead of 25).
func (ac *Context) rpcClient(ctx context.Context) (rpc.Client, error) {
	// Get chain RPC endpoints only (lightweight query)
	ch, err := ac.AdminDB.GetChainRPCEndpoints(ctx, ac.ChainID)
	if err != nil {
		return nil, err
	}

	factory := ac.RPCFactory
	if factory == nil {
		factory = rpc.NewHTTPFactory(ac.RPCOpts)
	}

	return factory.NewClient(ch.RPCEndpoints), nil
}

// rpcClientForHeight returns an RPC client configured with endpoints that have height >= minHeight.
// Endpoints are ordered by height (highest first) for optimal failover.
// Returns an error if no endpoints have sufficient height.
func (ac *Context) rpcClientForHeight(ctx context.Context, minHeight uint64) (rpc.Client, error) {
	// Get endpoints with sufficient height from the database
	endpoints, err := ac.AdminDB.GetEndpointsWithMinHeight(ctx, ac.ChainID, minHeight)
	if err != nil {
		return nil, fmt.Errorf("failed to query endpoint health: %w", err)
	}

	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints available with height >= %d for chain %d", minHeight, ac.ChainID)
	}

	// Extract endpoint URLs, already ordered by height descending
	urls := make([]string, len(endpoints))
	for i, ep := range endpoints {
		urls[i] = ep.Endpoint
	}

	factory := ac.RPCFactory
	if factory == nil {
		factory = rpc.NewHTTPFactory(ac.RPCOpts)
	}

	return factory.NewClient(urls), nil
}

// WorkerPool returns a shared worker pool for parallel RPC fetching and batch operations.
// Pool size defaults to four workers per CPU (with sensible caps) but can be overridden.
// Use NewGroupContext() on the returned pool to get a subgroup for specific tasks.
func (ac *Context) WorkerPool(batchSize int) pond.Pool {
	ac.workerPoolOnce.Do(func() {
		maxWorkers := WorkerParallelism(ac.WorkerMaxParallelism)
		ac.workerPoolSize = maxWorkers
		queueSize := WorkerQueueSize(maxWorkers, batchSize)
		ac.workerPool = pond.NewPool(
			maxWorkers,
			pond.WithQueueSize(queueSize),
		)
	})

	return ac.workerPool
}

// WorkerPoolSize exposes the configured pool size for logging purposes.
func (ac *Context) WorkerPoolSize() int {
	if ac.workerPoolSize != 0 {
		return ac.workerPoolSize
	}
	return WorkerParallelism(ac.WorkerMaxParallelism)
}

// WorkerParallelism calculates the optimal parallelism for the worker pool.
func WorkerParallelism(override int) int {
	if override > 0 {
		if override > 512 {
			return 512
		}
		return override
	}

	n := runtime.NumCPU()
	if n < 1 {
		n = 1
	}

	// Use 4x CPU multiplier for increased throughput (target: 200k+ workflows/sec)
	parallelism := n * 4
	if parallelism < 2 {
		parallelism = 2
	}
	if parallelism > 512 {
		parallelism = 512
	}

	return parallelism
}

// WorkerQueueSize calculates the optimal queue size for the worker pool.
func WorkerQueueSize(parallelism, batchSize int) int {
	if parallelism < 1 {
		parallelism = 1
	}
	if batchSize < 1 {
		batchSize = 1
	}

	// Allow large batches to enqueue without blocking submissions.
	queue := parallelism * batchSize
	if queue < 4096 {
		queue = 4096
	}
	if queue > 262144 {
		queue = 262144
	}
	return queue
}

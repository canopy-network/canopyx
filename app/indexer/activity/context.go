package activity

import (
	"context"
	"runtime"
	"sync"

	"github.com/alitto/pond/v2"
	"go.uber.org/zap"

	adminstore "github.com/canopy-network/canopyx/pkg/db/admin"
	chainstore "github.com/canopy-network/canopyx/pkg/db/chain"
	"github.com/canopy-network/canopyx/pkg/redis"
	"github.com/canopy-network/canopyx/pkg/rpc"
	temporalclient "github.com/canopy-network/canopyx/pkg/temporal"
)

type Context struct {
	ChainID uint64
	Logger  *zap.Logger
	// Admin and per Chain DBs
	AdminDB adminstore.Store
	ChainDB chainstore.Store
	// For RPC calls to the blockchain
	RPCFactory rpc.Factory
	RPCOpts    rpc.Opts
	// For scheduling workflows
	TemporalClient *temporalclient.Client
	// For publishing real-time events
	RedisClient *redis.Client
	// SchedulerMaxParallelism allows overriding the default scheduling pool size.
	SchedulerMaxParallelism int
	schedulerPoolOnce       sync.Once
	schedulerPool           pond.Pool
	schedulerPoolSize       int
}

// GetChainDb return chain db or create it to return
func (ac *Context) GetChainDb(ctx context.Context, chainID uint64) (chainstore.Store, error) {
	if ac.ChainDB != nil {
		return ac.ChainDB, nil

	}

	chainDB, chainDBErr := chainstore.New(ctx, ac.Logger, chainID)
	if chainDBErr != nil {
		return nil, chainDBErr
	}

	ac.ChainDB = chainDB

	return ac.ChainDB, nil
}

// rpcClient creates and returns an RPC client using the provided endpoints and the context's RPCFactory or default factory.
func (ac *Context) rpcClient(ctx context.Context) (rpc.Client, error) {
	// Get chain metadata
	ch, err := ac.AdminDB.GetChain(ctx, ac.ChainID)
	if err != nil {
		return nil, err
	}

	factory := ac.RPCFactory
	if factory == nil {
		factory = rpc.NewHTTPFactory(ac.RPCOpts)
	}

	return factory.NewClient(ch.RPCEndpoints), nil
}

// schedulerBatchPool returns a shared worker pool for batch scheduling activities.
// Pool size defaults to four workers per CPU (with sensible caps) but can be overridden.
func (ac *Context) schedulerBatchPool(batchSize int) pond.Pool {
	ac.schedulerPoolOnce.Do(func() {
		maxWorkers := SchedulerParallelism(ac.SchedulerMaxParallelism)
		ac.schedulerPoolSize = maxWorkers
		queueSize := SchedulerQueueSize(maxWorkers, batchSize)
		ac.schedulerPool = pond.NewPool(
			maxWorkers,
			pond.WithQueueSize(queueSize),
		)
	})

	return ac.schedulerPool
}

// SchedulerPoolSize exposes the configured pool size for logging purposes.
func (ac *Context) SchedulerPoolSize() int {
	if ac.schedulerPoolSize != 0 {
		return ac.schedulerPoolSize
	}
	return SchedulerParallelism(ac.SchedulerMaxParallelism)
}

// SchedulerParallelism calculates the optimal parallelism for the scheduler pool.
func SchedulerParallelism(override int) int {
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

// SchedulerQueueSize calculates the optimal queue size for the scheduler pool.
func SchedulerQueueSize(parallelism, batchSize int) int {
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

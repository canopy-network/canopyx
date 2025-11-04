package activity

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/alitto/pond/v2"
	"github.com/puzpuzpuz/xsync/v4"
	"go.uber.org/zap"

	adminstore "github.com/canopy-network/canopyx/pkg/db/admin"
	chainstore "github.com/canopy-network/canopyx/pkg/db/chain"
	"github.com/canopy-network/canopyx/pkg/redis"
	"github.com/canopy-network/canopyx/pkg/rpc"
	temporalclient "github.com/canopy-network/canopyx/pkg/temporal"
)

type Context struct {
	Logger *zap.Logger
	// Admin and per Chain DBs
	AdminDB  adminstore.Store
	ChainsDB *xsync.Map[string, chainstore.Store]
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

// NewChainDb returns a chain store instance for the provided chain ID.
func (c *Context) NewChainDb(ctx context.Context, chainID uint64) (chainstore.Store, error) {
	chainIDStr := fmt.Sprintf("%d", chainID)
	if chainDb, ok := c.ChainsDB.Load(chainIDStr); ok {
		// chainDb is already loaded
		return chainDb, nil
	}

	chainDB, chainDBErr := chainstore.New(ctx, c.Logger, chainID)
	if chainDBErr != nil {
		return nil, chainDBErr
	}

	c.ChainsDB.Store(chainIDStr, chainDB)

	return chainDB, nil
}

// schedulerBatchPool returns a shared worker pool for batch scheduling activities.
// Pool size defaults to four workers per CPU (with sensible caps) but can be overridden.
func (c *Context) schedulerBatchPool(batchSize int) pond.Pool {
	c.schedulerPoolOnce.Do(func() {
		maxWorkers := SchedulerParallelism(c.SchedulerMaxParallelism)
		c.schedulerPoolSize = maxWorkers
		queueSize := SchedulerQueueSize(maxWorkers, batchSize)
		c.schedulerPool = pond.NewPool(
			maxWorkers,
			pond.WithQueueSize(queueSize),
		)
	})

	return c.schedulerPool
}

// SchedulerPoolSize exposes the configured pool size for logging purposes.
func (c *Context) SchedulerPoolSize() int {
	if c.schedulerPoolSize != 0 {
		return c.schedulerPoolSize
	}
	return SchedulerParallelism(c.SchedulerMaxParallelism)
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

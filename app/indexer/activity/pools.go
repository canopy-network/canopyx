package activity

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// IndexPools indexes pools for a given block height.
// Fetches all pools from RPC, converts to database models, and inserts to staging table.
// Returns output containing the number of indexed pools and execution duration in milliseconds.
func (c *Context) IndexPools(ctx context.Context, in types.IndexPoolsInput) (types.IndexPoolsOutput, error) {
	start := time.Now()

	// Get chain configuration
	ch, err := c.AdminDB.GetChain(ctx, in.ChainID)
	if err != nil {
		return types.IndexPoolsOutput{}, err
	}

	// Acquire (or ping) the chain DB to validate it exists
	chainDb, chainDbErr := c.NewChainDb(ctx, in.ChainID)
	if chainDbErr != nil {
		return types.IndexPoolsOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	// Fetch all pools from RPC
	// This returns ALL pools across all nested chains managed by this blockchain
	cli := c.rpcClient(ch.RPCEndpoints)
	rpcPools, err := cli.Pools(ctx)
	if err != nil {
		return types.IndexPoolsOutput{}, err
	}

	// Convert RPC pools to database models
	// Each pool has its own chain_id (from the blockchain's nested chain structure)
	// We index all pools at each height (snapshot pattern)
	pools := make([]*indexer.Pool, 0, len(rpcPools))
	for _, rpcPool := range rpcPools {
		pool := rpcPool.ToPool(in.Height)
		pool.HeightTime = in.BlockTime
		// Calculate the derived pool ID fields based on ChainID
		pool.CalculatePoolIDs()
		pools = append(pools, pool)
	}

	numPools := uint32(len(pools))
	c.Logger.Debug("IndexPools fetched from RPC",
		zap.Uint64("height", in.Height),
		zap.Uint32("numPools", numPools))

	// Insert pools to staging table (two-phase commit pattern)
	if err := chainDb.InsertPoolsStaging(ctx, pools); err != nil {
		return types.IndexPoolsOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.IndexPoolsOutput{
		NumPools:   numPools,
		DurationMs: durationMs,
	}, nil
}

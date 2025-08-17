package activity

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"

	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"go.temporal.io/sdk/temporal"

	"github.com/canopy-network/canopyx/pkg/rpc"
)

// IndexTransactions indexes transactions for a given block.
func (c *Context) IndexTransactions(ctx context.Context, in types.IndexBlockInput) (uint32, error) {
	ch, err := c.IndexerDB.GetChain(ctx, in.ChainID)
	if err != nil {
		return 0, err
	}

	// Acquire (or ping) the chain DB just to validate it exists.
	chainDb, chainDbErr := c.NewChainDb(ctx, in.ChainID)
	if chainDbErr != nil {
		return 0, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	cli := rpc.NewHTTPWithOpts(rpc.Opts{Endpoints: ch.RPCEndpoints, RPS: 20, Burst: 40, BreakerFailures: 3, BreakerCooldown: 5 * time.Second})
	txs, txsRaw, err := cli.TxsByHeight(ctx, in.Height)
	if err != nil {
		return 0, err
	}

	numTxs := uint32(len(txs))

	insertTxsErr := indexer.InsertTransactions(ctx, chainDb.Db, txs, txsRaw)
	if insertTxsErr != nil {
		return 0, insertTxsErr
	}

	return numTxs, nil
}

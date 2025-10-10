package activity

import (
	"context"

	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"go.temporal.io/sdk/temporal"
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

	cli := c.rpcClient(ch.RPCEndpoints)
	txs, txsRaw, err := cli.TxsByHeight(ctx, in.Height)
	if err != nil {
		return 0, err
	}

	numTxs := uint32(len(txs))

	if err := chainDb.InsertTransactions(ctx, txs, txsRaw); err != nil {
		return 0, err
	}

	return numTxs, nil
}

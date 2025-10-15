package activity

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// IndexTransactions indexes transactions for a given block.
// Returns output containing the number of indexed transactions and execution duration in milliseconds.
func (c *Context) IndexTransactions(ctx context.Context, in types.IndexBlockInput) (types.IndexTransactionsOutput, error) {
	start := time.Now()

	ch, err := c.IndexerDB.GetChain(ctx, in.ChainID)
	if err != nil {
		return types.IndexTransactionsOutput{}, err
	}

	// Acquire (or ping) the chain DB just to validate it exists.
	chainDb, chainDbErr := c.NewChainDb(ctx, in.ChainID)
	if chainDbErr != nil {
		return types.IndexTransactionsOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	cli := c.rpcClient(ch.RPCEndpoints)
	txs, txsRaw, err := cli.TxsByHeight(ctx, in.Height)
	if err != nil {
		return types.IndexTransactionsOutput{}, err
	}

	numTxs := uint32(len(txs))
	c.Logger.Debug("IndexTransactions fetched from RPC",
		zap.Uint64("height", in.Height),
		zap.Uint32("numTxs", numTxs),
		zap.Int("txsRaw_len", len(txsRaw)))

	if err := chainDb.InsertTransactions(ctx, txs, txsRaw); err != nil {
		return types.IndexTransactionsOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.IndexTransactionsOutput{NumTxs: numTxs, DurationMs: durationMs}, nil
}

package activity

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/pkg/indexer/types"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// IndexTransactions indexes transactions for a given block.
// Returns output containing the number of indexed transactions, counts by type, and execution duration in milliseconds.
func (c *Context) IndexTransactions(ctx context.Context, in types.IndexTransactionsInput) (types.IndexTransactionsOutput, error) {
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

	// Fetch and parse transactions from RPC (single-table design)
	cli := c.rpcClient(ch.RPCEndpoints)
	txs, err := cli.TxsByHeight(ctx, in.Height)
	if err != nil {
		return types.IndexTransactionsOutput{}, err
	}

	// Populate HeightTime field for all transactions using the block timestamp
	for i := range txs {
		txs[i].HeightTime = in.BlockTime
	}

	// Count transactions by type for analytics
	txCountsByType := make(map[string]uint32)
	for _, tx := range txs {
		txCountsByType[tx.MessageType]++
	}

	numTxs := uint32(len(txs))
	c.Logger.Debug("IndexTransactions fetched from RPC",
		zap.Uint64("height", in.Height),
		zap.Uint32("numTxs", numTxs),
		zap.Any("txCountsByType", txCountsByType))

	// Insert transactions to staging table (two-phase commit pattern)
	if err := chainDb.InsertTransactionsStaging(ctx, txs); err != nil {
		return types.IndexTransactionsOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.IndexTransactionsOutput{
		NumTxs:         numTxs,
		TxCountsByType: txCountsByType,
		DurationMs:     durationMs,
	}, nil
}

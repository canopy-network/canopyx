package activity

import (
	"context"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	indexer "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"github.com/canopy-network/canopyx/pkg/db/transform"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// IndexTransactions indexes transactions for a given block.
// Returns output containing the number of indexed transactions, counts by type, and execution duration in milliseconds.
func (ac *Context) IndexTransactions(ctx context.Context, in types.ActivityIndexAtHeight) (types.ActivityIndexTransactionsOutput, error) {
	start := time.Now()

	cli, err := ac.rpcClient(ctx)
	if err != nil {
		return types.ActivityIndexTransactionsOutput{}, err
	}

	// Acquire (or ping) the chain DB just to validate it exists.
	chainDb, chainDbErr := ac.GetChainDb(ctx, ac.ChainID)
	if chainDbErr != nil {
		return types.ActivityIndexTransactionsOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	// Fetch and parse transactions from RPC
	rpcTxs, err := cli.TxsByHeight(ctx, in.Height)
	if err != nil {
		return types.ActivityIndexTransactionsOutput{}, err
	}

	// Convert RPC transactions to indexer models
	txs := make([]*indexer.Transaction, 0, len(rpcTxs))
	for _, rpcTx := range rpcTxs {
		tx, err := transform.Transaction(rpcTx)
		if err != nil {
			// Fail fast - conversion errors mean corrupted/incomplete data
			return types.ActivityIndexTransactionsOutput{}, fmt.Errorf("convert transaction at height %d, hash %s: %w", in.Height, rpcTx.TxHash, err)
		}
		// Populate the HeightTime field using the block timestamp
		tx.HeightTime = in.BlockTime
		txs = append(txs, tx)
	}

	// Count transactions by type for analytics
	txCountsByType := make(map[string]uint32)
	for _, tx := range txs {
		txCountsByType[tx.MessageType]++
	}

	numTxs := uint32(len(txs))
	ac.Logger.Debug("IndexTransactions fetched from RPC",
		zap.Uint64("height", in.Height),
		zap.Uint32("numTxs", numTxs),
		zap.Any("txCountsByType", txCountsByType))

	// Insert transactions to staging table (two-phase commit pattern)
	if err := chainDb.InsertTransactionsStaging(ctx, txs); err != nil {
		return types.ActivityIndexTransactionsOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.ActivityIndexTransactionsOutput{
		NumTxs:         numTxs,
		TxCountsByType: txCountsByType,
		DurationMs:     durationMs,
	}, nil
}

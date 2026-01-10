package activity

import (
	"context"
	"fmt"
	globalstore "github.com/canopy-network/canopyx/pkg/db/global"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"github.com/canopy-network/canopyx/pkg/db/transform"
)

// indexTransactionsFromBlob indexes transactions for a given block.
func (ac *Context) indexTransactionsFromBlob(ctx context.Context, chainDb globalstore.Store, height uint64, heightTime time.Time, currentData *blobData) (types.ActivityIndexTransactionsOutput, float64, error) {
	start := time.Now()
	txCountsByType := make(map[string]uint32)
	txs := make([]*indexermodels.Transaction, 0, len(currentData.block.Transactions))
	for _, rpcTx := range currentData.block.Transactions {
		tx, err := transform.Transaction(rpcTx)
		if err != nil {
			return types.ActivityIndexTransactionsOutput{}, 0, fmt.Errorf("convert transaction at height %d, hash %s: %w", height, rpcTx.TxHash, err)
		}
		tx.HeightTime = heightTime
		txs = append(txs, tx)
		txCountsByType[tx.MessageType]++
	}
	if len(txs) > 0 {
		if err := chainDb.InsertTransactions(ctx, txs); err != nil {
			return types.ActivityIndexTransactionsOutput{}, 0, err
		}
	}
	out := types.ActivityIndexTransactionsOutput{
		NumTxs:         uint32(len(txs)),
		TxCountsByType: txCountsByType,
	}
	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return out, durationMs, nil
}

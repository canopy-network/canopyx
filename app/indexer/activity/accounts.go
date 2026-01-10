package activity

import (
	"context"
	"encoding/hex"
	"time"

	globalstore "github.com/canopy-network/canopyx/pkg/db/global"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"

	"github.com/canopy-network/canopyx/app/indexer/types"
)

// indexAccountsFromBlob indexes account balance changes for a given block using the snapshot-on-change pattern.
func (ac *Context) indexAccountsFromBlob(ctx context.Context, chainDb globalstore.Store, height uint64, heightTime time.Time, currentData *blobData, previousData *blobData, events *blobEventMaps) (types.ActivityIndexAccountsOutput, float64, error) {
	start := time.Now()
	prevAccountMap := make(map[string]uint64, len(previousData.accounts))
	for _, acc := range previousData.accounts {
		prevAccountMap[hex.EncodeToString(acc.Address)] = acc.Amount
	}
	changedAccounts := make([]*indexermodels.Account, 0)
	var numAccountsNew uint32
	for _, curr := range currentData.accounts {
		addrHex := hex.EncodeToString(curr.Address)
		prevAmount, existed := prevAccountMap[addrHex]
		rewardAmount := events.reward[addrHex]
		slashAmount := events.slash[addrHex]

		if curr.Amount != prevAmount || rewardAmount > 0 || slashAmount > 0 {
			changedAccounts = append(changedAccounts, &indexermodels.Account{
				Address:    addrHex,
				Amount:     curr.Amount,
				Rewards:    rewardAmount,
				Slashes:    slashAmount,
				Height:     height,
				HeightTime: heightTime,
			})
			if !existed {
				numAccountsNew++
			}
		}
	}
	if len(changedAccounts) > 0 {
		if err := chainDb.InsertAccounts(ctx, changedAccounts); err != nil {
			return types.ActivityIndexAccountsOutput{}, 0, err
		}
	}
	out := types.ActivityIndexAccountsOutput{
		NumAccounts:    uint32(len(changedAccounts)),
		NumAccountsNew: numAccountsNew,
	}
	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return out, durationMs, nil
}

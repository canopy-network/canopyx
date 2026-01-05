package activity

import (
	"context"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	sdktemporal "go.temporal.io/sdk/temporal"
)

// UpdateTVLSnapshot computes the current TVL for this chain and updates the hourly snapshot.
// This is called after each block is indexed, so the snapshot is always fresh.
// ReplacingMergeTree deduplication handles repeated updates to the same hour.
func (ac *Context) UpdateTVLSnapshot(ctx context.Context, in types.ActivityUpdateTVLSnapshotInput) (types.ActivityUpdateTVLSnapshotOutput, error) {
	start := time.Now()

	chainDB, err := ac.GetChainDb(ctx, ac.ChainID)
	if err != nil {
		return types.ActivityUpdateTVLSnapshotOutput{}, sdktemporal.NewApplicationErrorWithCause(
			"unable to acquire chain database", "chain_db_error", err)
	}

	// Compute TVL for each entity type from production tables
	accountsTVL, err := chainDB.SumAccountBalances(ctx)
	if err != nil {
		return types.ActivityUpdateTVLSnapshotOutput{}, sdktemporal.NewApplicationErrorWithCause(
			"failed to compute accounts TVL", "tvl_error", err)
	}

	poolsTVL, err := chainDB.SumPoolAmounts(ctx)
	if err != nil {
		return types.ActivityUpdateTVLSnapshotOutput{}, sdktemporal.NewApplicationErrorWithCause(
			"failed to compute pools TVL", "tvl_error", err)
	}

	validatorsTVL, err := chainDB.SumValidatorStakes(ctx)
	if err != nil {
		return types.ActivityUpdateTVLSnapshotOutput{}, sdktemporal.NewApplicationErrorWithCause(
			"failed to compute validators TVL", "tvl_error", err)
	}

	ordersTVL, err := chainDB.SumOpenOrderAmounts(ctx)
	if err != nil {
		return types.ActivityUpdateTVLSnapshotOutput{}, sdktemporal.NewApplicationErrorWithCause(
			"failed to compute orders TVL", "tvl_error", err)
	}

	dexOrdersTVL, err := chainDB.SumActiveDexOrderAmounts(ctx)
	if err != nil {
		return types.ActivityUpdateTVLSnapshotOutput{}, sdktemporal.NewApplicationErrorWithCause(
			"failed to compute dex_orders TVL", "tvl_error", err)
	}

	totalTVL := accountsTVL + poolsTVL + validatorsTVL + ordersTVL + dexOrdersTVL

	// Create snapshot for the current hour
	now := time.Now().UTC()
	snapshotHour := in.BlockTime.Truncate(time.Hour)

	snapshot := &indexermodels.TVLSnapshot{
		ChainID:        ac.ChainID,
		SnapshotHour:   snapshotHour,
		SnapshotHeight: in.Height,
		AccountsTVL:    accountsTVL,
		PoolsTVL:       poolsTVL,
		ValidatorsTVL:  validatorsTVL,
		OrdersTVL:      ordersTVL,
		DexOrdersTVL:   dexOrdersTVL,
		TotalTVL:       totalTVL,
		ComputedAt:     now,
		UpdatedAt:      now,
	}

	// Insert into cross-chain global table
	if err := ac.CrossChainDB.InsertTVLSnapshots(ctx, []*indexermodels.TVLSnapshot{snapshot}); err != nil {
		return types.ActivityUpdateTVLSnapshotOutput{}, sdktemporal.NewApplicationErrorWithCause(
			fmt.Sprintf("failed to insert TVL snapshot for chain %d", ac.ChainID), "tvl_insert_error", err)
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0

	return types.ActivityUpdateTVLSnapshotOutput{
		SnapshotHour: snapshotHour,
		TotalTVL:     totalTVL,
		DurationMs:   durationMs,
	}, nil
}

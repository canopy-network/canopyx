package activity

import (
	"context"
	"fmt"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// IndexCommittees indexes committee data for a given block height.
// Committees are only inserted when their data differs from the previous height to maintain a sparse historical record.
// This follows the RPC(H) vs RPC(H-1) pattern for change detection, never querying the database.
// Returns output indicating the number of changed committees and execution duration in milliseconds.
func (c *Context) IndexCommittees(ctx context.Context, in types.IndexCommitteesInput) (types.IndexCommitteesOutput, error) {
	start := time.Now()

	ch, err := c.AdminDB.GetChain(ctx, in.ChainID)
	if err != nil {
		return types.IndexCommitteesOutput{}, err
	}

	// Acquire (or ping) the chain DB to validate it exists
	chainDb, chainDbErr := c.NewChainDb(ctx, in.ChainID)
	if chainDbErr != nil {
		return types.IndexCommitteesOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	// Fetch committees at both H and H-1 from RPC
	cli := c.rpcClient(ch.RPCEndpoints)

	// Fetch committees at current height (H)
	committeesAtH, err := cli.CommitteesDataByHeight(ctx, in.Height)
	if err != nil {
		return types.IndexCommitteesOutput{}, fmt.Errorf("fetch committees at height %d: %w", in.Height, err)
	}

	// Also fetch subsidized and retired lists at H
	subsidizedAtH, err := cli.SubsidizedCommitteesByHeight(ctx, in.Height)
	if err != nil {
		return types.IndexCommitteesOutput{}, fmt.Errorf("fetch subsidized committees at height %d: %w", in.Height, err)
	}

	retiredAtH, err := cli.RetiredCommitteesByHeight(ctx, in.Height)
	if err != nil {
		return types.IndexCommitteesOutput{}, fmt.Errorf("fetch retired committees at height %d: %w", in.Height, err)
	}

	// Build lookup maps for subsidized and retired status at H
	subsidizedMapAtH := make(map[uint64]bool)
	for _, chainID := range subsidizedAtH {
		subsidizedMapAtH[chainID] = true
	}

	retiredMapAtH := make(map[uint64]bool)
	for _, chainID := range retiredAtH {
		retiredMapAtH[chainID] = true
	}

	// Convert RPC committees at H to entity models with status flags
	currentCommittees := make(map[uint64]*indexermodels.Committee)
	for _, rpcCommittee := range committeesAtH {
		currentCommittees[rpcCommittee.ChainID] = &indexermodels.Committee{
			ChainID:                rpcCommittee.ChainID,
			LastRootHeightUpdated:  rpcCommittee.LastRootHeightUpdated,
			LastChainHeightUpdated: rpcCommittee.LastChainHeightUpdated,
			NumberOfSamples:        rpcCommittee.NumberOfSamples,
			Subsidized:             subsidizedMapAtH[rpcCommittee.ChainID],
			Retired:                retiredMapAtH[rpcCommittee.ChainID],
			Height:                 in.Height,
			HeightTime:             in.BlockTime,
		}
	}

	// Determine which committees changed by comparing with H-1
	var changedCommittees []*indexermodels.Committee
	if in.Height == 1 {
		// Genesis block: always insert all committees
		for _, committee := range currentCommittees {
			changedCommittees = append(changedCommittees, committee)
		}
		c.Logger.Debug("IndexCommittees genesis block - inserting all committees",
			zap.Uint64("height", in.Height),
			zap.Int("numCommittees", len(changedCommittees)))
	} else {
		// Fetch committees at previous height (H-1)
		committeesAtH1, err := cli.CommitteesDataByHeight(ctx, in.Height-1)
		if err != nil {
			return types.IndexCommitteesOutput{}, fmt.Errorf("fetch committees at height %d: %w", in.Height-1, err)
		}

		// Fetch subsidized and retired lists at H-1
		subsidizedAtH1, err := cli.SubsidizedCommitteesByHeight(ctx, in.Height-1)
		if err != nil {
			return types.IndexCommitteesOutput{}, fmt.Errorf("fetch subsidized committees at height %d: %w", in.Height-1, err)
		}

		retiredAtH1, err := cli.RetiredCommitteesByHeight(ctx, in.Height-1)
		if err != nil {
			return types.IndexCommitteesOutput{}, fmt.Errorf("fetch retired committees at height %d: %w", in.Height-1, err)
		}

		// Build lookup maps for subsidized and retired status at H-1
		subsidizedMapAtH1 := make(map[uint64]bool)
		for _, chainID := range subsidizedAtH1 {
			subsidizedMapAtH1[chainID] = true
		}

		retiredMapAtH1 := make(map[uint64]bool)
		for _, chainID := range retiredAtH1 {
			retiredMapAtH1[chainID] = true
		}

		// Convert RPC committees at H-1 to entity models
		prevMap := make(map[uint64]*indexermodels.Committee)
		for _, rpcCommittee := range committeesAtH1 {
			prevMap[rpcCommittee.ChainID] = &indexermodels.Committee{
				ChainID:                rpcCommittee.ChainID,
				LastRootHeightUpdated:  rpcCommittee.LastRootHeightUpdated,
				LastChainHeightUpdated: rpcCommittee.LastChainHeightUpdated,
				NumberOfSamples:        rpcCommittee.NumberOfSamples,
				Subsidized:             subsidizedMapAtH1[rpcCommittee.ChainID],
				Retired:                retiredMapAtH1[rpcCommittee.ChainID],
			}
		}

		// Compare each committee at H with H-1 to detect changes
		for chainID, currentCommittee := range currentCommittees {
			prevCommittee, existed := prevMap[chainID]

			// New committee (didn't exist at H-1)
			if !existed {
				changedCommittees = append(changedCommittees, currentCommittee)
				continue
			}

			// Existing committee - check if any field changed
			if !committeesEqual(prevCommittee, currentCommittee) {
				changedCommittees = append(changedCommittees, currentCommittee)
			}
		}

		// Check for committees that were removed (existed at H-1 but not at H)
		// In this case, we don't insert anything since we only track active committees

		c.Logger.Debug("IndexCommittees compared RPC(H) vs RPC(H-1)",
			zap.Uint64("height", in.Height),
			zap.Int("committeesAtH", len(currentCommittees)),
			zap.Int("committeesAtH1", len(prevMap)),
			zap.Int("changedCommittees", len(changedCommittees)))
	}

	// Only insert if committees changed (sparse insert)
	if len(changedCommittees) > 0 {
		if err := chainDb.InsertCommitteesStaging(ctx, changedCommittees); err != nil {
			return types.IndexCommitteesOutput{}, err
		}
		c.Logger.Info("Committees changed, inserted to staging",
			zap.Uint64("height", in.Height),
			zap.Uint64("chainID", in.ChainID),
			zap.Int("numChanged", len(changedCommittees)))
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.IndexCommitteesOutput{
		NumCommittees: uint32(len(changedCommittees)),
		DurationMs:    durationMs,
	}, nil
}

// committeesEqual compares all fields of two Committee instances (excluding Height and HeightTime).
// Returns true if all committee data values are identical.
func committeesEqual(a, b *indexermodels.Committee) bool {
	return a.ChainID == b.ChainID &&
		a.LastRootHeightUpdated == b.LastRootHeightUpdated &&
		a.LastChainHeightUpdated == b.LastChainHeightUpdated &&
		a.NumberOfSamples == b.NumberOfSamples &&
		a.Subsidized == b.Subsidized &&
		a.Retired == b.Retired
}

package activity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/canopy-network/canopy/lib"
	"github.com/canopy-network/canopyx/app/indexer/types"
	indexermodels "github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// bytesToHex converts a byte slice to a hex-encoded string.
// Returns empty string if bytes are nil or empty.
func bytesToHex(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return fmt.Sprintf("%x", b)
}

// IndexCommittees indexes committee data for a given block height.
// Committees are only inserted when their data differs from the previous height to maintain a sparse historical record.
// This follows the RPC(H) vs RPC(H-1) pattern for change detection, never querying the database.
// Returns output indicating the number of changed committees and execution duration in milliseconds.
func (ac *Context) IndexCommittees(ctx context.Context, in types.ActivityIndexAtHeight) (types.ActivityIndexCommitteesOutput, error) {
	start := time.Now()

	// Get RPC client with height-aware endpoint selection
	cli, err := ac.rpcClientForHeight(ctx, in.Height)
	if err != nil {
		return types.ActivityIndexCommitteesOutput{}, err
	}

	chainDb, chainDbErr := ac.GetChainDb(ctx, ac.ChainID)
	if chainDbErr != nil {
		return types.ActivityIndexCommitteesOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	// Parallel RPC fetch using shared worker pool for performance
	var (
		committeesAtH   []*lib.CommitteeData
		committeesAtH1  []*lib.CommitteeData
		subsidizedAtH   []uint64
		subsidizedAtH1  []uint64
		retiredAtH      []uint64
		retiredAtH1     []uint64
		committeesErr   error
		committeesH1Err error
		subsidizedErr   error
		subsidizedH1Err error
		retiredErr      error
		retiredH1Err    error
	)

	// Get a subgroup from the shared worker pool for parallel RPC fetching (6 workers)
	pool := ac.WorkerPool(6)
	group := pool.NewGroupContext(ctx)
	groupCtx := group.Context()

	// Worker 1: Fetch committees at height H
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		committeesAtH, committeesErr = cli.CommitteesDataByHeight(groupCtx, in.Height)
	})

	// Worker 2: Fetch subsidized committees at height H
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		subsidizedAtH, subsidizedErr = cli.SubsidizedCommitteesByHeight(groupCtx, in.Height)
	})

	// Worker 3: Fetch retired committees at height H
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		retiredAtH, retiredErr = cli.RetiredCommitteesByHeight(groupCtx, in.Height)
	})

	// Worker 4: Fetch committees at height H-1
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		if in.Height <= 1 {
			committeesAtH1 = make([]*lib.CommitteeData, 0)
			return
		}
		committeesAtH1, committeesH1Err = cli.CommitteesDataByHeight(groupCtx, in.Height-1)
	})

	// Worker 5: Fetch subsidized committees at height H-1
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		if in.Height <= 1 {
			subsidizedAtH1 = make([]uint64, 0)
			return
		}
		subsidizedAtH1, subsidizedH1Err = cli.SubsidizedCommitteesByHeight(groupCtx, in.Height-1)
	})

	// Worker 6: Fetch retired committees at height H-1
	group.Submit(func() {
		if err := groupCtx.Err(); err != nil {
			return
		}
		if in.Height <= 1 {
			retiredAtH1 = make([]uint64, 0)
			return
		}
		retiredAtH1, retiredH1Err = cli.RetiredCommitteesByHeight(groupCtx, in.Height-1)
	})

	// Wait for all workers to complete
	if err := group.Wait(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, pond.ErrGroupStopped) {
		ac.Logger.Warn("parallel RPC fetch encountered error",
			zap.Uint64("chainId", ac.ChainID),
			zap.Uint64("height", in.Height),
			zap.Error(err),
		)
	}

	// Check for errors
	if committeesErr != nil {
		return types.ActivityIndexCommitteesOutput{}, fmt.Errorf("fetch committees at height %d: %w", in.Height, committeesErr)
	}
	if subsidizedErr != nil {
		return types.ActivityIndexCommitteesOutput{}, fmt.Errorf("fetch subsidized committees at height %d: %w", in.Height, subsidizedErr)
	}
	if retiredErr != nil {
		return types.ActivityIndexCommitteesOutput{}, fmt.Errorf("fetch retired committees at height %d: %w", in.Height, retiredErr)
	}
	if committeesH1Err != nil {
		return types.ActivityIndexCommitteesOutput{}, fmt.Errorf("fetch committees at height %d: %w", in.Height-1, committeesH1Err)
	}
	if subsidizedH1Err != nil {
		return types.ActivityIndexCommitteesOutput{}, fmt.Errorf("fetch subsidized committees at height %d: %w", in.Height-1, subsidizedH1Err)
	}
	if retiredH1Err != nil {
		return types.ActivityIndexCommitteesOutput{}, fmt.Errorf("fetch retired committees at height %d: %w", in.Height-1, retiredH1Err)
	}

	// Calculate aggregate counts for all committees at this height
	numSubsidizedTotal := uint8(len(subsidizedAtH))
	numRetiredTotal := uint8(len(retiredAtH))

	// Convert RPC committees at H to entity models with aggregate counts
	currentCommittees := make(map[uint64]*indexermodels.Committee)
	for _, rpcCommittee := range committeesAtH {
		currentCommittees[rpcCommittee.ChainId] = &indexermodels.Committee{
			ChainID:                uint16(rpcCommittee.ChainId),
			LastRootHeightUpdated:  rpcCommittee.LastRootHeightUpdated,
			LastChainHeightUpdated: rpcCommittee.LastChainHeightUpdated,
			NumberOfSamples:        rpcCommittee.NumberOfSamples,
			Subsidized:             numSubsidizedTotal, // Total count across all committees
			Retired:                numRetiredTotal,    // Total count across all committees
			Height:                 in.Height,
			HeightTime:             in.BlockTime,
		}
	}

	// Determine which committees changed by comparing with H-1
	// Also count status breakdowns from all current committees
	var changedCommittees []*indexermodels.Committee
	var numCommitteesNew uint32

	if in.Height == 1 {
		// Genesis block: always insert all committees
		for _, committee := range currentCommittees {
			changedCommittees = append(changedCommittees, committee)
		}
		numCommitteesNew = uint32(len(currentCommittees))
		ac.Logger.Debug("IndexCommittees genesis block - inserting all committees",
			zap.Uint64("height", in.Height),
			zap.Int("numCommittees", len(changedCommittees)))
	} else {
		// Calculate aggregate counts for H-1
		numSubsidizedTotalH1 := uint8(len(subsidizedAtH1))
		numRetiredTotalH1 := uint8(len(retiredAtH1))

		// Convert RPC committees at H-1 to entity models
		prevMap := make(map[uint64]*indexermodels.Committee)
		for _, rpcCommittee := range committeesAtH1 {
			prevMap[rpcCommittee.ChainId] = &indexermodels.Committee{
				ChainID:                uint16(rpcCommittee.ChainId),
				LastRootHeightUpdated:  rpcCommittee.LastRootHeightUpdated,
				LastChainHeightUpdated: rpcCommittee.LastChainHeightUpdated,
				NumberOfSamples:        rpcCommittee.NumberOfSamples,
				Subsidized:             numSubsidizedTotalH1, // Total count at H-1
				Retired:                numRetiredTotalH1,    // Total count at H-1
			}
		}

		// Compare each committee at H with H-1 to detect changes
		for chainID, currentCommittee := range currentCommittees {
			prevCommittee, existed := prevMap[chainID]

			// New committee (didn't exist at H-1)
			if !existed {
				changedCommittees = append(changedCommittees, currentCommittee)
				numCommitteesNew++
				continue
			}

			// Existing committee - check if any field changed
			if !committeesEqual(prevCommittee, currentCommittee) {
				changedCommittees = append(changedCommittees, currentCommittee)
			}
		}

		// Check for committees that were removed (existed at H-1 but not at H)
		// In this case, we don't insert anything since we only track active committees

		ac.Logger.Debug("IndexCommittees compared RPC(H) vs RPC(H-1)",
			zap.Uint64("height", in.Height),
			zap.Int("committeesAtH", len(currentCommittees)),
			zap.Int("committeesAtH1", len(prevMap)),
			zap.Int("changedCommittees", len(changedCommittees)))
	}

	// Extract payment percents for all committees at height H BEFORE inserts
	// PaymentPercents track reward distribution for each committee
	var payments []*indexermodels.CommitteePayment
	for _, rpcCommittee := range committeesAtH {
		for _, pp := range rpcCommittee.PaymentPercents {
			payments = append(payments, &indexermodels.CommitteePayment{
				CommitteeID: rpcCommittee.ChainId,
				Address:     bytesToHex(pp.Address),
				Percent:     pp.Percent,
				Height:      in.Height,
				HeightTime:  in.BlockTime,
			})
		}
	}

	// Insert committees and payments to staging tables in PARALLEL
	// Each insert goes to a different table, so no conflicts
	insertPool := ac.WorkerPool(2) // 2 workers for 2 parallel inserts
	insertGroup := insertPool.NewGroupContext(ctx)
	insertCtx := insertGroup.Context()

	var (
		insertCommitteesErr error
		insertPaymentsErr   error
	)

	// Worker 1: Insert committees (if changed)
	if len(changedCommittees) > 0 {
		insertGroup.Submit(func() {
			if err := insertCtx.Err(); err != nil {
				return
			}
			insertCommitteesErr = chainDb.InsertCommitteesStaging(insertCtx, changedCommittees)
		})
	}

	// Worker 2: Insert payments (always insert, even if committees didn't change)
	if len(payments) > 0 {
		insertGroup.Submit(func() {
			if err := insertCtx.Err(); err != nil {
				return
			}
			insertPaymentsErr = chainDb.InsertCommitteePaymentsStaging(insertCtx, payments)
		})
	}

	// Wait for all inserts to complete (ignore pool errors, check individual errors below)
	_ = insertGroup.Wait()

	// Check for insert errors
	if insertCommitteesErr != nil {
		return types.ActivityIndexCommitteesOutput{}, fmt.Errorf("insert committees staging: %w", insertCommitteesErr)
	}
	if insertPaymentsErr != nil {
		return types.ActivityIndexCommitteesOutput{}, fmt.Errorf("insert committee payments: %w", insertPaymentsErr)
	}

	// Log results
	if len(changedCommittees) > 0 {
		ac.Logger.Info("Committees changed, inserted to staging",
			zap.Uint64("height", in.Height),
			zap.Uint64("chainID", ac.ChainID),
			zap.Int("numChanged", len(changedCommittees)))
	}
	if len(payments) > 0 {
		ac.Logger.Debug("Committee payments inserted",
			zap.Uint64("height", in.Height),
			zap.Int("numPayments", len(payments)))
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.ActivityIndexCommitteesOutput{
		NumCommittees:           uint32(len(changedCommittees)),
		NumCommitteesNew:        numCommitteesNew,
		NumCommitteesSubsidized: uint32(len(subsidizedAtH)),
		NumCommitteesRetired:    uint32(len(retiredAtH)),
		NumCommitteePayments:    uint32(len(payments)),
		DurationMs:              durationMs,
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

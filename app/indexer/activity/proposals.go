package activity

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// IndexProposals captures governance proposal data snapshots at regular intervals.
// Fetches current proposals state from RPC, converts to database models, and inserts directly to production table.
// Returns output containing the number of indexed proposals and execution duration in milliseconds.
//
// ARCHITECTURAL NOTE: This activity is executed via a scheduled workflow (every 5 minutes).
// Unlike other indexing activities, it does NOT use height-based indexing because the /v1/gov/proposals
// endpoint does not support height parameters - it always returns the current proposals state.
//
// Proposal snapshots are time-based and inserted directly to the production table (no staging).
// ReplacingMergeTree deduplicates by (proposal_hash, snapshot_time).
func (ac *Context) IndexProposals(ctx context.Context) (types.ActivityIndexProposalsOutput, error) {
	start := time.Now()

	// Get chain configuration
	cli, err := ac.rpcClient(ctx)
	if err != nil {
		return types.ActivityIndexProposalsOutput{}, err
	}

	// Acquire (or ping) the chain DB to validate it exists
	chainDb, chainDbErr := ac.GetChainDb(ctx, ac.ChainID)
	if chainDbErr != nil {
		return types.ActivityIndexProposalsOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	// Fetch current proposals state from RPC
	// NOTE: This endpoint does NOT support height parameter - always returns current state
	rpcProposals, err := cli.Proposals(ctx)
	if err != nil {
		return types.ActivityIndexProposalsOutput{}, err
	}

	// Convert RPC proposals to database models
	// Each proposal in the map becomes a separate snapshot row
	snapshotTime := time.Now()
	snapshots := make([]*indexer.ProposalSnapshot, 0, len(rpcProposals))
	for proposalHash, proposalData := range rpcProposals {
		snapshot := &indexer.ProposalSnapshot{
			ProposalHash: proposalHash,
			Proposal:     string(proposalData.Proposal), // Convert json.RawMessage to string
			Approve:      proposalData.Approve,
			SnapshotTime: snapshotTime,
		}
		snapshots = append(snapshots, snapshot)
	}

	numProposals := uint32(len(snapshots))
	ac.Logger.Info("IndexProposals captured snapshot",
		zap.Time("snapshotTime", snapshotTime),
		zap.Uint32("numProposals", numProposals))

	// Insert proposal snapshots directly to production table (no staging for time-based snapshots)
	// Note: This may be an empty slice if no active proposals, which is valid
	if err := chainDb.InsertProposalSnapshots(ctx, snapshots); err != nil {
		return types.ActivityIndexProposalsOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.ActivityIndexProposalsOutput{
		NumProposals: numProposals,
		DurationMs:   durationMs,
	}, nil
}

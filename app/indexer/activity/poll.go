package activity

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/zap"
)

// IndexPoll indexes governance poll data for a given block height.
// Fetches current poll state from RPC, converts to database models, and inserts to staging table.
// Returns output containing the number of indexed proposals and execution duration in milliseconds.
//
// ARCHITECTURAL NOTE: Unlike other indexing activities, this activity does NOT use the
// RPC(H) vs RPC(H-1) comparison pattern because the /v1/gov/poll endpoint does not support
// height parameters. It always returns the current poll state.
//
// This means we snapshot the current poll state at each height, even if it hasn't changed.
// This is a necessary architectural exception due to RPC endpoint limitations.
func (ac *Context) IndexPoll(ctx context.Context, in types.ActivityIndexAtHeight) (types.ActivityIndexPollOutput, error) {
	start := time.Now()

	// Get chain configuration
	cli, err := ac.rpcClient(ctx)
	if err != nil {
		return types.ActivityIndexPollOutput{}, err
	}

	// Acquire (or ping) the chain DB to validate it exists
	chainDb, chainDbErr := ac.GetChainDb(ctx, ac.ChainID)
	if chainDbErr != nil {
		return types.ActivityIndexPollOutput{}, temporal.NewApplicationErrorWithCause("unable to acquire chain database", "chain_db_error", chainDbErr)
	}

	// Fetch current poll state from RPC
	// NOTE: This endpoint does NOT support height parameter - always returns current state
	rpcPoll, err := cli.Poll(ctx)
	if err != nil {
		return types.ActivityIndexPollOutput{}, err
	}

	// Convert RPC poll results to database models
	// Each proposal in the poll map becomes a separate snapshot row
	snapshots := make([]*indexer.PollSnapshot, 0, len(rpcPoll))
	for proposalHash, pollResult := range rpcPoll {
		snapshot := &indexer.PollSnapshot{
			ProposalHash: proposalHash,
			Height:       in.Height,
			ProposalURL:  pollResult.ProposalURL,

			// Account voting stats
			AccountsApproveTokens:     pollResult.Accounts.ApproveTokens,
			AccountsRejectTokens:      pollResult.Accounts.RejectTokens,
			AccountsTotalVotedTokens:  pollResult.Accounts.TotalVotedTokens,
			AccountsTotalTokens:       pollResult.Accounts.TotalTokens,
			AccountsApprovePercentage: pollResult.Accounts.ApprovePercentage,
			AccountsRejectPercentage:  pollResult.Accounts.RejectPercentage,
			AccountsVotedPercentage:   pollResult.Accounts.VotedPercentage,

			// Validator voting stats
			ValidatorsApproveTokens:     pollResult.Validators.ApproveTokens,
			ValidatorsRejectTokens:      pollResult.Validators.RejectTokens,
			ValidatorsTotalVotedTokens:  pollResult.Validators.TotalVotedTokens,
			ValidatorsTotalTokens:       pollResult.Validators.TotalTokens,
			ValidatorsApprovePercentage: pollResult.Validators.ApprovePercentage,
			ValidatorsRejectPercentage:  pollResult.Validators.RejectPercentage,
			ValidatorsVotedPercentage:   pollResult.Validators.VotedPercentage,

			HeightTime: in.BlockTime,
		}
		snapshots = append(snapshots, snapshot)
	}

	numProposals := uint32(len(snapshots))
	ac.Logger.Debug("IndexPoll fetched from RPC",
		zap.Uint64("height", in.Height),
		zap.Uint32("numProposals", numProposals))

	// Insert poll snapshots to the staging table (two-phase commit pattern)
	// Note: This may be an empty slice if no active proposals, which is valid
	if err := chainDb.InsertPollSnapshotsStaging(ctx, snapshots); err != nil {
		return types.ActivityIndexPollOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.ActivityIndexPollOutput{
		NumProposals: numProposals,
		DurationMs:   durationMs,
	}, nil
}

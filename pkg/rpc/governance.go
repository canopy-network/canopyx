package rpc

import (
	"context"
	"fmt"
	"net/http"
)

// Poll returns the current governance poll state.
// This endpoint does NOT support height parameters - it always returns the current state.
// Returns a map of proposal hash to poll results, which may be empty if no active proposals.
//
// IMPORTANT: Unlike other RPC endpoints, /v1/gov/poll does not support historical queries.
// It always returns the current poll state from the server's in-memory state.
// This means we cannot use the typical RPC(H) vs RPC(H-1) comparison pattern.
// Instead, we snapshot the current poll state at each height.
func (c *HTTPClient) Poll(ctx context.Context) (RpcPoll, error) {
	var poll RpcPoll
	if err := c.doJSON(ctx, http.MethodGet, pollPath, nil, &poll); err != nil {
		return nil, fmt.Errorf("fetch poll: %w", err)
	}

	// If poll is nil (no active proposals), return an empty map
	if poll == nil {
		return make(RpcPoll), nil
	}

	return poll, nil
}

// RpcVoteStats represents vote statistics for a proposal.
// This matches the VoteStats structure from Canopy's governance system.
type RpcVoteStats struct {
	ApproveTokens     uint64 `json:"approveTokens"`    // Tokens voted 'yay'
	RejectTokens      uint64 `json:"rejectTokens"`     // Tokens voted 'nay'
	TotalVotedTokens  uint64 `json:"totalVotedTokens"` // Total tokens that voted
	TotalTokens       uint64 `json:"totalTokens"`      // Total tokens that could vote
	ApprovePercentage uint64 `json:"approvedPercent"`  // % approve (note: "approved" not "approve" in JSON)
	RejectPercentage  uint64 `json:"rejectPercent"`    // % reject
	VotedPercentage   uint64 `json:"votedPercent"`     // % participated
}

// RpcPollResult represents the current state of voting for a proposal.
// This matches the PollResult structure from Canopy's governance system.
type RpcPollResult struct {
	ProposalHash string       `json:"proposalHash"` // Hash of the proposal
	ProposalURL  string       `json:"proposalURL"`  // URL of the proposal document
	Accounts     RpcVoteStats `json:"accounts"`     // Vote statistics for accounts
	Validators   RpcVoteStats `json:"validators"`   // Vote statistics for validators
}

// RpcPoll represents a map of poll results keyed by proposal hash.
// This matches the Poll structure from Canopy's governance system.
// The map may be empty if there are no active proposals.
type RpcPoll map[string]RpcPollResult

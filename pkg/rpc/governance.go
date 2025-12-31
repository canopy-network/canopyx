package rpc

import (
	"context"
	"fmt"
	"net/http"

	"github.com/canopy-network/canopy/fsm"
)

// Poll returns the current governance poll state.
// This endpoint does NOT support height parameters - it always returns the current state.
// Returns a map of proposal hash to poll results, which may be empty if no active proposals.
// JSON from RPC is unmarshaled directly into fsm.Poll (protobuf types support JSON).
//
// IMPORTANT: Unlike other RPC endpoints, /v1/gov/poll does not support historical queries.
// It always returns the current poll state from the server's in-memory state.
// This means we cannot use the typical RPC(H) vs RPC(H-1) comparison pattern.
// Instead, we snapshot the current poll state at each height.
func (c *HTTPClient) Poll(ctx context.Context) (fsm.Poll, error) {
	var poll fsm.Poll
	if err := c.doJSON(ctx, http.MethodGet, pollPath, nil, &poll); err != nil {
		return nil, fmt.Errorf("fetch poll: %w", err)
	}

	// If poll is nil (no active proposals), return an empty map
	if poll == nil {
		return make(fsm.Poll), nil
	}

	return poll, nil
}

// Proposals returns the current governance proposals state.
// This endpoint does NOT support height parameters - it always returns the current state.
// Returns a map of proposal hash to proposal data, which may be empty if no proposals exist.
// JSON from RPC is unmarshaled directly into fsm.GovProposals (protobuf types support JSON).
//
// IMPORTANT: Unlike other RPC endpoints, /v1/gov/proposals does not support historical queries.
// It always returns the current proposals state from the server.
// This means we cannot use the typical RPC(H) vs RPC(H-1) comparison pattern.
// Instead, we snapshot the current proposals state at regular intervals.
func (c *HTTPClient) Proposals(ctx context.Context) (fsm.GovProposals, error) {
	var proposals fsm.GovProposals
	if err := c.doJSON(ctx, http.MethodGet, proposal, nil, &proposals); err != nil {
		return nil, fmt.Errorf("fetch proposals: %w", err)
	}

	// If proposals is nil (no active proposals), return an empty map
	if proposals == nil {
		return make(fsm.GovProposals), nil
	}

	return proposals, nil
}

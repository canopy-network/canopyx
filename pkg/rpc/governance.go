package rpc

import (
	"context"
	"fmt"
	"net/http"
)

const (
	pollPath = "/v1/gov/poll"
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

	// If poll is nil (no active proposals), return empty map
	if poll == nil {
		return make(RpcPoll), nil
	}

	return poll, nil
}

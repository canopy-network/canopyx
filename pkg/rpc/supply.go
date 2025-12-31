package rpc

import (
	"context"
	"fmt"
	"net/http"

	"github.com/canopy-network/canopy/fsm"
)

// SupplyByHeight queries the /v1/query/supply endpoint to retrieve token supply metrics at a specific height.
// JSON from RPC is unmarshaled directly into fsm.Supply (protobuf types support JSON).
// Callers should convert to indexer models if needed.
//
// Parameters:
//   - height: The block height to query supply for (0 = latest)
//
// Returns:
//   - *fsm.Supply: Supply metrics at the specified height including committee stake breakdowns
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) SupplyByHeight(ctx context.Context, height uint64) (*fsm.Supply, error) {
	var supply fsm.Supply
	req := QueryByHeightRequest{Height: height}
	if err := c.doJSON(ctx, http.MethodPost, supplyPath, req, &supply); err != nil {
		return nil, fmt.Errorf("fetch supply at height %d: %w", height, err)
	}
	return &supply, nil
}

package rpc

import (
	"context"
	"fmt"
	"net/http"

	"github.com/canopy-network/canopy/fsm"
)

// AllParamsByHeight queries the /v1/query/params endpoint to retrieve all chain parameters at a specific height.
// This includes consensus, validator, fee, and governance parameters.
//
// Parameters:
//   - height: The block height to query params for (0 = latest)
//
// Returns:
//   - *fsm.Params: Complete set of chain parameters at the specified height (protobuf type from Canopy)
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) AllParamsByHeight(ctx context.Context, height uint64) (*fsm.Params, error) {
	var params fsm.Params
	req := QueryByHeightRequest{Height: height}
	if err := c.doJSON(ctx, http.MethodPost, allParamsPath, req, &params); err != nil {
		return nil, fmt.Errorf("fetch all params at height %d: %w", height, err)
	}
	return &params, nil
}

// ValParamsByHeight queries the /v1/query/val-params endpoint to retrieve validator parameters at a specific height.
// Validator parameters control staking, slashing, committees, and orders.
//
// Parameters:
//   - height: The block height to query params for (0 = latest)
//
// Returns:
//   - *fsm.ValidatorParams: Validator-related parameters at the specified height (protobuf type from Canopy)
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) ValParamsByHeight(ctx context.Context, height uint64) (*fsm.ValidatorParams, error) {
	var params fsm.ValidatorParams
	req := QueryByHeightRequest{Height: height}
	if err := c.doJSON(ctx, http.MethodPost, valParamsPath, req, &params); err != nil {
		return nil, fmt.Errorf("fetch validator params at height %d: %w", height, err)
	}
	return &params, nil
}

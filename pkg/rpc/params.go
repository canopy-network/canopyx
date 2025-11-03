package rpc

import (
	"context"
	"fmt"
	"net/http"
)

const (
	allParamsPath = "/v1/query/params"
	feeParamsPath = "/v1/query/fee-params"
	conParamsPath = "/v1/query/con-params"
	valParamsPath = "/v1/query/val-params"
	govParamsPath = "/v1/query/gov-params"
)

// AllParams queries the /v1/query/params endpoint to retrieve all chain parameters at a specific height.
// This includes consensus, validator, fee, and governance parameters.
//
// Parameters:
//   - height: The block height to query params for (0 = latest)
//
// Returns:
//   - *RpcAllParams: Complete set of chain parameters at the specified height
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) AllParams(ctx context.Context, height uint64) (*RpcAllParams, error) {
	var params RpcAllParams
	req := map[string]any{"height": height}
	if err := c.doJSON(ctx, http.MethodPost, allParamsPath, req, &params); err != nil {
		return nil, fmt.Errorf("fetch all params at height %d: %w", height, err)
	}
	return &params, nil
}

// FeeParams queries the /v1/query/fee-params endpoint to retrieve fee parameters at a specific height.
// Fee parameters define the costs for various transaction types.
//
// Parameters:
//   - height: The block height to query params for (0 = latest)
//
// Returns:
//   - *FeeParams: Fee-related parameters at the specified height
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) FeeParams(ctx context.Context, height uint64) (*FeeParams, error) {
	var params FeeParams
	req := map[string]any{"height": height}
	if err := c.doJSON(ctx, http.MethodPost, feeParamsPath, req, &params); err != nil {
		return nil, fmt.Errorf("fetch fee params at height %d: %w", height, err)
	}
	return &params, nil
}

// ConParams queries the /v1/query/con-params endpoint to retrieve consensus parameters at a specific height.
// Consensus parameters include block size, protocol version, and chain hierarchy.
//
// Parameters:
//   - height: The block height to query params for (0 = latest)
//
// Returns:
//   - *ConsensusParams: Consensus-related parameters at the specified height
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) ConParams(ctx context.Context, height uint64) (*ConsensusParams, error) {
	var params ConsensusParams
	req := map[string]any{"height": height}
	if err := c.doJSON(ctx, http.MethodPost, conParamsPath, req, &params); err != nil {
		return nil, fmt.Errorf("fetch consensus params at height %d: %w", height, err)
	}
	return &params, nil
}

// ValParams queries the /v1/query/val-params endpoint to retrieve validator parameters at a specific height.
// Validator parameters control staking, slashing, committees, and orders.
//
// Parameters:
//   - height: The block height to query params for (0 = latest)
//
// Returns:
//   - *ValidatorParams: Validator-related parameters at the specified height
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) ValParams(ctx context.Context, height uint64) (*ValidatorParams, error) {
	var params ValidatorParams
	req := map[string]any{"height": height}
	if err := c.doJSON(ctx, http.MethodPost, valParamsPath, req, &params); err != nil {
		return nil, fmt.Errorf("fetch validator params at height %d: %w", height, err)
	}
	return &params, nil
}

// GovParams queries the /v1/query/gov-params endpoint to retrieve governance parameters at a specific height.
// Governance parameters control DAO and governance settings.
//
// Parameters:
//   - height: The block height to query params for (0 = latest)
//
// Returns:
//   - *GovParams: Governance-related parameters at the specified height
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) GovParams(ctx context.Context, height uint64) (*GovParams, error) {
	var params GovParams
	req := map[string]any{"height": height}
	if err := c.doJSON(ctx, http.MethodPost, govParamsPath, req, &params); err != nil {
		return nil, fmt.Errorf("fetch governance params at height %d: %w", height, err)
	}
	return &params, nil
}

package rpc

import (
	"context"
	"fmt"
	"net/http"
)

// StateByHeight queries the /v1/query/state endpoint to retrieve blockchain state at a specific height.
// When height is 0, it returns the genesis state. When height is nil, it returns the current state.
//
// The endpoint returns:
//   - time: state timestamp (genesis time if height=0)
//   - accounts: array of account objects with address and amount
//   - validators: array of validators
//   - pools: array of pools
//   - params: chain parameters including RootChainID
//
// This unified method replaces the previous separate State() and GetGenesisState() methods.
func (c *HTTPClient) StateByHeight(ctx context.Context, height uint64) (*StateResponse, error) {
	var state StateResponse
	var err error

	// Height specified - fetch state at that height (GET with height param)
	req := QueryByHeightRequest{Height: height}
	err = c.doJSON(ctx, http.MethodGet, statePath, req, &state)

	if err != nil {
		return nil, fmt.Errorf("fetch state at height=%d: %w", height, err)
	}

	return &state, nil
}

// Params represents the governance parameters returned from /v1/query/state.
// Contains consensus, validator, fee, and other blockchain parameters.
type Params struct {
	ConsensusParams *ConsensusParams `json:"consensusParams"`
	ValidatorParams *ValidatorParams `json:"validatorParams"`
	FeeParams       *FeeParams       `json:"feeParams"`
	GovParams       *GovParams       `json:"govParams"`
}

// GenesisState represents the full genesis state returned by the RPC endpoint.
// This contains all initial blockchain states, including accounts, validators, pools, and parameters.
type GenesisState struct {
	Time     uint64     `json:"time"`     // Genesis timestamp
	Accounts []*Account `json:"accounts"` // Initial account balances
	// Future fields for complete genesis state
	Validators []interface{} `json:"validators"` // Initial validators (not used yet)
	Pools      []interface{} `json:"pools"`      // Initial pools (not used yet)
	Params     *Params       `json:"params"`     // Chain parameters
}

// StateResponse represents the response from /v1/query/state endpoint.
// This is an alias for GenesisState as they return the same structure.
type StateResponse = GenesisState

package rpc

import (
	"context"
	"fmt"
	"net/http"

	"github.com/canopy-network/canopy/lib"
)

// CommitteesDataByHeight queries the /v1/query/committees-data endpoint to retrieve all committees data at a specific height.
// This returns data for all active committees in the network.
//
// Parameters:
//   - height: The block height to query committees data for (0 = latest)
//
// Returns:
//   - []*lib.CommitteeData: List of all committees data at the specified height (from canopy/lib)
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) CommitteesDataByHeight(ctx context.Context, height uint64) ([]*lib.CommitteeData, error) {
	var committeesData struct {
		List []*lib.CommitteeData `json:"list"`
	}
	req := QueryByHeightRequest{Height: height}
	if err := c.doJSON(ctx, http.MethodPost, committeesDataPath, req, &committeesData); err != nil {
		return nil, fmt.Errorf("fetch committees data at height %d: %w", height, err)
	}
	if committeesData.List == nil {
		return []*lib.CommitteeData{}, nil
	}
	return committeesData.List, nil
}

// SubsidizedCommitteesByHeight queries the /v1/query/subsidized-committees endpoint to retrieve subsidized committee IDs at a specific height.
// Subsidized committees meet the stake percentage requirement and receive subsidies from the protocol.
//
// Parameters:
//   - height: The block height to query subsidized committees for (0 = latest)
//
// Returns:
//   - []uint64: List of subsidized committee IDs at the specified height
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) SubsidizedCommitteesByHeight(ctx context.Context, height uint64) ([]uint64, error) {
	var committees []uint64
	req := QueryByHeightRequest{Height: height}
	if err := c.doJSON(ctx, http.MethodPost, subsidizedCommitteesPath, req, &committees); err != nil {
		return nil, fmt.Errorf("fetch subsidized committees at height %d: %w", height, err)
	}
	if committees == nil {
		return []uint64{}, nil
	}
	return committees, nil
}

// RetiredCommitteesByHeight queries the /v1/query/retired-committees endpoint to retrieve retired committee IDs at a specific height.
// Retired committees have shut down and are marked as 'forever unsubsidized' on the root chain.
//
// Parameters:
//   - height: The block height to query retired committees for (0 = latest)
//
// Returns:
//   - []uint64: List of retired committee IDs at the specified height
//   - error: If the endpoint is unreachable or returns invalid data
func (c *HTTPClient) RetiredCommitteesByHeight(ctx context.Context, height uint64) ([]uint64, error) {
	var committees []uint64
	req := QueryByHeightRequest{Height: height}
	if err := c.doJSON(ctx, http.MethodPost, retiredCommitteesPath, req, &committees); err != nil {
		return nil, fmt.Errorf("fetch retired committees at height %d: %w", height, err)
	}
	if committees == nil {
		return []uint64{}, nil
	}
	return committees, nil
}

// NOTE: RpcPaymentPercent, RpcCommitteeData, and RpcCommitteesData have been removed.
// Use lib.PaymentPercents and lib.CommitteeData from github.com/canopy-network/canopy/lib instead.
// These types are defined in the Canopy protocol and have proper JSON marshaling support.

package rpc

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/canopy-network/canopy/lib"
)

// HeadBlock represents the response from the /v1/query/height endpoint.
type HeadBlock struct {
	Height uint64 `json:"height"`
}

// ChainHead returns the height of the chain head.
func (c *HTTPClient) ChainHead(ctx context.Context) (uint64, error) {
	var resp HeadBlock
	err := c.doJSON(ctx, http.MethodPost, headPath, EmptyRequest{}, &resp)
	if err == nil && resp.Height > 0 {
		return resp.Height, nil
	}
	return uint64(0), fmt.Errorf("cannot probe head %v", err)
}

// BlockByHeight returns the Canopy BlockResult at the given height.
// JSON from RPC is unmarshaled directly into lib.BlockResult (protobuf types support JSON).
// Callers should convert to indexer models using transform.Block() if needed.
func (c *HTTPClient) BlockByHeight(ctx context.Context, h uint64) (*lib.BlockResult, error) {
	var out lib.BlockResult
	if err := c.doJSON(ctx, http.MethodPost, blockByHeightPath, QueryByHeightRequest{Height: h}, &out); err != nil {
		return nil, err
	}

	if out.BlockHeader.Height != h {
		// OR maybe just a bug?
		return nil, errors.New("block height is not ready yet (it comes empty from canopy)")
	}

	if out.Meta.Size == 0 {
		// OR maybe just a bug?
		return nil, errors.New("block size is zero (means that probably is not ready yet to be indexed)")
	}

	return &out, nil
}

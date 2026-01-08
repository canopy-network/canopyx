package rpc

import (
	"context"
	"net/http"

	"github.com/canopy-network/canopy/fsm"
)

func (c *HTTPClient) Blob(ctx context.Context, height uint64) (*fsm.IndexerBlobs, error) {
	var resp fsm.IndexerBlobs
	if err := c.doProtobuf(ctx, http.MethodPost, indexerBlobsPath, QueryByHeightRequest{Height: height}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

package activity

import (
	"context"
	"time"

	"github.com/canopy-network/canopyx/app/indexer/types"
)

// FetchBlobFromRPC fetches the indexer blob payload from the RPC endpoint.
func (ac *Context) FetchBlobFromRPC(ctx context.Context, in types.ActivityFetchBlockInput) (types.ActivityFetchBlobOutput, error) {
	start := time.Now()

	cli, err := ac.rpcClientForHeight(ctx, in.Height)
	if err != nil {
		return types.ActivityFetchBlobOutput{}, err
	}

	blobs, err := cli.Blob(ctx, in.Height)
	if err != nil {
		return types.ActivityFetchBlobOutput{}, err
	}

	durationMs := float64(time.Since(start).Microseconds()) / 1000.0
	return types.ActivityFetchBlobOutput{Blobs: blobs, DurationMs: durationMs}, nil
}

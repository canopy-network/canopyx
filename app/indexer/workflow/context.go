package workflow

import (
	"github.com/canopy-network/canopyx/app/indexer/activity"
	"github.com/canopy-network/canopyx/pkg/temporal"
)

// Config holds the workflow configuration.
type Config struct {
	CatchupThreshold        uint64
	DirectScheduleBatchSize uint64
	SchedulerBatchSize      uint64
	BlockTimeSeconds        uint64
}

// Context holds the workflow context.
type Context struct {
	ChainID         uint64
	ChainClient     *temporal.ChainClient
	ActivityContext *activity.Context
	Config          Config
}

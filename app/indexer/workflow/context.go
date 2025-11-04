package workflow

import (
	"github.com/canopy-network/canopyx/app/indexer/activity"
	"github.com/canopy-network/canopyx/pkg/temporal"
)

type Context struct {
	TemporalClient  *temporal.Client
	ActivityContext *activity.Context
	Config          Config
}

type Config struct {
	CatchupThreshold        uint64
	DirectScheduleBatchSize uint64
	SchedulerBatchSize      uint64
	BlockTimeSeconds        uint64
}

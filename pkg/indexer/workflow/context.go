package workflow

import (
	"github.com/canopy-network/canopyx/pkg/indexer/activity"
	"github.com/canopy-network/canopyx/pkg/temporal"
)

type Context struct {
	TemporalClient  *temporal.Client
	ActivityContext *activity.Context
}

package workflow

import (
	"github.com/canopy-network/canopyx/app/admin/activity"
	"github.com/canopy-network/canopyx/pkg/temporal"
)

type Context struct {
	ActivityContext *activity.Context
	// This is needed to trigger the new workflows and get the task queue name
	TemporalClient *temporal.Client
}

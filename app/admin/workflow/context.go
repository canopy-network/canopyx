package workflow

import (
	"github.com/canopy-network/canopyx/app/admin/activity"
	"github.com/canopy-network/canopyx/pkg/temporal"
)

// Context holds dependencies for admin maintenance workflows.
type Context struct {
	TemporalManager *temporal.ClientManager
	ActivityContext *activity.Context
}

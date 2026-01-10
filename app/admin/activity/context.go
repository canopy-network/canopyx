package activity

import (
	adminstore "github.com/canopy-network/canopyx/pkg/db/admin"
	globalstore "github.com/canopy-network/canopyx/pkg/db/global"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"go.uber.org/zap"
)

// Context holds dependencies for admin maintenance activities.
type Context struct {
	Logger          *zap.Logger
	AdminDB         adminstore.Store
	GlobalDB        globalstore.Store
	TemporalManager *temporal.ClientManager
}

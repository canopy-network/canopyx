package activity

import (
	adminstore "github.com/canopy-network/canopyx/pkg/db/admin"
	"github.com/canopy-network/canopyx/pkg/db/crosschain"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"go.uber.org/zap"
)

// Context holds dependencies for admin maintenance activities.
type Context struct {
	Logger          *zap.Logger
	AdminDB         adminstore.Store
	CrossChainDB    crosschain.Store
	TemporalManager *temporal.ClientManager
}

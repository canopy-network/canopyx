package activity

import (
	"github.com/canopy-network/canopyx/pkg/db/crosschain"
	"github.com/canopy-network/canopyx/pkg/temporal"
	"go.uber.org/zap"
)

// Context holds dependencies for admin maintenance activities.
type Context struct {
	Logger         *zap.Logger
	CrossChainDB   *crosschain.Store
	TemporalClient *temporal.Client
}

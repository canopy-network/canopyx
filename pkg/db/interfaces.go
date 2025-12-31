package db

import (
	adminpkg "github.com/canopy-network/canopyx/pkg/db/admin"
	chainpkg "github.com/canopy-network/canopyx/pkg/db/chain"
	crosschainpkg "github.com/canopy-network/canopyx/pkg/db/crosschain"
)

type AdminStore = adminpkg.Store
type ChainStore = chainpkg.Store
type CrossChainStore = crosschainpkg.Store

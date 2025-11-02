package rpc

// TODO: replace this types with the RPC types from canopy as package once available
//   that will means a mayor refactor probably but deserves the time

// --- Query types

type QueryByHeightRequest map[string]any

func NewQueryByHeightRequest(height uint64) QueryByHeightRequest {
	return QueryByHeightRequest{"height": height}
}

func (r QueryByHeightRequest) Height() uint64 {
	if height, ok := r["height"].(uint64); !ok {
		return 0
	} else {
		return height
	}
}

// --- Response types

// GenesisState represents the full genesis state returned by the RPC endpoint.
// This contains all initial blockchain states, including accounts, validators, pools, and parameters.
type GenesisState struct {
	Time     uint64     `json:"time"`     // Genesis timestamp
	Accounts []*Account `json:"accounts"` // Initial account balances
	// Future fields for complete genesis state
	Validators []interface{} `json:"validators"` // Initial validators (not used yet)
	Pools      []interface{} `json:"pools"`      // Initial pools (not used yet)
	Params     *Params       `json:"params"`     // Chain parameters
}

// Params represents the governance parameters returned from /v1/query/state.
// Contains consensus, validator, fee, and other blockchain parameters.
type Params struct {
	ConsensusParams *ConsensusParams `json:"consensusParams"`
	// Other param sections can be added here as needed
}

// ConsensusParams represents consensus-related parameters.
// Includes block size limits, protocol version, and root chain ID.
type ConsensusParams struct {
	BlockSize       uint64 `json:"blockSize"`       // Maximum allowed block size
	ProtocolVersion string `json:"protocolVersion"` // Minimum protocol version required
	RootChainID     uint64 `json:"rootChainID"`     // Root chain ID (parent chain)
	Retired         uint64 `json:"retired"`         // Whether chain is retired
}

// StateResponse represents the response from /v1/query/state endpoint.
// This is an alias for GenesisState as they return the same structure.
type StateResponse = GenesisState

// RpcDexLimitOrder represents a DEX limit order from the RPC.
// This matches the DexLimitOrder proto structure returned by dex-batch endpoints.
type RpcDexLimitOrder struct {
	OrderID         string `json:"orderId"`         // Hex-encoded order ID
	AmountForSale   uint64 `json:"amountForSale"`   // Amount of asset being sold
	RequestedAmount uint64 `json:"requestedAmount"` // Minimum amount of counter-asset to receive
	Address         string `json:"address"`         // Hex-encoded address
}

// RpcDexLiquidityDeposit represents a liquidity deposit from the RPC.
// This matches the DexLiquidityDeposit proto structure.
type RpcDexLiquidityDeposit struct {
	OrderID string `json:"orderId"` // Hex-encoded order ID
	Address string `json:"address"` // Hex-encoded address
	Amount  uint64 `json:"amount"`  // Amount being deposited
}

// RpcDexLiquidityWithdraw represents a liquidity withdrawal from the RPC.
// This matches the DexLiquidityWithdraw proto structure.
type RpcDexLiquidityWithdraw struct {
	OrderID string `json:"orderId"` // Hex-encoded order ID
	Address string `json:"address"` // Hex-encoded address
	Percent uint64 `json:"percent"` // Percentage of pool points being withdrawn (0-100)
}

// RpcPoolPoints represents an address's pool points allocation.
// This matches the PoolPoints proto structure.
type RpcPoolPoints struct {
	Address string `json:"address"` // Hex-encoded address
	Points  uint64 `json:"points"`  // Number of pool points
}

// RpcDexBatch represents a DEX batch from the RPC.
// This matches the DexBatch proto structure returned by /v1/query/dex-batch and /v1/query/next-dex-batch.
type RpcDexBatch struct {
	Committee       uint64                     `json:"committee"`       // Committee ID (counter-asset chain ID)
	ReceiptHash     string                     `json:"receiptHash"`     // Hash of counter chain batch receipts
	Orders          []*RpcDexLimitOrder        `json:"orders"`          // Limit orders in this batch
	Deposits        []*RpcDexLiquidityDeposit  `json:"deposits"`        // Liquidity deposits in this batch
	Withdrawals     []*RpcDexLiquidityWithdraw `json:"withdrawals"`     // Liquidity withdrawals in this batch
	PoolSize        uint64                     `json:"poolSize"`        // Current liquidity pool balance
	CounterPoolSize uint64                     `json:"counterPoolSize"` // Last known counter chain pool size
	PoolPoints      []*RpcPoolPoints           `json:"poolPoints"`      // Pool points by holder
	TotalPoolPoints uint64                     `json:"totalPoolPoints"` // Total pool points
	Receipts        []uint64                   `json:"receipts"`        // Execution receipts (amounts distributed)
	LockedHeight    uint64                     `json:"lockedHeight"`    // Height when batch was locked
}

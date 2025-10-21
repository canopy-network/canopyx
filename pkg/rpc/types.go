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
	Params     interface{}   `json:"params"`     // Chain parameters (not used yet)
}

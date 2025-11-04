package rpc

// TODO: replace this types with the RPC types from canopy as package once available
//   that will means a mayor refactor probably but deserves the time

// --- Request types for RPC queries ---
// All request types are consolidated here for type safety and to prepare for canopy RPC migration.
// These replace map[string]any usage throughout the RPC package.

// QueryByHeightRequest represents a properly typed height query request.
// Replaces map[string]any for type safety and to prepare for canopy RPC migration.
type QueryByHeightRequest struct {
	Height uint64 `json:"height"`
}

func NewQueryByHeightRequest(height uint64) QueryByHeightRequest {
	return QueryByHeightRequest{Height: height}
}

// DexBatchRequest represents a properly typed DEX batch query request.
// Used for both /v1/query/dex-batch and /v1/query/next-dex-batch endpoints.
type DexBatchRequest struct {
	Height    uint64 `json:"height"`
	Committee uint64 `json:"committee"`
}

// CommitteeDataRequest represents a properly typed committee data query request.
// Used for /v1/query/committee-data endpoint which requires both height and chainId.
type CommitteeDataRequest struct {
	Height  uint64 `json:"height"`
	ChainID uint64 `json:"chainId"` // Note: JSON uses chainId (camelCase)
}

// DexPriceRequest represents a properly typed DEX price query request.
// Used for /v1/query/dex-price endpoint.
type DexPriceRequest struct {
	Height uint64 `json:"height"`
	ID     uint64 `json:"id"` // Chain ID for the price query
}

// EmptyRequest represents an empty request body.
// Used for endpoints that don't require parameters but expect a JSON object.
type EmptyRequest struct{}

// PoolByIDRequest represents a properly typed pool query request.
// Used for /v1/query/pool endpoint.
type PoolByIDRequest struct {
	ID uint64 `json:"id"` // Pool ID
}

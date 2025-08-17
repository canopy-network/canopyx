package types

type ChainStatus struct {
	LastIndexed uint64 `json:"last_indexed"`
	Head        uint64 `json:"head"`
}

package types

type ReindexRequest struct {
	Heights []uint64 `json:"heights"`
	From    *uint64  `json:"from"`
	To      *uint64  `json:"to"`
}
